//go:build windows

package rdp

import (
	"runtime"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	advapi32 = windows.NewLazySystemDLL("advapi32.dll")
	// Credits are written and forgotten here; the tests read them back,
	// as the client does.
	procCredWrite  = advapi32.NewProc("CredWriteW")
	procCredDelete = advapi32.NewProc("CredDeleteW")
	procCredRead   = advapi32.NewProc("CredReadW")
	procCredFree   = advapi32.NewProc("CredFree")
)

// What the credential store takes beyond the fields of credentialW: a
// generic credential, as the client's own are, kept on this computer.
const (
	credTypeGeneric         = 1 // CRED_TYPE_GENERIC
	credPersistLocalMachine = 2 // CRED_PERSIST_LOCAL_MACHINE
)

// credential is what a launch left in the credential store, to forget when
// the desktop closes: the targets written, none when there is no password
// to put there.
type credential struct {
	targets []string
}

// remove forgets the credentials the launch wrote.
func (c credential) remove() {
	for _, target := range c.targets {
		deleteCredential(target)
	}
}

// setCredential puts c's password where the client reads it, so that it
// does not ask for it. Nothing is written without a user and a password,
// and what is written is removed when the desktop closes.
func setCredential(c Config) credential {
	user, targets := credentialOf(c)
	var kept []string
	for _, target := range targets {
		if writeCredential(target, user, c.Password) == nil {
			kept = append(kept, target)
		}
	}
	return credential{targets: kept}
}

// credentialOf is what setCredential writes: the user the client logs in
// as, DOMAIN\user when the connection names a domain, and the targets it
// looks the password up by, the address it opens and the one without the
// port. Nothing for a connection without a user or a password.
func credentialOf(c Config) (user string, targets []string) {
	if c.Password == "" || c.User == "" {
		return "", nil
	}
	user = c.User
	if c.Domain != "" {
		user = c.Domain + `\` + c.User
	}
	return user, []string{"TERMSRV/" + c.Address(), "TERMSRV/" + c.Host}
}

// credentialW is a CREDENTIALW, what CredWriteW takes. Its password is
// UTF-16, as the client writes it.
type credentialW struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        windows.Filetime
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

// writeCredential puts password for user at target in the credential
// store of this computer.
func writeCredential(target, user, password string) error {
	t, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	u, err := windows.UTF16PtrFromString(user)
	if err != nil {
		return err
	}
	utf := utf16.Encode([]rune(password))
	blob := make([]byte, len(utf)*2)
	for i, v := range utf {
		blob[2*i], blob[2*i+1] = byte(v), byte(v>>8)
	}
	cred := credentialW{
		Type:               credTypeGeneric,
		TargetName:         t,
		UserName:           u,
		CredentialBlobSize: uint32(len(blob)),
		CredentialBlob:     &blob[0],
		Persist:            credPersistLocalMachine,
	}
	if r, _, err := procCredWrite.Call(uintptr(unsafe.Pointer(&cred)), 0); r == 0 {
		return err
	}
	runtime.KeepAlive(blob)
	return nil
}

// deleteCredential forgets the credential at target: none there is no
// error.
func deleteCredential(target string) {
	t, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return
	}
	procCredDelete.Call(uintptr(unsafe.Pointer(t)), credTypeGeneric, 0)
}
