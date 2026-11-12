package rdp

import "os"

// Session is a desktop a client opened: the client's name, as the app
// shows it, the process this package started for it and what the launch
// left behind to forget. proc is nil for a client that opened the desktop
// itself, as Microsoft Remote Desktop does, which Close therefore leaves
// running.
type Session struct {
	Client string
	proc   *os.Process
	forget func() // what to forget with the desktop, nil for nothing
}

// Close ends the desktop when this package started the client; one the
// client opened on its own stays. What the launch wrote for the client,
// as its password in Windows' credential store, goes with it.
func (s Session) Close() {
	if s.proc != nil {
		s.proc.Kill()
	}
	if s.forget != nil {
		s.forget()
	}
}
