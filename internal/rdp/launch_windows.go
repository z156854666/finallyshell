//go:build windows

package rdp

import "fmt"

// clientName is what the app shows as the client a connection opened in.
const clientName = "Remote Desktop Connection"

// Launch opens c in Remote Desktop Connection (mstsc): its address and the
// size of its screen on the command line, and its password in the
// credential store the client reads, under TERMSRV/<address>, as its own
// "Remember me" box does. It is not opened from an .rdp file: since
// Windows' April 2026 security update the client warns of every connection
// file it opens, signed or not, and a warning each time is of no use. Nor
// can a command line carry the color depth, the clipboard or the drives:
// those are the client's own settings.
func Launch(c Config) (Session, error) {
	cred := setCredential(c)
	proc, err := start("mstsc.exe", mstscArgs(c))
	if err != nil {
		cred.remove()
		return Session{}, err
	}
	return Session{Client: clientName, proc: proc, forget: cred.remove}, nil
}

// mstscArgs is the command line that opens c: its address, and the size of
// its screen, which is the client's own (full screen) when the connection
// names none.
func mstscArgs(c Config) []string {
	args := []string{"/v:" + c.Address()}
	if c.size() == "" {
		return append(args, "/f")
	}
	return append(args, fmt.Sprintf("/w:%d", c.Width), fmt.Sprintf("/h:%d", c.Height))
}
