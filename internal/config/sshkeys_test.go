package config

import (
	"os"
	"path/filepath"
	"testing"
)

// keys.json keeps the saved keys, by ID and by fingerprint, and a key
// without an ID, or without its text, is not one.
func TestKeyStore(t *testing.T) {
	t.Setenv("FINALLYSHELL_DIR", t.TempDir())
	if s := LoadKeys(); len(s.Keys) != 0 {
		t.Fatalf("a new store has %d keys", len(s.Keys))
	}
	s := LoadKeys()
	s.Keys = append(s.Keys,
		&SSHKey{ID: "k1", Name: "id_ed25519", Type: "ED25519", Bits: 256, Fingerprint: "SHA256:a", Key: "one"},
		&SSHKey{ID: "k2", Name: "id_rsa", Type: "RSA", Bits: 4096, Fingerprint: "SHA256:b", Key: "two"})
	if err := SaveKeys(s); err != nil {
		t.Fatal(err)
	}
	got := LoadKeys()
	if len(got.Keys) != 2 || got.Key("k1") == nil || got.Key("k1").Key != "one" {
		t.Fatalf("loaded %+v", got.Keys)
	}
	if got.FindFingerprint("SHA256:b") != 1 || got.Find("k9") != -1 || got.Key("") != nil {
		t.Errorf("found %d, %d", got.FindFingerprint("SHA256:b"), got.Find("k9"))
	}
	if !got.Remove("k1") || got.Remove("k1") || len(got.Keys) != 1 || got.Keys[0].ID != "k2" {
		t.Errorf("removed %+v", got.Keys)
	}

	// A key without an ID, or without its text, as a half-written file
	// leaves, is dropped.
	data := `{"version": 1, "keys": [{"id": "k1"}, {"name": "x"}, {"id": "k3", "key": "three"}]}`
	if err := os.WriteFile(filepath.Join(Dir(), "keys.json"), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	if keys := LoadKeys(); len(keys.Keys) != 1 || keys.Keys[0].ID != "k3" {
		t.Errorf("loaded %+v", keys.Keys)
	}
}
