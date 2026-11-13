package app

import (
	"fmt"

	"finallyshell/internal/monitor"
	"finallyshell/internal/widgets"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// monitorPanel shows the active session's server, as FinalShell's left
// column does: the skeleton for a new tab. Its context menu chooses the
// parts that show (monitorparts.go). A local terminal has no server: a
// line says what it is.
func (a *App) monitorPanel(c *ui.Context) {
	s := a.connSession()
	if s != nil && s.isNewTab() {
		s = nil
	}
	ui.Scroll(c).Fill().ContextMenu(a.monitorMenu).Children(func() {
		if s.isLocal() {
			localMonitor(c)
			return
		}
		if s.isRDP() {
			rdpMonitor(c)
			return
		}
		a.serverView(c, s)
	})
}

// serverView is a server as the side panel shows it: its IP, the System
// info button, and its stats, without the parts the user hid. Without a
// session, or before the first stats, it is the skeleton FinalShell
// shows: dashes and empty bars.
func (a *App) serverView(c *ui.Context, s *Session) {
	ui.Column(c).FillWidth().Padding(10, 10, 8, 10).Gap(6).Children(func() {
		ui.Row(c).Gap(6).Children(func() {
			ip := "-"
			if s != nil {
				ip = s.ip
				if ip == "" {
					ip = s.Host.Addr
				}
			}
			ui.Text(c, "IP "+ip).SingleLine().Grow(1).MinWidth(0)
			if s != nil {
				cp := ui.Text(c, locales.Copy()).TextColor(theme.Accent).Cursor(ui.CursorPointer)
				if cp.Clicked() {
					c.WriteClipboard(ip)
				}
			}
		})
		a.sysInfoButton(c, s)
		var st *monitor.Stats
		switch {
		case s == nil:
		case s.status != statusConnected:
			widgets.Muted(c, s.statusText())
		case !execAllowed(s.Host):
			widgets.Muted(c, locales.MonitorOff())
		case !s.stats.OK:
			widgets.Muted(c, locales.ReadingServerInfo())
		default:
			st = &s.stats
		}
		serverStats(c, st, a.monitorHidden(), a.netPicker(s), a.openProcesses)
	})
}

// serverStats shows st, or the skeleton for nil, without the parts in
// hidden, or the hint to the panel's menu when every part is; net is the
// choice of network interfaces, and a click on the process list calls
// openProcs.
func serverStats(c *ui.Context, st *monitor.Stats, hidden hiddenParts, net netPicker, openProcs func()) {
	if hidden.none() {
		monitorHint(c)
		return
	}
	if st == nil {
		if hidden.shows(partUptime) {
			ui.Text(c, locales.UptimeUnknown())
			ui.Text(c, locales.LoadAverage("-"))
		}
		if hidden.shows(partCPU) {
			widgets.UsageBar(c, locales.Cpu(), 0, "0%", colors.cpu)
			widgets.UsageBar(c, locales.Memory(), 0, "0%", colors.mem)
			widgets.UsageBar(c, locales.Swap(), 0, "0%", colors.swap)
		}
		if hidden.shows(partProcesses) {
			procTable(c, nil, nil)
		}
		if hidden.shows(partNetwork) {
			netRow(c, 0, 0, "", "", nil)
			widgets.Sparkline(c, monitor.HistoryLen, nil, nil, formatBytes)
		}
		if hidden.shows(partLatency) {
			latencyRow(c, "-")
			widgets.Sparkline(c, monitor.HistoryLen, nil, nil, formatMillis)
		}
		if hidden.shows(partDisks) {
			diskTable(c, nil)
		}
		return
	}
	if hidden.shows(partUptime) {
		ui.Text(c, locales.UptimeDays(int(st.Uptime.Hours()/24)))
		ui.Text(c, locales.LoadAverage(fmt.Sprintf("%.2f, %.2f, %.2f", st.Load[0], st.Load[1], st.Load[2])))
	}
	if hidden.shows(partCPU) {
		widgets.UsageBar(c, locales.Cpu(), st.CPU/100, fmt.Sprintf("%.0f%%", st.CPU), colors.cpu)
		widgets.UsageBar(c, locales.Memory(), ratio(st.MemUsed, st.MemTotal), usage(st.MemUsed, st.MemTotal), colors.mem)
		widgets.UsageBar(c, locales.Swap(), ratio(st.SwapUsed, st.SwapTotal), usage(st.SwapUsed, st.SwapTotal), colors.swap)
	}
	if hidden.shows(partProcesses) {
		procTable(c, st.Procs, openProcs)
	}
	if hidden.shows(partNetwork) {
		netGraph(c, st, net)
	}
	if hidden.shows(partLatency) {
		latencyRow(c, formatLatency(st.Latency))
		widgets.Sparkline(c, monitor.HistoryLen, [][]float64{st.LatHist}, []ui.Color{latencyChart}, formatMillis)
	}
	if hidden.shows(partDisks) {
		diskTable(c, st.Disks)
	}
}

func formatMillis(v float64) string { return fmt.Sprintf("%.0f", v) }

// netRow is the speeds above the network graph and, at the right, label:
// the button of the interfaces' menu when there is a menu.
func netRow(c *ui.Context, tx, rx float64, label, tip string, menu func(m *ui.Menu)) {
	ui.Row(c).Gap(8).Margin(4, 0, 0, 0).Children(func() {
		ui.Text(c, "↑"+formatBytes(tx)).TextColor(colors.tx).Shrink(0)
		ui.Text(c, "↓"+formatBytes(rx)).TextColor(colors.rx).Shrink(0)
		ui.Spacer(c)
		if menu != nil {
			netLabelButton(c, label, tip, menu)
		} else {
			widgets.Muted(c, label)
		}
	})
}

func latencyRow(c *ui.Context, latency string) {
	ui.Row(c).Gap(8).Margin(4, 0, 0, 0).Children(func() {
		ui.Text(c, latency).TextColor(theme.Accent)
		ui.Spacer(c)
		widgets.Muted(c, locales.Latency())
	})
}

func ratio(a, b uint64) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}

func usage(used, total uint64) string {
	return fmt.Sprintf("%.0f%%  %s/%s", 100*ratio(used, total), formatBytes(float64(used)), formatBytes(float64(total)))
}

// procTable is the processes using the most memory; a click opens the
// process manager with open, when it is not nil.
func procTable(c *ui.Context, procs []monitor.Proc, open func()) {
	headText := colors.procHeadText
	table := ui.Column(c).Border(1, colors.frame).Background(colors.procTable)
	if open != nil {
		table.Cursor(ui.CursorPointer).Tooltip(locales.OpenProcesses()).Role(ui.RoleButton).Label(locales.OpenProcesses())
		if table.Clicked() {
			open()
		}
	}
	table.Children(func() {
		ui.Row(c).Background(colors.procHead).PaddingX(6).Height(20).Children(func() {
			ui.Text(c, locales.Memory()).Width(54).TextColor(headText).FontSize(12)
			ui.Text(c, locales.Cpu()).Width(40).TextColor(headText).FontSize(12)
			ui.Text(c, locales.Command()).TextColor(headText).FontSize(12)
		})
		for i, p := range procs {
			ui.Row(c.Key(i)).PaddingX(6).Height(19).Children(func() {
				ui.Text(c, formatBytes(float64(p.RSS))).Width(54).FontSize(12)
				ui.Text(c, fmt.Sprintf("%.1f", p.CPU)).Width(40).FontSize(12)
				ui.Text(c, p.Name).FontSize(12).SingleLine().Grow(1).MinWidth(0)
			})
		}
	})
}

func diskTable(c *ui.Context, disks []monitor.Disk) {
	ui.Column(c).Margin(6, 0, 0, 0).Border(1, colors.frame).Background(colors.diskTable).Children(func() {
		ui.Row(c).PaddingX(6).Height(20).BorderWidth(0, 0, 1, 0).BorderColor(colors.line).Children(func() {
			ui.Text(c, locales.DiskPath()).Grow(1).FontSize(12)
			ui.Text(c, locales.DiskFreeSize()).FontSize(12)
		})
		for i, d := range disks {
			ui.Row(c.Key(i)).PaddingX(6).Height(19).Gap(6).Children(func() {
				ui.Text(c, d.Mount).FontSize(12).SingleLine().Grow(1).MinWidth(0)
				ui.Text(c, formatBytes(float64(d.Avail))+"/"+formatBytes(float64(d.Size))).FontSize(12)
			})
		}
	})
}

// connectionPanel is the left panel under the server monitor: its header,
// and the saved connections in their folders (conntree.go).
func (a *App) connectionPanel(c *ui.Context) {
	ui.Column(c).Fill().BorderWidth(1, 0, 0, 0).BorderColor(colors.frame).Children(func() {
		a.connectionHeader(c)
		a.connectionPage(c)
	})
}
