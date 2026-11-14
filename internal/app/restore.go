package app

import (
	"io"

	"github.com/egoist/mygo/plugins/terminal"
)

// A session that connects again, as Reconnect does after its connection
// dropped, keeps what its terminals showed above what the new shells print,
// rather than starting with an empty screen: the terminal plugin's Snapshot
// gives a terminal's scrollback and screen as escape sequences, which Feed
// replays in the terminal that takes its place. The pane keeps the snapshot
// of the terminal it ends (keepShown), and the connection gives it to the
// terminal that replaces it (replay).

// paneKeep is what a terminal showed when the session connected again: its
// snapshot, and the size it was laid out at, which the terminal replaying it
// takes first.
type paneKeep struct {
	vt         []byte
	cols, rows int
}

// keepShown keeps what p's terminal shows, for the terminal that takes its
// place as the session connects again. It keeps nothing while p has no
// terminal, which would lose what an earlier reconnect kept and no terminal
// shows yet.
func (p *pane) keepShown() {
	if p.term == nil {
		return
	}
	cols, rows := p.term.Size()
	p.keep = &paneKeep{vt: p.term.Snapshot(), cols: cols, rows: rows}
}

// keepShown has each of s's panes keep what its terminal shows, for the
// terminals that take their places.
func (s *Session) keepShown() {
	for _, p := range s.panes.panes() {
		p.keepShown()
	}
}

// show makes term p's terminal, closing the one it takes the place of: the
// one the pane showed while the connection connected again.
func (p *pane) show(term *terminal.Terminal) {
	if p.term != nil && p.term != term {
		p.term.Close()
	}
	p.term = term
	p.keep = nil // term shows what it showed
}

// replay shows k, what a pane's terminal showed before, in term, the one that
// replaces it, laid out at the size it was, as a terminal attaching again to
// a session a server keeps does: what the shell prints comes after it.
func replay(term *terminal.Terminal, k *paneKeep) {
	if term == nil || k == nil || len(k.vt) == 0 {
		return
	}
	if k.cols > 0 && k.rows > 0 {
		term.Resize(k.cols, k.rows)
	}
	term.Feed(k.vt)
}

// heldConn holds what its terminal reads of a shell until the terminal is
// given what the pane showed before: a terminal reads its Conn from a
// goroutine as it is made, and the shell's output then comes after that.
type heldConn struct {
	io.ReadWriteCloser
	ready chan struct{}
}

func (c *heldConn) Read(p []byte) (int, error) {
	<-c.ready
	return c.ReadWriteCloser.Read(p)
}

// Resize is the Resize of what it holds, which the terminal tells the server's
// shell its size with. A Conn without one takes the size as told, as it
// would have without this.
func (c *heldConn) Resize(cols, rows int) error {
	if r, ok := c.ReadWriteCloser.(interface{ Resize(cols, rows int) error }); ok {
		return r.Resize(cols, rows)
	}
	return nil
}
