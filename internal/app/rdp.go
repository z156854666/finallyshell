package app

import (
	"context"
	"time"

	"finallyshell/internal/icons"
	"finallyshell/internal/rdp"
	"finallyshell/internal/widgets"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// A remote desktop session is a session of a host whose type is
// config.KindRDP, as FinalShell's RDP links are. The desktop is drawn by an
// RDP client: the tab's own, Windows' Remote Desktop control (internal/rdp),
// which is the default, or the system's client (internal/rdp too), which
// the settings ask for (rdpInWindow) and which the app starts with the
// host's address, user, password and the settings of the connection's
// Remote desktop page, in a window of its own.
//
// The tab shows the desktop, or where the system's client opened it, with
// the buttons that open it again and edit the connection; closing the tab
// closes the desktop, as closing a shell's tab ends its shell. It has no
// shell, so the monitor, the files, the tunnels, the processes, the network
// and the command bar have nothing to read from it, and it never splits
// into panes.

// rdpTickEvery is how often a tab showing an embedded desktop has a frame
// built, which follows the connection: Windows' control connects and ends
// on its own, and the tab says so.
const rdpTickEvery = time.Second

// rdpEmbeds tells whether the system can draw a remote desktop inside a
// window of the app's: Windows, whose control does (rdp.EmbedSupported).
// Tests set it on the systems that have no control.
var rdpEmbeds = rdp.EmbedSupported()

// isRDP tells whether s is a remote desktop session; false for nil.
func (s *Session) isRDP() bool { return s != nil && s.Host != nil && s.Host.IsRDP() }

// rdpInTab tells whether a remote desktop opens in its tab, which it does
// unless the settings say otherwise, and the system has the control that
// draws one there.
func (a *App) rdpInTab() bool { return !a.settings.RDPInWindow && rdpEmbeds }

// connectRDP opens s's desktop, on a goroutine: in the tab, in Windows'
// control, or else in the system's client, started and not waited for,
// which runs in a window of its own. A client this computer does not have
// fails the session, with the reason shown.
func (a *App) connectRDP(s *Session, secret string) {
	s.disconnect()
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel, s.ctx = cancel, ctx
	s.status, s.err = statusConnecting, nil
	cfg := rdp.For(s.Host, secret)
	if a.rdpInTab() {
		a.embedRDP(s, ctx, cfg)
		return
	}
	launch := a.launchRDP
	if launch == nil {
		launch = rdp.Launch
	}
	go func() {
		desk, err := launch(cfg)
		a.post(func() {
			if ctx.Err() != nil { // closed while opening
				if err == nil {
					desk.Close()
				}
				return
			}
			if err != nil {
				s.status, s.err = statusFailed, err
				return
			}
			s.desk = &desk
			s.status, s.err = statusConnected, nil
			a.noteConnected(s.Host)
			a.loggedIn(s.Host)
		})
	}()
}

// embedRDP opens cfg in s's tab, on a goroutine: the control is made on
// the UI thread, as the platform wants, and draws the desktop in the tab's
// area from the next frame on (placeDesktops). It connects on its own
// afterwards, which rdpTick follows.
func (a *App) embedRDP(s *Session, ctx context.Context, cfg rdp.Config) {
	open := a.embedDesktop
	if open == nil {
		open = a.plat.EmbedRDP
	}
	go func() {
		desk, err := open(cfg)
		a.post(func() {
			if ctx.Err() != nil { // closed while opening
				if err == nil {
					desk.Close()
				}
				return
			}
			if err != nil {
				s.status, s.err = statusFailed, err
				return
			}
			s.embed = desk
			a.rdpTick(s)
		})
	}()
}

// rdpTick follows the connection of s's embedded desktop, as the tab shows
// it: the connection ends while the tab is open, as the server closes it or
// the login fails, and the tab says why.
func (a *App) rdpTick(s *Session) {
	if s.embed == nil {
		return
	}
	connected, err := s.embed.State()
	switch {
	case err != nil:
		s.status, s.err = statusFailed, err
		s.closeEmbed()
	case connected && s.status != statusConnected:
		s.status, s.err = statusConnected, nil
		a.noteConnected(s.Host)
		a.loggedIn(s.Host)
	case !connected && s.status == statusConnected:
		s.status, s.err = statusFailed, rdp.ErrEnded
		s.closeEmbed()
	}
}

// placeDesktops shows each embedded desktop where its tab draws it, and
// hides the ones whose tab is not shown: the desktop's control covers the
// tab's area, and the tab paints it every frame.
func (a *App) placeDesktops() {
	cur := a.current()
	for _, s := range a.sessions {
		if s.embed == nil {
			continue
		}
		// A tab that has not drawn its place yet, in the frame after the
		// one that opened the desktop, is placed by the frame after it:
		// the tab asks for one (embeddedArea).
		if s != cur || s.deskRect.W < 1 || s.deskRect.H < 1 {
			s.embed.Hide()
			continue
		}
		s.embed.Place(s.deskRect)
	}
}

// rdpArea is the main area of a remote desktop session: the desktop it
// opened, or, where the desktop is the system's client's, the buttons that
// open it again and edit the connection.
func (a *App) rdpArea(c *ui.Context, s *Session) {
	s.deskRect = rdp.Rect{}
	if s.embed != nil {
		a.rdpTick(s)
	}
	if s.embed != nil {
		a.embeddedArea(c, s)
		return
	}
	a.clientArea(c, s)
}

// embeddedArea is the main area of a desktop drawn in the tab: the place
// the control draws over, which is the whole area from the first frame. A
// banner above it would make the desktop connect to a smaller area, and the
// control draws its own progress over the place anyway; what makes the
// connection fail is said by the tab once the control is gone (rdpTick).
func (a *App) embeddedArea(c *ui.Context, s *Session) {
	ui.Column(c).Key(s.ID).Fill().Background(theme.Background).Children(func() {
		// Windows' control draws over this box, so the tab paints nothing
		// of it but its box, which places the control while the frames
		// after this one follow (Bounds is the previous frame's).
		place := ui.Row(c).Key("rdp-desktop").Grow(1).MinHeight(0).MinWidth(0)
		if b := place.Bounds(); b.W > 0 && b.H > 0 {
			s.deskRect = rdp.Rect{X: b.X, Y: b.Y, W: b.W, H: b.H}
		}
	})
	// The control connects and ends on its own: a frame now and then says
	// where it is (rdpTick).
	c.After(rdpTickEvery)
}

// clientArea is the main area of a desktop opened in a window of the
// system's client: where it opened, and the buttons that open it again and
// edit the connection.
func (a *App) clientArea(c *ui.Context, s *Session) {
	ui.Column(c).Key(s.ID).Fill().Background(theme.Background).Children(func() {
		if s.status != statusConnected {
			a.statusBanner(c, s)
		}
		ui.Column(c).Grow(1).Center().Gap(12).Padding(0, 24).Children(func() {
			ui.Icon(c, icons.Get("monitor")).FontSize(44).TextColor(theme.TextMuted)
			ui.Text(c, s.label()).FontSize(16).Bold()
			widgets.Muted(c, s.target())
			switch s.status {
			case statusConnecting:
				ui.Spinner(c)
				ui.Text(c, locales.ConnectingTo(s.target()))
			case statusConnected:
				if s.desk != nil && s.desk.Client != "" {
					widgets.Muted(c, locales.RdpOpenedIn(s.desk.Client)).TextAlign(ui.Center).MaxWidth(540)
				}
				widgets.Muted(c, locales.RdpTabNote()).TextAlign(ui.Center).MaxWidth(540).FontSize(12)
			}
			ui.Row(c).Gap(8).Children(func() {
				if ui.Button(c, locales.RdpOpenAgain()).Disabled(s.status == statusConnecting).Clicked() {
					a.reconnect(s)
				}
				if ui.Button(c, locales.EditConnection()).Clicked() {
					a.editConnection(s.Host)
				}
			})
		})
	})
}

// rdpMonitor is the server monitor's place for a remote desktop, which
// has no server to read: a muted line, no stats, no polling.
func rdpMonitor(c *ui.Context) {
	ui.Row(c).FillWidth().Padding(10, 10, 8, 10).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, icons.Get("monitor")).FontSize(14).TextColor(theme.TextMuted)
		widgets.Muted(c, locales.RemoteDesktop()).SingleLine().MinWidth(0).Shrink(1)
	})
}
