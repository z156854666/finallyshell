package app

import (
	"strings"
	"testing"
	"time"

	"finallyshell/locales"
)

// Reconnecting keeps what the panes showed, above what the new shells
// print: an empty screen would lose the output the user was reading when
// the connection dropped.
func TestReconnectKeepsOutput(t *testing.T) {
	a, p, s, tt, srv := paneHost(t, followHome(t), false)
	press(tt, a, scSplitRight)
	untilPanes(t, p, s, 2)
	before := s.panes.panes()
	const first, second = "kept: the first pane", "kept: the second pane"
	before[0].term.Feed([]byte(first + "\r\n"))
	before[1].term.Feed([]byte(second + "\r\n"))

	srv.DropConnections()
	p.until(t, "the session lost", func() bool { return s.status != statusConnected })
	settle(p, 300*time.Millisecond) // the other panes' ends
	if s.panes.count() != 2 {
		t.Fatalf("%d panes after the drop", s.panes.count())
	}
	if err := tt.Click(locales.Reconnect()); err != nil {
		t.Fatalf("%v; texts %q", err, tt.Texts())
	}
	p.until(t, "connected again", func() bool {
		if s.hostKeyQ != nil {
			s.hostKeyQ.Answer <- true
			s.hostKeyQ = nil
		}
		return s.status != statusConnecting
	})
	if s.status != statusConnected {
		t.Fatalf("status %d, err %v", s.status, s.err)
	}
	for i, want := range []string{first, second} {
		q := s.panes.panes()[i]
		if q.term == nil {
			t.Fatalf("pane %d has no terminal", i+1)
		}
		if got := q.term.Text(); !strings.Contains(got, want) {
			t.Errorf("pane %d shows %q, without %q", i+1, got, want)
		}
	}
}
