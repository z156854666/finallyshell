package sshx

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"testing"

	"golang.org/x/crypto/ssh"
)

// A private key says what it is without its passphrase, encrypted or not,
// and what is not a key at all is an error.
func TestKeyInfoOf(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatal(err)
	}
	locked, err := ssh.MarshalPrivateKeyWithPassphrase(priv, "", []byte("phrase"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name      string
		data      []byte
		encrypted bool
	}{
		{"a key", pem.EncodeToMemory(plain), false},
		{"a key with a passphrase", pem.EncodeToMemory(locked), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info, err := KeyInfoOf(tc.data)
			if err != nil {
				t.Fatal(err)
			}
			if info.Type != "ED25519" || info.Bits != 256 || info.Encrypted != tc.encrypted || info.Fingerprint == "" {
				t.Errorf("info %+v", info)
			}
		})
	}
	if _, err := KeyInfoOf([]byte("not a key")); err == nil {
		t.Error("what is not a key was read as one")
	}
}
