package app

import (
	"slices"
	"strconv"

	"finallyshell/internal/icons"
	"finallyshell/internal/widgets"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// The session tabs are the bar at the top of the main area, as FinalShell's
// are: a tab for each session, new tabs and panel rows included, numbered in
// their order, with the session's status dot and its name. A click selects a
// tab, which shows in the terminal's background with an accent line under
// it; the tabs scroll sideways where they do not fit, and the one selected
// comes into view as it changes. The "+" after the last tab opens a new one.
//
// A tab has the context menu the row of a vertical list would have, a double
// click reconnects a session that is not connected, F2 renames it while the
// tabs have the keyboard focus, its close button shows on the selected tab
// and the one under the pointer, and the tabs can be dragged into another
// order (moveTab).

// sessionTabsHeight is the height of the bar, in DIPs, and sessionTabHeight
// the height of a tab in it.
const (
	sessionTabsHeight = 36
	sessionTabHeight  = 28
)

// sessionTabs is the bar of the session tabs, over the main area.
func (a *App) sessionTabs(c *ui.Context) {
	bar := ui.Row(c.Key("session-tabs")).FillWidth().Height(sessionTabsHeight).Shrink(0).
		PaddingX(4).Background(colors.sidebar).BorderColor(colors.line).BorderWidth(0, 0, 1, 0).
		AlignItems(ui.Center)
	bar.Children(func() {
		// The tabs, and the buttons after them, scroll sideways where they
		// do not fit, as FinalShell's do; the wheel scrolls them too
		// (sessionTabsWheel).
		ui.ScrollHorizontal(c).Grow(1).MinWidth(0).Scrollbars(false).
			TrackScroll(&a.sessionTabsScroll).HandleInput(a.sessionTabsWheel).Children(func() {
			ui.Row(c).Gap(2).AlignItems(ui.Center).Children(func() {
				a.sessionTabList(c)
				a.sessionTabButton(c)
			})
		})
	})
}

// sessionTabList is the tabs themselves, in the sessions' order, and the
// keys they take while they have the focus: F2 renames the selected one.
func (a *App) sessionTabList(c *ui.Context) {
	tabs := ui.TabsBase(c, &a.active, len(a.sessions))
	// A tab asks to close sessions, its own or the other tabs', which close
	// once the tabs are built: closing one in the tab would leave the tabs
	// after it to build with fewer sessions than the list was given.
	var closing tabClosing
	var selected ui.Element
	hasSelected := false
	tabs.List.Role(ui.RoleList).Gap(2).AlignItems(ui.Center).Children(func() {
		for i, s := range a.sessions {
			tab := tabs.Tab(i)
			if i == a.active {
				selected, hasSelected = tab, true
			}
			a.sessionTab(c, tab, s, i, &closing)
		}
	})
	// The tab selected last, as the keys or a new session chose it, comes
	// into view: the tabs may have scrolled away from it.
	if hasSelected && a.tabsShown != a.active {
		selected.ScrollIntoView(ui.ScrollIntoViewOptions{})
	}
	a.tabsShown = a.active
	// The keys go back to the selected tab, as after a rename.
	if a.focusTabs {
		a.focusTabs = false
		if hasSelected {
			selected.Focus()
		}
	}
	// The terminal keeps F2 for its programs: only the tabs rename.
	if tabs.List.Shortcut(0, ui.KeyF2) && a.rename.s == nil {
		if s := a.current(); s != nil {
			a.startRename(s)
		}
	}
	for _, s := range closing.rows {
		a.closeSession(s)
	}
	// The tab whose menu closed the others stays selected, as it would be
	// had the user clicked it.
	if s := closing.selected; s != nil {
		if i := slices.Index(a.sessions, s); i >= 0 {
			a.active = i
		}
	}
}

// sessionCount is the number of sessions, local terminals included, new
// tabs and panel rows left out.
func (a *App) sessionCount() int {
	n := 0
	for _, s := range a.sessions {
		if !s.isNewTab() {
			n++
		}
	}
	return n
}

// tabClosing is what the tabs ask to close as they build: the rows to close
// once the tabs are built, and the tab to select then, if one.
type tabClosing struct {
	rows     []*Session
	selected *Session
}

// add asks to close rows.
func (t *tabClosing) add(rows ...*Session) { t.rows = append(t.rows, rows...) }

// sessionTab is the tab of s, at row i of the sessions, which adds what it
// asks to close to closing.
func (a *App) sessionTab(c *ui.Context, tab ui.Element, s *Session, i int, closing *tabClosing) {
	active := i == a.active
	// An item of the tabs' list, rather than a leaf tab: assistive
	// technology then reaches the logo of the agent running in the session,
	// the panes, and the button that closes the tab. Its accessible name is
	// the name of the session, as the tab shows it.
	row := tab.Role(ui.RoleGroup).Label(listLabel(s)).AccessSelected(active).
		Height(sessionTabHeight).PaddingX(8).Radius(6).Gap(6).
		AlignItems(ui.Center).Shrink(0).MaxWidth(260).Cursor(ui.CursorPointer)
	hovered := row.Hovered()
	switch {
	case active:
		row.Background(theme.Background)
	case hovered:
		row.Background(theme.SurfaceHover)
	}
	if active {
		row.DrawOver(func(p *ui.Painter, r ui.Rect) {
			in, h := theme.Space(2), theme.Space(0.5)
			p.Fill(ui.Rect{X: r.X + in, Y: r.Y + r.H - h, W: r.W - 2*in, H: h}, theme.Accent, h/2)
		})
	}
	a.sessionTabMenu(row, s, closing)
	a.sessionTabDrag(row, s)
	// DoubleClicked, as the tab is clickable itself: it is what takes the
	// press, and the tab list chooses the session on the click.
	renaming := a.renaming(s)
	if row.DoubleClicked() && !renaming && !s.isPanelTab() && s.status != statusConnected && s.status != statusConnecting {
		a.reconnect(s)
	}
	switch {
	case s.isPanelTab():
		row.Tooltip(panelLabel(s))
	case s.name != "" && !renaming:
		row.Tooltip(renamedTooltip(s))
	}
	row.Children(func() {
		// Its place among the tabs, as FinalShell numbers them: the order
		// the keys that choose a tab by number go by.
		num := ui.Text(c, strconv.Itoa(i+1)).FontSize(11).Shrink(0)
		if active {
			num.TextColor(theme.AccentText.Alpha(0.8))
		} else {
			num.TextColor(theme.TextMuted)
		}
		switch {
		case s.isPanelTab():
			icon := "app-window"
			if s.panelTab >= 0 && s.panelTab < len(panelTabIcons) {
				icon = panelTabIcons[s.panelTab]
			}
			ui.Icon(c, icons.Get(icon)).FontSize(14).Shrink(0)
		case s.isNewTab():
			// As wide as a status dot, so that the names line up.
			ui.Box(c).Size(8, 8).Center().Shrink(0).Children(func() {
				ui.Icon(c, icons.Get("app-window")).FontSize(13).TextColor(tabSubColor(active))
			})
		default:
			sessionMark(c, s, active)
		}
		name := func() {
			if s.isPanelTab() {
				tabName(c, panelTabName(s), active)
				return
			}
			if s.isNewTab() {
				tabName(c, locales.NewTab(), active)
				return
			}
			a.sessionName(c, s, active)
		}
		if renaming {
			a.renameField(c, active)
		} else {
			name()
		}
		closeBtn := tabCloseButton(c, s)
		if !hovered && !active {
			closeBtn.Invisible()
		}
		if closeBtn.Clicked() {
			closing.add(s)
		}
	})
}

// tabCloseButton is the button that closes s's tab, at its right end: a
// key of its own, as every tab has one. The tab reads by the name of its
// session alone, which a close button in it would otherwise be read with;
// the tab's context menu closes it too. It takes nearly the tab's height, so
// that it is easy to press.
func tabCloseButton(c *ui.Context, s *Session) ui.Element {
	b := ui.Box(c.Key("session-tab-close-"+strconv.FormatInt(s.ID, 10))).Size(22, 22).Center().Radius(5).
		Role(ui.RoleButton).Label(locales.CloseSession()).Tooltip(locales.CloseSession()).AccessHidden(true)
	if b.Hovered() {
		b.Background(theme.Text.Alpha(0.07))
	}
	b.Children(func() {
		ui.Icon(c, icons.Get("x")).FontSize(14).TextColor(theme.TextMuted)
	})
	return b
}

// tabNameText is the name of a tab, bold while the tab is selected.
func tabNameText(c *ui.Context, name string, active bool) ui.Element {
	t := ui.Text(c, name).SingleLine()
	if active {
		t.Bold()
	}
	return t
}

// tabName is the name of a tab that is no session's: a new tab's, or a
// panel row's, cut short in a long one.
func tabName(c *ui.Context, name string, active bool) {
	tabNameText(c, name, active).MaxWidth(200)
}

// tabSubColor is the color of the icon of a tab that has no status dot: the
// accent while it is selected.
func tabSubColor(active bool) ui.Color {
	if active {
		return theme.AccentText.Alpha(0.8)
	}
	return theme.TextMuted
}

// sessionTabMenu is the context menu of s's tab: reconnect, the panes,
// rename, close, and the items that close several tabs at once.
func (a *App) sessionTabMenu(row ui.Element, s *Session, closing *tabClosing) {
	row.ContextMenu(func(m *ui.Menu) {
		if s.isPanelTab() {
			if m.Item(locales.BackToTerminal()).Chosen() {
				a.backToTerminal(s)
			}
			m.Separator()
			if m.Item(locales.Close()).Danger().Chosen() {
				closing.add(s)
			}
			m.Separator()
			a.closeTabsMenu(m, s, closing)
			return
		}
		if s.isNewTab() { // a panel row has no host either, and is no new tab
			if m.Item(locales.Close()).Danger().Chosen() {
				closing.add(s)
			}
			m.Separator()
			a.closeTabsMenu(m, s, closing)
			return
		}
		if m.Item(locales.Reconnect()).Chosen() {
			a.reconnect(s)
		}
		if m.Item(locales.DuplicateSession()).Chosen() {
			// In a tab of its own, though a new tab is selected.
			if s.isLocal() {
				a.openLocal(nil)
			} else {
				a.openSessionIn(nil, s.Host)
			}
		}
		if m.Item(locales.Disconnect()).Disabled(s.status != statusConnected && s.status != statusConnecting).Chosen() {
			a.transfers.cancelSession(s)
			// What the terminals show stays for the connection that
			// takes their places (restore.go).
			s.keepShown()
			s.disconnect()
			s.status, s.err = statusClosed, nil
		}
		m.Separator()
		a.paneMenu(m, s)
		m.Separator()
		a.renameMenu(m, s)
		m.Separator()
		if m.Item(locales.Close()).Danger().Chosen() {
			closing.add(s)
		}
		m.Separator()
		a.closeTabsMenu(m, s, closing)
	})
}

// closeTabsMenu adds the items of a tab's menu that close several tabs: the
// other tabs, and the tabs to the right. Each is off while it has nothing to
// close.
func (a *App) closeTabsMenu(m *ui.Menu, s *Session, closing *tabClosing) {
	if others := a.otherTabs(s); m.Item(locales.CloseOtherTabs()).Disabled(len(others) == 0).Chosen() {
		closing.add(others...)
		// The tab the menu opened on stays, and selected, as it would be
		// had the user clicked it.
		closing.selected = s
	}
	if right := a.tabsRight(s); m.Item(locales.CloseTabsRight()).Disabled(len(right) == 0).Chosen() {
		closing.add(right...)
	}
}

// otherTabs returns the rows that closing the other tabs of s's closes: every
// tab but s's own, as "close other tabs" offers. A panel row stays with its
// session, so the panel row of s's session stays too, as does the session of
// a panel row the menu opened on.
func (a *App) otherTabs(s *Session) []*Session {
	keep := s
	if s.isPanelTab() {
		keep = s.panelOf
	}
	panel := a.panelRowOf(keep)
	var rows []*Session
	for _, r := range a.sessions {
		if r == s || r == keep || r == panel {
			continue
		}
		rows = append(rows, r)
	}
	return rows
}

// tabsRight returns the rows after s's tab, leaving out the panel row of its
// session where that lies after it.
func (a *App) tabsRight(s *Session) []*Session {
	i := slices.Index(a.sessions, s)
	if i < 0 {
		return nil
	}
	keep := s
	if s.isPanelTab() {
		keep = s.panelOf
	}
	panel := a.panelRowOf(keep)
	var rows []*Session
	for _, r := range a.sessions[i+1:] {
		if r == keep || r == panel {
			continue
		}
		rows = append(rows, r)
	}
	return rows
}

// sessionTabDrag makes s's tab a value to drag into another tab's place, and
// a place for one: dropping a tab on another moves it before that one, with
// its panel row, as dragging a row of a list does.
func (a *App) sessionTabDrag(row ui.Element, s *Session) {
	row.Drag(s)
	if _, over := ui.DragOver[*Session](row); over {
		row.Border(1, theme.Accent)
	}
	if d, ok := ui.Drop[*Session](row); ok {
		a.moveTab(d, s)
	}
}

// moveTab puts s, and its panel row, where at is.
func (a *App) moveTab(s, at *Session) {
	i, j := slices.Index(a.sessions, s), slices.Index(a.sessions, at)
	if i < 0 || j < 0 || i == j {
		return
	}
	a.moveSessions([]int{i}, j)
}

// sessionTabButton is the "+" after the last tab: it opens a new tab, from a
// button as high as the bar leaves room for, easier to press than the small
// ones.
func (a *App) sessionTabButton(c *ui.Context) {
	ui.Row(c).Shrink(0).Padding(0, 6).AlignItems(ui.Center).Children(func() {
		if widgets.IconButton(c, "plus", locales.NewTab()).Size(32, sessionTabsHeight-4).Clicked() {
			a.NewTab()
		}
	})
}

// sessionTabsWheel has the vertical wheel, or a vertical swipe of a
// touchpad, over the session tabs scroll them sideways while they do not
// fit: down toward the last tab. Sideways scrolling, and any while they fit,
// goes on to the container as it would without it.
func (a *App) sessionTabsWheel(ev ui.InputEvent) bool {
	s := &a.sessionTabsScroll
	if ev.Kind != ui.InputScroll || ev.DX != 0 || ev.DY == 0 || s.MaxX <= 0 || ev.Mods&ui.Ctrl != 0 {
		return false
	}
	s.X = min(max(s.X+ev.DY, 0), s.MaxX)
	return true
}
