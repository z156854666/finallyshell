//go:build windows

package rdp

import (
	"slices"
	"testing"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

// TestMstscArgs is the command line: the address, and the size of the
// screen or the client's own, full screen.
func TestMstscArgs(t *testing.T) {
	if got, want := mstscArgs(Config{Host: "10.0.0.7"}), []string{"/v:10.0.0.7:3389", "/f"}; !slices.Equal(got, want) {
		t.Errorf("%q, want %q", got, want)
	}
	got := mstscArgs(Config{Host: "10.0.0.7", Port: 3390, Width: 1600, Height: 900})
	want := []string{"/v:10.0.0.7:3390", "/w:1600", "/h:900"}
	if !slices.Equal(got, want) {
		t.Errorf("%q, want %q", got, want)
	}
}

// TestCredentialOf is the user and the targets the client's password is
// kept under.
func TestCredentialOf(t *testing.T) {
	user, targets := credentialOf(Config{Host: "10.0.0.7", User: "zfx", Password: "pw"})
	if want := []string{"TERMSRV/10.0.0.7:3389", "TERMSRV/10.0.0.7"}; user != "zfx" || !slices.Equal(targets, want) {
		t.Errorf("%q %q, want %q", user, targets, want)
	}
	if user, _ := credentialOf(Config{Host: "w", User: "u", Domain: "CORP", Password: "pw"}); user != `CORP\u` {
		t.Errorf("user %q", user)
	}
	// Nothing to keep without a user or a password.
	for _, c := range []Config{{Host: "w", User: "u"}, {Host: "w", Password: "pw"}} {
		if user, targets := credentialOf(c); user != "" || targets != nil {
			t.Errorf("%+v: %q %q", c, user, targets)
		}
	}
}

// TestCredentialStore writes a password where the client reads it, reads
// it back as the client does, and forgets it. Its target is the test's
// own, and is removed whatever happens.
func TestCredentialStore(t *testing.T) {
	const target = "TERMSRV/finallyshell-test.invalid"
	deleteCredential(target)
	t.Cleanup(func() { deleteCredential(target) })
	if err := writeCredential(target, `CORP\zfx`, "pä§s"); err != nil {
		t.Fatal(err)
	}
	read := func() (*credentialW, bool) {
		t.Helper()
		tp, err := windows.UTF16PtrFromString(target)
		if err != nil {
			t.Fatal(err)
		}
		var p unsafe.Pointer
		if r, _, _ := procCredRead.Call(uintptr(unsafe.Pointer(tp)), credTypeGeneric, 0, uintptr(unsafe.Pointer(&p))); r == 0 {
			return nil, false
		}
		t.Cleanup(func() { procCredFree.Call(uintptr(p)) })
		return (*credentialW)(p), true
	}
	c, ok := read()
	if !ok {
		t.Fatal("the credential was not written")
	}
	if user := windows.UTF16PtrToString(c.UserName); user != `CORP\zfx` {
		t.Errorf("user %q", user)
	}
	blob := unsafe.Slice(c.CredentialBlob, int(c.CredentialBlobSize))
	u := make([]uint16, len(blob)/2)
	for i := range u {
		u[i] = uint16(blob[2*i]) | uint16(blob[2*i+1])<<8
	}
	if got := string(utf16.Decode(u)); got != "pä§s" {
		t.Errorf("password %q", got)
	}
	deleteCredential(target)
	if _, ok := read(); ok {
		t.Error("the credential stayed")
	}
}
