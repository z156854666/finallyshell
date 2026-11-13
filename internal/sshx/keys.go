package sshx

// Private keys as the app keeps them: what a key's text says about itself,
// for the editor's list of saved keys, and the text itself for a host that
// logs in with one (config.Host.Key) rather than with a file of its own
// (config.Host.KeyPath).

import (
	"bytes"
	"encoding/pem"
	"errors"
	"os"

	"finallyshell/internal/config"

	"golang.org/x/crypto/ssh"
)

// ErrSavedKeyGone is a host whose saved key the app does not have: one
// deleted here, or one another device has not synced yet.
var ErrSavedKeyGone = errors.New("the saved key is gone from keys.json")

// KeyInfo is what a private key says about itself, as the editor's list of
// saved keys shows it.
type KeyInfo struct {
	// Type is the key's algorithm, as keyTypeLabel names it: "ED25519",
	// "RSA", "ECDSA"; "" when the key's type could not be read.
	Type string
	// Bits is the size of the key's public part: 256 for ED25519, 4096
	// for an RSA key of that size; 0 when unknown.
	Bits int
	// Fingerprint is the key's public part, as ssh.FingerprintSHA256
	// shows it ("SHA256:…").
	Fingerprint string
	// Encrypted says the key needs a passphrase to be used.
	Encrypted bool
}

// KeyInfoOf reads a private key's text, OpenSSH or PEM. A key with a
// passphrase says what it is without it, as its public part is not
// encrypted; only the text of a key that is not a key at all is an error.
func KeyInfoOf(data []byte) (KeyInfo, error) {
	signer, err := ssh.ParsePrivateKey(data)
	if err == nil {
		return infoOf(signer.PublicKey(), false), nil
	}
	var missing *ssh.PassphraseMissingError
	if !errors.As(err, &missing) {
		return KeyInfo{}, err
	}
	info := KeyInfo{Encrypted: true}
	switch {
	case missing.PublicKey != nil:
		info = infoOf(missing.PublicKey, true)
	default:
		if pub, ok := encryptedOpenSSHPublicKey(data); ok {
			info = infoOf(pub, true)
		}
	}
	if info.Type == "" {
		info.Type = pemKeyType(data)
	}
	return info, nil
}

// encryptedOpenSSHPublicKey is the public part of an encrypted OpenSSH
// private key, which the format keeps unencrypted: the key says what it is
// without its passphrase.
func encryptedOpenSSHPublicKey(data []byte) (ssh.PublicKey, bool) {
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "OPENSSH PRIVATE KEY" {
		return nil, false
	}
	const magic = "openssh-key-v1\x00"
	if !bytes.HasPrefix(block.Bytes, []byte(magic)) {
		return nil, false
	}
	var k struct {
		CipherName    string
		KDFName       string
		KDFOptions    string
		NumKeys       uint32
		PublicKey     string
		EncryptedBlob string
	}
	if err := ssh.Unmarshal(block.Bytes[len(magic):], &k); err != nil {
		return nil, false
	}
	pub, err := ssh.ParsePublicKey([]byte(k.PublicKey))
	if err != nil {
		return nil, false
	}
	return pub, true
}

// infoOf is what pub says of a key, encrypted or not.
func infoOf(pub ssh.PublicKey, encrypted bool) KeyInfo {
	typ, bits := keyTypeAndBits(pub)
	return KeyInfo{
		Type: typ, Bits: bits,
		Fingerprint: ssh.FingerprintSHA256(pub), Encrypted: encrypted,
	}
}

// pemKeyType names the type of an encrypted PEM key from its header, which
// says the algorithm but not the size: "BEGIN RSA PRIVATE KEY" is RSA.
func pemKeyType(data []byte) string {
	switch {
	case bytes.Contains(data, []byte("RSA PRIVATE KEY")):
		return "RSA"
	case bytes.Contains(data, []byte("EC PRIVATE KEY")):
		return "ECDSA"
	case bytes.Contains(data, []byte("DSA PRIVATE KEY")):
		return "DSA"
	}
	return ""
}

// privateKeyData is h's private key: the text of the key saved in the app,
// or the file KeyPath names.
func privateKeyData(h *config.Host) ([]byte, error) {
	if h.Key == "" {
		return os.ReadFile(ExpandHome(h.KeyPath))
	}
	if k := config.LoadKeys().Key(h.Key); k != nil && k.Key != "" {
		return []byte(k.Key), nil
	}
	return nil, ErrSavedKeyGone
}
