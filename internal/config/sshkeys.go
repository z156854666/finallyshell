package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// KeysVersion is the version of keys.json this build writes.
const KeysVersion = 1

// SSHKey is a private key saved in the app, as keys.json keeps it: a host
// that logs in with a key names one of them (Host.Key) rather than a file,
// so that the key stays where the app keeps it, and travels with the
// connections when they sync, as FinalShell's keys travel with its
// connections.
//
// Its passphrase is not here, as a password is not in hosts.json: it is the
// host's secret, in the system's credential store (internal/app).
type SSHKey struct {
	ID string `json:"id"`
	// Name is how the key shows in the editor's list: the key file's name
	// when it was imported, or one the user gave it.
	Name string `json:"name"`
	// Type is the key's algorithm, as ssh-add -l names it: "ED25519",
	// "RSA", "ECDSA". "" when the key's type could not be read.
	Type string `json:"type,omitempty"`
	// Bits is the size of the key's public part: 256 for ED25519, 4096
	// for an RSA key of that size; 0 when unknown.
	Bits int `json:"bits,omitempty"`
	// Fingerprint is the key's public part, as ssh.FingerprintSHA256
	// shows it ("SHA256:…"): the dialog keeps one copy of a key imported
	// twice.
	Fingerprint string `json:"fingerprint,omitempty"`
	// Encrypted says the key needs a passphrase to be used.
	Encrypted bool `json:"encrypted,omitempty"`
	// AddedAt is when the key was imported; zero for one synced here.
	AddedAt time.Time `json:"addedAt,omitzero"`
	// Key is the private key's text, as imported: OpenSSH or PEM.
	Key string `json:"key"`
}

// KeyStore is keys.json: the private keys saved in the app.
type KeyStore struct {
	Version int       `json:"version"`
	Keys    []*SSHKey `json:"keys"`
}

func keysPath() string { return filepath.Join(Dir(), "keys.json") }

// LoadKeys reads keys.json: no keys when there is none, or it does not
// parse. A key without an ID, or without its text, is dropped.
func LoadKeys() *KeyStore {
	s := &KeyStore{Version: KeysVersion}
	if data, err := os.ReadFile(keysPath()); err == nil {
		json.Unmarshal(data, s)
	}
	keys := s.Keys[:0]
	for _, k := range s.Keys {
		if k != nil && k.ID != "" && k.Key != "" {
			keys = append(keys, k)
		}
	}
	s.Keys = keys
	return s
}

// SaveKeys writes keys.json.
func SaveKeys(s *KeyStore) error {
	s.Version = KeysVersion
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(keysPath(), append(data, '\n'))
}

// Key is the saved key with the ID: nil for one that is not there, as a
// host whose key another device has not synced yet.
func (s *KeyStore) Key(id string) *SSHKey {
	if i := s.Find(id); i >= 0 {
		return s.Keys[i]
	}
	return nil
}

// Find is the row of the saved key with the ID, -1 for none.
func (s *KeyStore) Find(id string) int {
	if id == "" {
		return -1
	}
	for i, k := range s.Keys {
		if k.ID == id {
			return i
		}
	}
	return -1
}

// FindFingerprint is the row of the saved key with the fingerprint, -1 for
// none: the same key imported twice is one key.
func (s *KeyStore) FindFingerprint(fingerprint string) int {
	if fingerprint == "" {
		return -1
	}
	for i, k := range s.Keys {
		if k.Fingerprint == fingerprint {
			return i
		}
	}
	return -1
}

// Remove takes out the saved key with the ID, reporting whether it had one.
func (s *KeyStore) Remove(id string) bool {
	i := s.Find(id)
	if i < 0 {
		return false
	}
	s.Keys = append(s.Keys[:i], s.Keys[i+1:]...)
	return true
}

// NewKeyID is the ID of a new saved key.
func NewKeyID() string { return fmt.Sprintf("k%d", newID()) }
