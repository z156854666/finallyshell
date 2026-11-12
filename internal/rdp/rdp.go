// Package rdp opens a remote desktop connection in the system's own RDP
// client, as FinalShell's RDP links do: Remote Desktop Connection
// (mstsc) on Windows, FreeRDP where it is installed, Microsoft Remote
// Desktop on macOS.
//
// The RDP protocol is not spoken here: a client that speaks it runs in a
// window of its own, with the address, the user and the password the
// connection saved, and what is shared with the remote desktop. The app
// keeps the session's row and tab as for a shell; closing the tab ends
// nothing, as the window is the system's.
package rdp

import (
	"errors"
	"net"
	"strconv"

	"finallyshell/internal/config"
)

// DefaultPort is the port a remote desktop listens on.
const DefaultPort = 3389

// ErrNoClient is what Launch fails with when the system has no remote
// desktop client this package can drive.
var ErrNoClient = errors.New("no remote desktop client")

// Config is a remote desktop connection to open.
type Config struct {
	Host     string
	Port     int // 0 for DefaultPort
	User     string
	Domain   string
	Password string // "" when the app does not know it: the client asks
	// Width and Height are the desktop's size in pixels; 0 for the
	// client's own, full screen on Windows.
	Width  int
	Height int
	// Depth is the color depth in bits: 16, 24 or 32; 0 for 32.
	Depth     int
	Clipboard bool
	Drives    bool
}

// For is the remote desktop h opens, with the password the app has for
// it, "" when it does not know one.
func For(h *config.Host, password string) Config {
	o := h.RDPOf()
	return Config{
		Host:      h.Addr,
		Port:      h.PortOr3389(),
		User:      h.User,
		Domain:    o.Domain,
		Password:  password,
		Width:     o.Width,
		Height:    o.Height,
		Depth:     o.Depth,
		Clipboard: !o.NoClipboard,
		Drives:    !o.NoDrives,
	}
}

// Address is the host and the port as the clients take them, host:port,
// the port DefaultPort when the config names none.
func (c Config) Address() string {
	port := c.Port
	if port == 0 {
		port = DefaultPort
	}
	return net.JoinHostPort(c.Host, strconv.Itoa(port))
}

// depth is the color depth the clients are asked for: 32 when the
// connection names none.
func (c Config) depth() int {
	switch c.Depth {
	case 16, 24, 32:
		return c.Depth
	}
	return 32
}

// size is the desktop's size, "" when the client chooses it.
func (c Config) size() string {
	if c.Width <= 0 || c.Height <= 0 {
		return ""
	}
	return strconv.Itoa(c.Width) + "x" + strconv.Itoa(c.Height)
}
