package app

import (
	"slices"
	"strconv"
	"strings"

	"finallyshell/internal/config"
	"finallyshell/internal/icons"
	"finallyshell/internal/widgets"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// The left panel under the server monitor is Connections, the saved hosts
// in their folders, as MobaXterm's session tree and Xshell's Sessions pane
// keep them in the main window: the connection manager's tree
// (manager.go), with its search, menus and dragging, in one column: the
// names, a dot on the hosts with a session open, and the rest in a
// tooltip. Opening a host there opens a session as the start page does,
// and the panel stays, to open the next. The open sessions are the tabs
// over the terminal (sessiontabs.go).
//
// Both trees read a.hosts, so a change in one shows in the other at once.
// Each has its own managerState: what is open, chosen and searched, and
// its questions, which this panel asks in the main window. It never shows
// the deleted hosts, so it asks before deleting one, and has neither the
// manager's filter nor its sort.
//
// The folders open are kept in the settings (connectionTreeExpanded), on
// this device only: by their IDs, which stay as they are renamed and moved.

// leftPanel is what the left panel keeps besides its tree.
type leftPanel struct {
	// focusSearch gives the panel's search the keyboard in the next frame
	// it shows.
	focusSearch bool
	// saved are the IDs of the folders open, as the settings have them.
	saved []string
}

// connIndent is how far each level of the tree is indented, in DIPs.
const connIndent = 14

// restoreLeftPanel opens the folders the settings keep: every folder
// before the user opened or closed any.
func (a *App) restoreLeftPanel() {
	open := &a.connTree.tree.Open
	if ids := a.settings.ConnectionTreeExpanded; ids != nil {
		for _, id := range *ids {
			open.Add(managerItem{folder: id})
		}
	} else {
		for _, f := range a.hosts.Folders {
			open.Add(managerItem{folder: f.ID})
		}
	}
	a.left.saved = a.openFolders()
}

// openFolders are the IDs of the folders open in the page's tree, each
// after the one it is in.
func (a *App) openFolders() []string {
	out := []string{}
	for _, f := range a.allFolders() {
		if a.connTree.tree.Open.Has(managerItem{folder: f.ID}) {
			out = append(out, f.ID)
		}
	}
	return out
}

// keepFoldersOpen saves the folders open in the page's tree as they
// change: not while searching, as the search opens those holding what it
// finds, and closes them again once it is cleared.
func (a *App) keepFoldersOpen() {
	if a.connTree.searching {
		return
	}
	ids := a.openFolders()
	if slices.Equal(ids, a.left.saved) {
		return
	}
	a.left.saved = ids
	a.settings.ConnectionTreeExpanded = &ids
	a.saveSettings(a.settings)
}

// switchLeftPanel is the shortcut's: the connections panel's search takes
// the keyboard, leaving full screen first to show it.
func (a *App) switchLeftPanel() {
	if a.fullScreen.on {
		a.setFullScreen(false) // to show it
	}
	a.left.focusSearch = true
}

// connectionHeader is the panel's header: its name, the options button and
// the one that adds a connection.
func (a *App) connectionHeader(c *ui.Context) {
	ui.Row(c).Padding(8, 6, 6, 8).Gap(2).AlignItems(ui.Center).Children(func() {
		ui.Text(c, locales.ConnectionsPage()).Bold().SingleLine().Grow(1).MinWidth(0)
		if headerButton(c, "settings", locales.Options()).Clicked() {
			a.openOptions()
		}
		if headerButton(c, "plus", locales.NewConnection()).Clicked() {
			m := &a.connTree
			a.newConnectionIn(m, a.treeTarget(m))
		}
	})
}

// headerButton is a button of the left panel's header: an icon button a
// little narrower than the others, so that the header keeps its buttons in
// a sidebar of the default width.
func headerButton(c *ui.Context, icon, label string) ui.Element {
	return widgets.IconButton(c, icon, label).Width(20)
}

// connectionPage is the panel's search over the tree.
func (a *App) connectionPage(c *ui.Context) {
	m := &a.connTree
	var later []func() // what changes the tree, once it is built
	ui.Column(c).Grow(1).MinHeight(0).Gap(4).Padding(0, 4, 4, 4).Children(func() {
		search := ui.SearchField(c, &m.query).Placeholder(locales.SearchConnections()).Label(locales.SearchConnections()).
			FillWidth().Margin(0, 2)
		if a.left.focusSearch {
			search.Focus()
			a.left.focusSearch = false
		}
		if search.Changed() {
			a.searchChanged(m)
		}
		if search.Shortcut(0, ui.KeyDown) {
			m.focus = true
		}
		if search.Submitted() {
			a.connectFound(m)
		}
		a.connTreeView(c, m, &later)
		if m.saveErr != nil {
			ui.Text(c, m.saveErr()).TextColor(theme.Danger).FontSize(12).Padding(0, 6)
		}
	})
	for _, fn := range later {
		fn()
	}
	a.keepFoldersOpen()
}

// connTreeView is the panel's tree: one column, with its own arrows (the
// Prefix of its rows), so
// that the whole row takes the menu and the drops.
func (a *App) connTreeView(c *ui.Context, m *managerState, later *[]func()) {
	t := a.newManagerTree(m)
	choose := a.treeFrame(c, m, t)
	over := managerItem{}
	drop := a.treeDropZone(m, later, &over)
	var prefix ui.OutlineRow
	m.tree.Prefix = func(r ui.OutlineRow) { prefix = r }
	tree := ui.Outline(c, &m.tree, t.roots(), t.children, func(it managerItem) {
		a.connTreeRow(c, m, it, prefix, drop, later)
	}).Grow(1).MinHeight(0)
	drop(tree, managerItem{}, "")
	m.over = over
	tree.Children(func() {
		if t.search && len(t.roots()) == 0 {
			widgets.Muted(c, locales.NoMatchingHosts()).Padding(8, 10)
		} else if len(a.hosts.Live()) == 0 && len(a.hosts.Folders) == 0 {
			widgets.Muted(c, locales.ConnTreeEmpty()).Padding(8, 10)
		}
	})
	a.newItemsMenu(tree, m, "", later)
	a.finishTree(c, m, tree, choose, later)
}

// connTreeRow is the row of it, a folder or a host, as deep and open as
// r says.
func (a *App) connTreeRow(c *ui.Context, m *managerState, it managerItem, r ui.OutlineRow, drop func(ui.Element, managerItem, string), later *[]func()) {
	var f *config.Folder
	var h *config.Host
	into := ""
	if it.isHost() {
		if h = a.hosts.Host(it.host); h == nil {
			return
		}
		into = a.folderOf(h)
	} else {
		if f = a.hosts.Folder(it.folder); f == nil {
			return
		}
		into = f.ID
	}
	color, muted := theme.Text, theme.TextMuted
	if m.isChosen(it) {
		color, muted = theme.AccentText, theme.AccentText.Alpha(0.75)
	}
	row := ui.Row(c).Grow(1).MinWidth(0).AlignSelf(ui.Stretch).Gap(5).AlignItems(ui.Center).Padding(4, 8, 4, 4).Radius(4)
	if it == m.over {
		row.Background(theme.Accent.Alpha(0.14))
	}
	drop(row, it, into)
	// A list choosing one row does not choose it on a right-click: the
	// menu acts on the row it was opened on, chosen for it.
	row.ContextMenu(func(menu *ui.Menu) {
		*later = append(*later, func() {
			if i := m.rowOfItem(it); i >= 0 {
				m.row = i
			}
		})
		a.managerItemMenu(c, m, menu, it, later)
	})
	if h != nil {
		row.Tooltip(hostTooltip(h))
	}
	row.Children(func() {
		if r.Depth > 0 {
			ui.Box(c).Width(float32(r.Depth) * connIndent).Shrink(0)
		}
		arrow := r.Arrow().Size(16, 16).Center()
		if r.Branch {
			icon := "chevron-right"
			if r.Open {
				icon = "chevron-down"
			}
			arrow.Children(func() {
				ui.Icon(c, icons.Get(icon)).FontSize(13).TextColor(muted)
			})
		}
		if f != nil {
			ui.Icon(c, icons.Get("folder")).FontSize(15).TextColor(folderColor)
			ui.Text(c, f.Name).SingleLine().MinWidth(0).TextColor(color)
			return
		}
		ui.Icon(c, icons.Get(hostIcon(h))).FontSize(14).TextColor(color.Alpha(0.75))
		ui.Text(c, h.Title()).SingleLine().MinWidth(0).TextColor(color)
		if dot, ok := a.sessionDot(h); ok {
			widgets.StatusDot(c, dot)
		}
	})
}

// hostTooltip is the tooltip of a host's row: where it connects, and the
// first line of its notes.
func hostTooltip(h *config.Host) string {
	s := h.User + "@" + h.Addr + ":" + strconv.Itoa(h.PortOr22())
	if notes, _, _ := strings.Cut(h.Notes, "\n"); strings.TrimSpace(notes) != "" {
		s += "\n" + notes
	}
	return s
}

// sessionDot is the color of the dot of h's row: the color of its
// sessions' dots on the session tabs, of one connected, else of one
// connecting. ok is false when none is.
func (a *App) sessionDot(h *config.Host) (color ui.Color, ok bool) {
	for _, s := range a.sessions {
		if s.Host == nil || s.Host.ID != h.ID { // a new tab, a local terminal
			continue
		}
		switch s.status {
		case statusConnected:
			return s.statusColor(theme), true
		case statusConnecting:
			color, ok = s.statusColor(theme), true
		}
	}
	return color, ok
}
