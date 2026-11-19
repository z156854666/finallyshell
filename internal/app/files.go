package app

import (
	"cmp"
	"fmt"
	"path"
	"slices"
	"strings"

	"finallyshell/internal/remotefs"
	"finallyshell/internal/widgets"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// fileState is the file panel of a session.
type fileState struct {
	cwd      string
	pathEdit string

	// pathEditing shows the path as a field to type in, in place of its
	// crumbs; pathSelect selects the field's text once, as it opens.
	pathEditing bool
	pathSelect  bool

	entries []remotefs.Entry
	loading bool
	err     error
	table   ui.ListState
	chosen  ui.Selection[string] // names in cwd
	row     int
	// rowName is the file of that row, which the row follows as a sort
	// moves the rows.
	rowName string

	// sort is the sort a click on a header chose; the zero value keeps
	// the listing's order, folders first, then by name.
	sort ui.SortOrder
	// view is the rows shown: entries sorted as sort says, and what they
	// were made from.
	view   []remotefs.Entry
	viewOf []remotefs.Entry
	viewBy ui.SortOrder

	// reveal is a name to choose in the table once cwd is read again: a
	// file just renamed or made.
	reveal string

	tree     ui.OutlineState[string]
	treeRow  int
	subdirs  map[string][]string // the directories in each loaded directory
	treeBusy map[string]bool
}

type sftpError struct{ err error }

func (e *sftpError) Error() string { return "sftp: " + e.err.Error() }

type dirError struct {
	dir string
	err error
}

func (e *dirError) Error() string { return e.dir + ": " + e.err.Error() }

func fileErrorText(err error) string {
	switch e := err.(type) {
	case *sftpError:
		return locales.SftpUnavailable(e.err.Error())
	case *dirError:
		return locales.CannotOpenFolder(e.dir, e.err.Error())
	}
	return err.Error()
}

func (f *fileState) init() {
	f.subdirs = map[string][]string{}
	f.treeBusy = map[string]bool{}
	f.row = -1
	f.treeRow = -1
	f.tree.Open.Add("/")
}

// chosenNames are the names of the rows chosen in the table.
func (f *fileState) chosenNames() []string {
	var names []string
	for _, e := range f.entries {
		if f.chosen.Has(e.Name) {
			names = append(names, e.Name)
		}
	}
	return names
}

// openDir lists dir in the table and makes it the current directory.
func (a *App) openDir(s *Session, dir string) { a.listDir(s, dir, false) }

// listDir is openDir; quiet leaves the panel as it was, with no error,
// when dir can't be read, as when it follows the shell into a folder
// SFTP can't open.
func (a *App) listDir(s *Session, dir string, quiet bool) {
	if s.sftp == nil {
		return
	}
	dir = path.Clean(dir)
	f := &s.files
	f.loading = true
	if !quiet {
		f.err = nil
		f.pathEdit = dir
	}
	sc := s.sftp
	go func() {
		entries, err := remotefs.ReadDir(sc, dir)
		a.post(func() {
			f.loading = false
			if err != nil {
				if !quiet {
					f.err = &dirError{dir, err}
					f.pathEdit = f.cwd
				}
				return
			}
			f.err = nil
			if dir != f.cwd {
				f.chosen.Clear()
				f.row, f.rowName = -1, ""
			}
			f.cwd, f.pathEdit, f.entries = dir, dir, entries
			f.subdirs[dir] = dirNames(dir, entries)
			if f.reveal != "" {
				// Where the sort puts it, not where the listing does.
				view := f.shown()
				if i := slices.IndexFunc(view, func(e remotefs.Entry) bool { return e.Name == f.reveal }); i >= 0 {
					f.chosen.Clear()
					f.chosen.Add(f.reveal)
					f.row, f.rowName = i, f.reveal
					f.table.ScrollIntoView(i)
				}
				f.reveal = ""
			}
			// Open the tree down to the directory.
			for p := dir; ; p = path.Dir(p) {
				f.tree.Open.Add(p)
				if p == "/" {
					break
				}
			}
			for p := path.Dir(dir); p != "/" && f.subdirs[p] == nil; p = path.Dir(p) {
				a.loadSubdirs(s, p)
			}
			if dir != "/" && f.subdirs["/"] == nil {
				a.loadSubdirs(s, "/")
			}
		})
	}()
}

// refreshIfShowing lists dir again when the session shows it.
func (a *App) refreshIfShowing(s *Session, dir string) {
	if s.files.cwd == dir {
		a.openDir(s, dir)
	}
	delete(s.files.subdirs, dir)
}

// loadSubdirs reads the directories in dir, for the tree.
func (a *App) loadSubdirs(s *Session, dir string) {
	f := &s.files
	if s.sftp == nil || f.treeBusy[dir] {
		return
	}
	f.treeBusy[dir] = true
	sc := s.sftp
	go func() {
		entries, err := remotefs.ReadDir(sc, dir)
		a.post(func() {
			delete(f.treeBusy, dir)
			if err != nil {
				f.subdirs[dir] = []string{}
				return
			}
			f.subdirs[dir] = dirNames(dir, entries)
		})
	}()
}

func dirNames(dir string, entries []remotefs.Entry) []string {
	names := []string{}
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, path.Join(dir, e.Name))
		}
	}
	return names
}

func entryKind(e remotefs.Entry) string {
	switch {
	case e.IsDir():
		return locales.KindFolder()
	case e.Broken:
		return locales.KindBrokenLink()
	case e.Link:
		return locales.KindLink()
	}
	if ext := path.Ext(e.Name); len(ext) > 1 && len(ext) <= 8 && strings.Trim(ext[1:], "0123456789") != "" {
		return locales.KindExtFile(strings.ToUpper(ext[1:]))
	}
	return locales.KindFile()
}

// fileColumns are the table's columns; their IDs are the sort's.
func fileColumns() []ui.TableColumn {
	return []ui.TableColumn{
		{ID: "name", Title: locales.ColName(), MinWidth: 160, Sortable: true},
		{ID: "size", Title: locales.ColSize(), Width: 90, Align: ui.End, Sortable: true},
		{ID: "type", Title: locales.ColType(), Width: 90, Sortable: true},
		{ID: "modified", Title: locales.ColModified(), Width: 140, Sortable: true},
		{ID: "perms", Title: locales.ColPermissions(), Width: 100, Sortable: true},
		{ID: "owner", Title: locales.ColOwner(), Width: 96, Sortable: true},
	}
}

// sortEntries is entries as by says: folders first, as the listing and
// Explorer put them, then the column, ties by name. The zero by keeps the
// listing's order.
func sortEntries(entries []remotefs.Entry, by ui.SortOrder) []remotefs.Entry {
	if by.Column == "" {
		return entries
	}
	out := slices.Clone(entries)
	slices.SortStableFunc(out, func(x, y remotefs.Entry) int {
		if x.IsDir() != y.IsDir() {
			if x.IsDir() {
				return -1
			}
			return 1
		}
		var r int
		switch by.Column {
		case "size":
			r = cmp.Compare(x.Size, y.Size)
		case "type":
			r = cmp.Compare(strings.ToLower(entryKind(x)), strings.ToLower(entryKind(y)))
		case "modified":
			r = x.ModTime.Compare(y.ModTime)
		case "perms":
			r = strings.Compare(x.Mode.Perm().String(), y.Mode.Perm().String())
		case "owner":
			r = cmp.Or(cmp.Compare(x.UID, y.UID), cmp.Compare(x.GID, y.GID))
		default: // name
			r = cmp.Compare(strings.ToLower(x.Name), strings.ToLower(y.Name))
		}
		if by.Descending {
			r = -r
		}
		if r == 0 && by.Column != "name" {
			r = cmp.Compare(strings.ToLower(x.Name), strings.ToLower(y.Name))
			if by.Descending {
				r = -r
			}
		}
		return r
	})
	return out
}

// shown is the rows the table shows, made again when the listing or the
// sort changed.
func (f *fileState) shown() []remotefs.Entry {
	same := len(f.viewOf) == len(f.entries) && (len(f.entries) == 0 || &f.viewOf[0] == &f.entries[0])
	if !same || f.viewBy != f.sort {
		f.view = sortEntries(f.entries, f.sort)
		f.viewOf, f.viewBy = f.entries, f.sort
	}
	return f.view
}

// filePanel browses the active session's server over SFTP.
func (a *App) filePanel(c *ui.Context) {
	s := a.connSession()
	if s == nil || s.isNewTab() {
		return
	}
	if s.isLocal() || s.isRDP() {
		localUnavailable(c, s)
		return
	}
	f := &s.files
	ui.Column(c.Key(s.ID)).Grow(1).Children(func() {
		ui.Row(c).Padding(4, 8).Gap(4).BorderWidth(0, 0, 1, 0).BorderColor(colors.line).Children(func() {
			a.pathBar(c, s)
			if widgets.IconButton(c, "arrow-up", locales.ParentFolder()).Clicked() && f.cwd != "" {
				a.openDir(s, path.Dir(f.cwd))
			}
			if widgets.IconButton(c, "refresh-cw", locales.Refresh()).Clicked() && f.cwd != "" {
				a.openDir(s, f.cwd)
			}
			if widgets.IconButton(c, "house", locales.HomeFolder()).Clicked() && s.sftp != nil {
				if home, err := s.sftp.Getwd(); err == nil {
					a.openDir(s, home)
				}
			}
			a.followButton(c, s)
			ui.Box(c).Size(1, 16).Background(colors.line).Margin(0, 4)
			// Their context menus also have the archive transfers.
			uploadMenu := func(m *ui.Menu) { a.uploadMenuItems(m, s, f.cwd) }
			if widgets.IconButton(c, "upload", locales.UploadFiles()).ContextMenu(uploadMenu).Clicked() {
				a.pickAndUpload(s, f.cwd, false)
			}
			if widgets.IconButton(c, "folder-up", locales.UploadFolder()).ContextMenu(uploadMenu).Clicked() {
				a.pickAndUpload(s, f.cwd, true)
			}
			names := f.chosenNames()
			dl := widgets.IconButton(c, "download", locales.Download())
			dl.ContextMenu(func(m *ui.Menu) { a.downloadMenuItems(m, s, f.chosenNames()) })
			if len(names) == 0 {
				dl.Opacity(0.4)
			}
			if dl.Clicked() && len(names) > 0 {
				a.download(s, names, "")
			}
			if f.loading {
				ui.Spinner(c)
			}
			ui.Box(c).Size(1, 16).Background(colors.line).Margin(0, 4)
			a.downloadsButton(c)
		})
		if f.err != nil {
			ui.Text(c, fileErrorText(f.err)).TextColor(theme.Danger).Padding(4, 10)
		}
		split := ui.Split(c, &a.treeWidth, func() { a.dirTree(c, s) }, func() { a.fileTable(c, s) }).SplitReset(defaultTreeWidth).Grow(1)
		a.splitMoved(c, split, &a.treeWidth, treeLimits)
	})
}

// dropZone makes e take files dropped from the system's file manager, to
// upload them into dir, and outlines it while they are over it.
func (a *App) dropZone(e ui.Element, s *Session, dir string) {
	if files := e.DroppedFiles(); len(files) > 0 && dir != "" {
		a.upload(s, files, dir)
	}
	if e.FileDragOver() && dir != "" {
		e.Border(2, theme.Accent).Background(theme.Accent.Alpha(0.06))
	}
}

func (a *App) dirTree(c *ui.Context, s *Session) {
	f := &s.files
	f.tree.List.Selected = &f.treeRow
	tree := ui.Outline(c, &f.tree, []string{"/"}, func(dir string) []string {
		subs, loaded := f.subdirs[dir]
		if !loaded {
			if f.tree.Open.Has(dir) {
				a.loadSubdirs(s, dir)
			}
			return []string{}
		}
		if len(subs) == 0 {
			return nil
		}
		return subs
	}, func(dir string) {
		row := ui.Row(c).Gap(5).Grow(1).Radius(4)
		a.dropZone(row, s, dir)
		row.ContextMenu(func(m *ui.Menu) { a.treeMenu(c, m, s, dir) })
		row.Children(func() {
			name := path.Base(dir)
			// "/" is a folder like any other, not a workspace's root,
			// which themes give an icon of its own.
			a.fileIcon(c, name, true, f.tree.Open.Has(dir), false)
			t := ui.Text(c, name).SingleLine()
			if dir == f.cwd {
				t.Bold()
			}
		})
	}).Fill()
	if tree.Changed() {
		if dir := f.tree.Item(f.treeRow); dir != "" && dir != f.cwd {
			a.openDir(s, dir)
		}
	}
}

func (a *App) fileTable(c *ui.Context, s *Session) {
	f := &s.files
	entries := f.shown()
	// The choice follows its file as the sort moves the rows.
	f.row = slices.IndexFunc(entries, func(e remotefs.Entry) bool { return e.Name == f.rowName })
	f.table.Selected = &f.row
	f.table.Sort = &f.sort
	f.table.Selection = &f.chosen
	f.table.Key = func(i int) any { return entries[i].Name }
	f.table.Label = func(i int) string { return entries[i].Name }
	zone := ui.Column(c).Fill()
	a.dropZone(zone, s, f.cwd)
	over := zone.FileDragOver() && f.cwd != ""
	zone.Children(func() {
		t := ui.Table(c, &f.table, fileColumns(), len(entries), func(row, col int) {
			e := entries[row]
			switch col {
			case 0:
				// A chosen row keeps the table's text color, which shows on
				// the accent.
				chosen := f.chosen.Has(e.Name)
				ui.Row(c).Gap(6).Children(func() {
					a.entryIcon(c, e)
					name := ui.Text(c, e.Name).SingleLine()
					if e.Broken && !chosen {
						name.TextColor(theme.Danger)
					}
					if e.Target != "" {
						// As ls -l shows it; cut before the name.
						target := ui.Text(c, "→ "+e.Target).SingleLine().Shrink(1).MinWidth(0)
						if !chosen {
							target.TextColor(theme.TextMuted)
						}
					}
				})
			case 1:
				// A broken link has no file to measure.
				if !e.IsDir() && !e.Broken {
					ui.Text(c, formatSize(e.Size))
				}
			case 2:
				ui.Text(c, entryKind(e)).SingleLine()
			case 3:
				ui.Text(c, e.ModTime.Format("2006/01/02 15:04")).SingleLine()
			case 4:
				ui.Text(c, e.Mode.Perm().String()).Font("monospace")
			case 5:
				ui.Text(c, fmt.Sprintf("%d/%d", e.UID, e.GID))
			}
		}).Grow(1)
		if f.row >= 0 && f.row < len(entries) {
			f.rowName = entries[f.row].Name
		}
		// A double click or Enter opens a folder, or a file in the editor.
		if t.Submitted() && f.row >= 0 && f.row < len(entries) {
			if e := entries[f.row]; e.IsDir() {
				a.openDir(s, path.Join(f.cwd, e.Name))
			} else if editable(e) {
				a.openRemoteFile(s, path.Join(f.cwd, e.Name))
			}
		}
		// F2 renames the row chosen; Delete, or Cmd+Backspace as in
		// Finder, deletes those chosen.
		rename := f.table.Shortcut(c, 0, ui.KeyF2)
		del := f.table.Shortcut(c, 0, ui.KeyDelete)
		del = f.table.Shortcut(c, ui.Cmd, ui.KeyBackspace) || del
		if e, ok := f.entry(f.chosenNames()); ok && rename {
			a.askRename(s, path.Join(f.cwd, e.Name), e.IsDir())
		}
		if del && f.chosen.Len() > 0 {
			a.askDeleteChosen(s)
		}
		// Cmd+C copies the rows chosen, and Cmd+V pastes into the
		// folder, as in a file manager.
		if f.table.Shortcut(c, ui.Cmd, ui.KeyC) {
			a.copyChosen(c, s)
		}
		if f.table.Shortcut(c, ui.Cmd, ui.KeyV) {
			a.pasteInto(c, s, f.cwd)
		}
		t.ContextMenu(func(m *ui.Menu) {
			names := f.chosenNames()
			if e, ok := f.entry(names); ok && editable(e) {
				if m.Item(locales.Edit()).Chosen() {
					a.openRemoteFile(s, path.Join(f.cwd, e.Name))
				}
				m.Separator()
			}
			if m.Item(locales.Copy()).Shortcut(ui.Cmd, ui.KeyC).Disabled(len(names) == 0).Chosen() && len(names) > 0 {
				a.copyNames(c, s, f.cwd, names)
			}
			if m.Item(locales.Paste()).Shortcut(ui.Cmd, ui.KeyV).Disabled(!a.canPaste(c)).Chosen() {
				a.pasteInto(c, s, f.cwd)
			}
			m.Separator()
			a.downloadMenuItems(m, s, names)
			m.Separator()
			a.uploadMenuItems(m, s, f.cwd)
			m.Separator()
			if m.Item(locales.NewFolder()).Chosen() {
				a.askNew(s, f.cwd, opMkdir)
			}
			if m.Item(locales.NewFile()).Chosen() {
				a.askNew(s, f.cwd, opCreate)
			}
			m.Separator()
			e, one := f.entry(names)
			if m.Item(locales.Rename()).Shortcut(0, ui.KeyF2).Disabled(!one).Chosen() && one {
				a.askRename(s, path.Join(f.cwd, e.Name), e.IsDir())
			}
			if m.Item(locales.Delete()).Shortcut(0, ui.KeyDelete).Disabled(len(names) == 0).Chosen() && len(names) > 0 {
				a.askDeleteChosen(s)
			}
			if m.Item(locales.ChangePermissions()).Disabled(!one).Chosen() && one {
				a.askChmod(s, path.Join(f.cwd, e.Name), e.Mode)
			}
			m.Separator()
			if m.Item(locales.Refresh()).Chosen() {
				a.openDir(s, f.cwd)
			}
			if len(names) == 1 && m.Item(locales.CopyPath()).Chosen() {
				c.WriteClipboard(path.Join(f.cwd, names[0]))
			}
		})
		if over {
			ui.Text(c, locales.DropToUpload(f.cwd)).Padding(6, 12).Radius(6).
				Background(theme.Accent).TextColor(theme.AccentText).
				Attach(ui.AnchorBottom, ui.AnchorBottom).Bottom(12)
		}
	})
}
