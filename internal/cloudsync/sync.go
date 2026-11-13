package cloudsync

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// Request is one sync: this device's payload, and what it last synced.
type Request struct {
	Remote   Remote
	Password string
	// Local is this device's payload. Its Secrets count only with
	// Secrets set.
	Local *Payload
	// Secrets says whether this device includes its saved passwords.
	// When it does not, the remote's are kept as they are.
	Secrets bool
	// Keys says whether this device includes the private keys saved in
	// it. When it does not, the remote's are kept as they are.
	Keys bool
	// Base is the envelope this device last synced, as the remote had
	// it; nil for none. It is what the merge compares both sides with.
	Base []byte
	// BaseSecrets says whether this device included its secrets in the
	// sync that made Base. When it did not, the secrets in Base were
	// never this device's, and are not compared with: otherwise each one
	// it lacks would count as deleted here.
	BaseSecrets bool
	// BaseKeys is BaseSecrets' for the saved keys.
	BaseKeys bool
	// Force replaces the remote file with Local, without reading or
	// merging it: for a new sync password, or to start over.
	Force bool
	// Pull replaces Local with the remote file, without merging it or
	// writing anything: what the file has is taken as it is. Secrets and
	// Keys say whether Local's are replaced too.
	Pull bool
	// Device and Now name the device and the time in what is uploaded.
	Device string
	Now    func() time.Time
}

// Result is what a sync did.
type Result struct {
	// Merged is the payload both sides now have. With Request.Secrets
	// unset, its Secrets are the remote's, kept as they were; so are its
	// Keys with Request.Keys unset.
	Merged *Payload
	// Envelope is the file the remote now has: the next Request's Base.
	Envelope []byte
	// Uploaded: the remote file was written. Created: it was new.
	Uploaded, Created bool
	// Changed: Merged differs from Local, which is to be updated.
	Changed bool
	// Conflicts is how many items changed on both sides differently;
	// this device's were kept.
	Conflicts int
	// Pulled: Merged is the remote file's, which local is to take as it
	// is.
	Pulled bool
	// RemoteDevice is the device that wrote the file this one read, ""
	// when it wrote none.
	RemoteDevice string
}

// maxAttempts is how many times Sync reads, merges and writes again when
// the file changes on the remote in between.
const maxAttempts = 3

// Sync reads the remote file, merges it with the local payload and writes
// the result back when the remote lacks something.
func Sync(ctx context.Context, req Request) (*Result, error) {
	now := req.Now
	if now == nil {
		now = time.Now
	}
	local := req.Local.Clone()
	if req.Pull {
		return pull(ctx, req, local)
	}
	local.Schema = PayloadSchema
	var err error
	for range maxAttempts {
		var res *Result
		if req.Force {
			res, err = replace(ctx, req, local, now)
		} else {
			res, err = syncOnce(ctx, req, local, now)
		}
		if !errors.Is(err, ErrConflict) {
			return res, err
		}
	}
	return nil, err
}

func syncOnce(ctx context.Context, req Request, local *Payload, now func() time.Time) (*Result, error) {
	data, v, err := req.Remote.Get(ctx)
	if errors.Is(err, ErrNotFound) {
		res, err := upload(ctx, req, local, nil, nil, now)
		if res != nil {
			res.Created = true
		}
		return res, err
	}
	if err != nil {
		return nil, err
	}
	remote, err := openPayload(req.Password, data)
	if err != nil {
		return nil, err
	}
	var base *Payload
	if req.Base != nil {
		base, _ = openPayload(req.Password, req.Base) // none if it no longer opens
		if base != nil && !req.BaseSecrets {
			base.Secrets = nil
		}
	}
	if !req.Secrets || !req.Keys {
		local = local.Clone()
		if !req.Secrets {
			local.Secrets = remote.Secrets
		}
		if !req.Keys {
			local.Keys = remote.Keys
		}
	}
	merged, conflicts := Merge(base, local, remote)
	if Equal(merged, remote) {
		return &Result{
			Merged: remote, Envelope: data, Changed: !Equal(remote, local),
			Conflicts: conflicts, RemoteDevice: remote.Device,
		}, nil
	}
	res, err := upload(ctx, req, merged, data, &v, now)
	if res != nil {
		res.Changed = !Equal(merged, local)
		res.Conflicts = conflicts
		res.RemoteDevice = remote.Device
	}
	return res, err
}

// replace writes local in place of whatever the remote has, with a new
// salt, as the password may be new.
func replace(ctx context.Context, req Request, local *Payload, now func() time.Time) (*Result, error) {
	_, v, err := req.Remote.Get(ctx)
	switch {
	case errors.Is(err, ErrNotFound):
		return upload(ctx, req, local, nil, nil, now)
	case err != nil:
		return nil, err
	}
	return upload(ctx, req, local, nil, &v, now)
}

// pull reads the remote file and returns it as Merged, for local to take
// as it is: nothing is written. Secrets and keys are left out of Merged
// when the request does not include them, so that local keeps its own.
// ErrNotFound says the remote file does not exist yet.
func pull(ctx context.Context, req Request, local *Payload) (*Result, error) {
	data, _, err := req.Remote.Get(ctx)
	if err != nil {
		return nil, err
	}
	remote, err := openPayload(req.Password, data)
	if err != nil {
		return nil, err
	}
	if !req.Secrets {
		local.Secrets = nil
		remote.Secrets = nil
	}
	if !req.Keys {
		local.Keys = nil
		remote.Keys = nil
	}
	out := remote.Clone()
	out.Schema = PayloadSchema
	return &Result{
		Merged: out, Envelope: data, Changed: !Equal(out, local),
		Pulled: true, RemoteDevice: remote.Device,
	}, nil
}

// upload seals p and writes it in place of prev, the envelope read (nil
// for none).
func upload(ctx context.Context, req Request, p *Payload, prevData []byte, prev *Version, now func() time.Time) (*Result, error) {
	out := p.Clone()
	out.Schema, out.Device, out.UpdatedAt = PayloadSchema, req.Device, now().UTC()
	plain, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}
	env, err := Seal(req.Password, plain, prevData)
	if err != nil {
		return nil, err
	}
	if _, err := req.Remote.Put(ctx, env, prev); err != nil {
		return nil, err
	}
	return &Result{Merged: out, Envelope: env, Uploaded: true}, nil
}

// openPayload decrypts and reads an envelope.
func openPayload(password string, data []byte) (*Payload, error) {
	plain, err := Open(password, data)
	if err != nil {
		return nil, err
	}
	var p Payload
	if err := json.Unmarshal(plain, &p); err != nil {
		return nil, ErrNotEnvelope
	}
	if p.Schema > PayloadSchema {
		return nil, ErrNewerFormat
	}
	return &p, nil
}
