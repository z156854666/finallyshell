package config

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestRDPHost is a remote desktop host: its kind, the port sessions open
// and the target lists show, and the defaults of what it shares.
func TestRDPHost(t *testing.T) {
	h := &Host{Addr: "10.0.0.7", User: "Administrator"}
	if h.Kind() != KindSSH || h.IsRDP() || h.ProtocolPort() != 22 || !sameRDP(h.RDPOf(), RDPConfig{}) {
		t.Errorf("a host without a type: kind %q, port %d", h.Kind(), h.ProtocolPort())
	}
	if got := h.Target(); got != "Administrator@10.0.0.7" {
		t.Errorf("target %q", got)
	}
	h.Port = 2222
	if got := h.Target(); got != "Administrator@10.0.0.7:2222" {
		t.Errorf("target %q", got)
	}

	h.Type, h.Port = KindRDP, 0
	if h.Kind() != KindRDP || !h.IsRDP() || h.ProtocolPort() != 3389 || h.PortOr22() != 22 {
		t.Errorf("kind %q, rdp %v, port %d", h.Kind(), h.IsRDP(), h.ProtocolPort())
	}
	if got := h.Target(); got != "Administrator@10.0.0.7" {
		t.Errorf("target %q", got)
	}
	h.Port = 3390
	if got := h.Target(); got != "Administrator@10.0.0.7:3390" {
		t.Errorf("target %q", got)
	}
	// The keys of the remote desktop page, and of the file.
	h.RDP = &RDPConfig{Domain: "CORP", Width: 1280, Height: 800, Depth: 24, NoClipboard: true}
	if o := h.RDPOf(); !sameRDP(o, *h.RDP) {
		t.Errorf("%+v", o)
	}
	data, err := json.Marshal(h)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"type":"rdp"`, `"rdp":{"domain":"CORP"`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("saved %s, want %s", data, want)
		}
	}
	var back Host
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if back.Type != KindRDP || !sameRDP(back.RDPOf(), *h.RDP) {
		t.Errorf("%+v", back)
	}
	// A copy shares nothing of it.
	c := h.Clone()
	c.RDP.Domain = "OTHER"
	if h.RDP.Domain != "CORP" {
		t.Error("a copy shares the remote desktop's settings")
	}
	// A host of a newer build's type this one does not know is an SSH
	// connection, and the key is kept as it was.
	var other Host
	if err := json.Unmarshal([]byte(`{"id":"h2","type":"vnc","addr":"x"}`), &other); err != nil {
		t.Fatal(err)
	}
	if other.Kind() != KindSSH {
		t.Errorf("kind %q", other.Kind())
	}
	if data, _ := json.Marshal(other); !strings.Contains(string(data), `"type":"vnc"`) {
		t.Errorf("saved %s", data)
	}
}

// sameRDP tells whether a and b are the same settings: the struct holds
// the keys of a newer build, which a comparison cannot see.
func sameRDP(a, b RDPConfig) bool {
	return a.Domain == b.Domain && a.Width == b.Width && a.Height == b.Height &&
		a.Depth == b.Depth && a.NoClipboard == b.NoClipboard && a.NoDrives == b.NoDrives
}
