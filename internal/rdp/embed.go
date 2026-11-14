package rdp

import "errors"

// ErrNoEmbed is what Embed fails with on a system that cannot draw a
// remote desktop in a window of the app's: every system but Windows,
// whose Remote Desktop Connection brings a control for it.
var ErrNoEmbed = errors.New("no remote desktop control")

// ErrEnded is what an embedded desktop reports once its connection ended,
// as the server closed it.
var ErrEnded = errors.New("remote desktop ended")

// EndError is a desktop whose connection ended: the reason code the control
// gave it, and the control's words for it, which are "" when it has none.
// It is an ErrEnded, whatever the reason.
type EndError struct {
	Reason int32
	Text   string
}

// Error is the control's words for the end, or ErrEnded's.
func (e EndError) Error() string {
	if e.Text != "" {
		return e.Text
	}
	return ErrEnded.Error()
}

// Is matches ErrEnded: a desktop that ended with a reason is one that ended.
func (e EndError) Is(target error) bool { return target == ErrEnded }

// Rect is where a desktop shows in the app's window: the box of the tab's
// area, in DIPs, as ui.Element.Bounds gives it.
type Rect struct{ X, Y, W, H float32 }

// Desktop is a remote desktop drawn inside the app's own window rather
// than in a window of the system's client: Remote Desktop Connection's
// control, which draws the desktop, takes the keyboard, the mouse and the
// clipboard itself, as mstsc's window does. The tab shows it where its
// area is, and closing the tab ends the connection.
//
// A Desktop belongs to the thread Embed ran on, the app's UI thread,
// which Windows' control, as every COM object of a single-threaded
// apartment, wants its calls on.
type Desktop interface {
	// Place shows the desktop at r, the box the tab draws for it, and
	// moves it there: the app calls it every frame, with the box of the
	// frame before.
	Place(r Rect)
	// Hide takes the desktop out of sight while its tab is not the one
	// shown; Show shows it where Place put it.
	Hide()
	Show()
	// State is where the connection is: connected once the desktop shows,
	// and why it is not, while it connects, when it failed and once it
	// ended.
	State() (connected bool, err error)
	// Close ends the connection and takes the desktop out of the window,
	// as closing mstsc's window does.
	Close()
}
