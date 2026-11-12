package app

import (
	"context"

	"finallyshell/internal/icons"
	"finallyshell/internal/rdp"
	"finallyshell/internal/widgets"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// A remote desktop session is a session of a host whose type is
// config.KindRDP, as FinalShell's RDP links are: the desktop itself is a
// window of the system's own RDP client (internal/rdp), which the app
// starts with the host's address, user, password and the settings of the
// connection's Remote desktop page.
//
// The tab shows where the desktop opened, with the buttons that open it
// again and edit the connection; closing the tab closes the desktop, as
// closing a shell's tab ends its shell. It has no shell, so the monitor,
// the files, the tunnels, the processes, the network and the command bar
// have nothing to read from it, and it never splits into panes.

// isRDP tells whether s is a remote desktop session; false for nil.
func (s *Session) isRDP() bool { return s != nil && s.Host != nil && s.Host.IsRDP() }

// connectRDP opens s's desktop in the system's client, on a goroutine:
// the client is started, not waited for, and it runs in a window of its
// own. A client this computer does not have fails the session, with the
// reason shown.
func (a *App) connectRDP(s *Session, secret string) {
	s.disconnect()
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel, s.ctx = cancel, ctx
	s.status, s.err = statusConnecting, nil
	h := s.Host
	cfg := rdp.For(h, secret)
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
			a.noteConnected(h)
			a.loggedIn(h)
		})
	}()
}

// rdpArea is the main area of a remote desktop session: the desktop it
// opened, and the buttons that open it again and edit the connection.
func (a *App) rdpArea(c *ui.Context, s *Session) {
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
