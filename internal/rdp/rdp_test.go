package rdp

import (
	"slices"
	"strings"
	"testing"

	"finallyshell/internal/config"
)

// TestFor is the connection a saved remote desktop opens: the port of an
// RDP host, its domain and what it shares, from the host's keys.
func TestFor(t *testing.T) {
	h := &config.Host{ID: "h1", Addr: "10.0.0.7", User: "Administrator", Type: config.KindRDP,
		RDP: &config.RDPConfig{Domain: "CORP", Width: 1280, Height: 800, Depth: 24, NoClipboard: true}}
	c := For(h, "secret")
	want := Config{Host: "10.0.0.7", Port: 3389, User: "Administrator", Password: "secret",
		Domain: "CORP", Width: 1280, Height: 800, Depth: 24, Clipboard: false, Drives: true}
	if c != want {
		t.Errorf("%+v, want %+v", c, want)
	}
	if got := c.Address(); got != "10.0.0.7:3389" {
		t.Errorf("address %q", got)
	}
	if got := c.size(); got != "1280x800" || c.depth() != 24 {
		t.Errorf("size %q, depth %d", got, c.depth())
	}

	// A host without the keys, as every host saved before RDP was
	// supported: the default port, 32 bits, both shared.
	h = &config.Host{Addr: "win", Type: config.KindRDP}
	c = For(h, "")
	if c.Port != DefaultPort || c.Depth != 0 || !c.Clipboard || !c.Drives || c.size() != "" {
		t.Errorf("%+v", c)
	}
	if got := c.Address(); got != "win:3389" {
		t.Errorf("address %q", got)
	}
	if got := c.depth(); got != 32 {
		t.Errorf("depth %d", got)
	}
	// 16 and 24 are kept; anything else is 32.
	for _, d := range []int{16, 24, 32} {
		if got := (Config{Depth: d}).depth(); got != d {
			t.Errorf("depth %d: %d", d, got)
		}
	}
	if got := (Config{Depth: 8}).depth(); got != 32 {
		t.Errorf("depth 8: %d", got)
	}
}

// TestFreeRDPArgs is the command line FreeRDP opens a connection with.
func TestFreeRDPArgs(t *testing.T) {
	c := Config{Host: "10.0.0.7", Port: 3390, User: "u", Domain: "CORP", Password: "pw",
		Width: 1600, Height: 900, Depth: 16, Clipboard: true, Drives: true}
	want := []string{"/v:10.0.0.7:3390", "/u:u", "/d:CORP", "/p:pw", "/size:1600x900", "/bpp:16",
		"+auto-reconnect", "+clipboard", "/drive:Home,/home/u"}
	if got := freeRDPArgs(c, "/home/u"); !slices.Equal(got, want) {
		t.Errorf("%q, want %q", got, want)
	}
	// Full screen, no clipboard, no drives, no password.
	c = Config{Host: "10.0.0.7"}
	want = []string{"/v:10.0.0.7:3389", "/f", "/bpp:32", "+auto-reconnect", "-clipboard"}
	if got := freeRDPArgs(c, "/home/u"); !slices.Equal(got, want) {
		t.Errorf("%q, want %q", got, want)
	}
}

// TestConnectionFileName is the file's name: the address and the user,
// which no path can hold as they are.
func TestConnectionFileName(t *testing.T) {
	c := Config{Host: "10.0.0.7", Port: 3390, User: `CORP\Administrator`}
	if got, want := connectionFileName(c), "10.0.0.7_3390-CORP_Administrator.rdp"; got != want {
		t.Errorf("%q, want %q", got, want)
	}
	if got, want := connectionFileName(Config{Host: "win"}), "win_3389.rdp"; got != want {
		t.Errorf("%q, want %q", got, want)
	}
}

// TestConnectionFile is the .rdp file macOS's client opens: what it sets,
// and that it asks for the password, which no file of another computer can
// carry.
func TestConnectionFile(t *testing.T) {
	c := Config{Host: "10.0.0.7", Port: 3390, User: "Administrator", Domain: "CORP",
		Width: 1280, Height: 800, Depth: 24, Clipboard: true, Drives: true}
	got := connectionFile(c)
	for _, line := range []string{
		"full address:s:10.0.0.7:3390",
		"username:s:Administrator",
		"domain:s:CORP",
		"prompt for credentials:i:1",
		"screen mode id:i:1",
		"desktopwidth:i:1280",
		"desktopheight:i:800",
		"session bpp:i:24",
		"redirectclipboard:i:1",
		"drivestoredirect:s:*",
	} {
		if !strings.Contains(got, line+"\r\n") {
			t.Errorf("no %q in\n%s", line, got)
		}
	}

	// Without a size the desktop is full screen.
	got = connectionFile(Config{Host: "win", User: "u", Clipboard: false, Drives: false})
	for _, line := range []string{
		"full address:s:win:3389",
		"screen mode id:i:2",
		"session bpp:i:32",
		"redirectclipboard:i:0",
		"drivestoredirect:s:",
	} {
		if !strings.Contains(got, line+"\r\n") {
			t.Errorf("no %q in\n%s", line, got)
		}
	}
	if strings.Contains(got, "password 51") || strings.Contains(got, "domain:s:") {
		t.Errorf("a password or a domain with none:\n%s", got)
	}
}
