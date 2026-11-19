package app

import (
	"path"
	"strings"

	"finallyshell/internal/icons"
	"finallyshell/internal/widgets"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// crumb is a folder in the path bar: its name as shown, and the path a
// click opens.
type crumb struct {
	name string
	dir  string
}

// pathCrumbs splits dir into the crumbs of its folders, the root first.
func pathCrumbs(dir string) []crumb {
	dir = path.Clean(dir)
	crumbs := []crumb{{name: "/", dir: "/"}}
	if dir == "/" || dir == "." {
		return crumbs
	}
	if !strings.HasPrefix(dir, "/") {
		// Not a server path, as a local folder in a test: one crumb.
		return []crumb{{name: dir, dir: dir}}
	}
	p := "/"
	for _, name := range strings.Split(strings.TrimPrefix(dir, "/"), "/") {
		p = path.Join(p, name)
		crumbs = append(crumbs, crumb{name: name, dir: p})
	}
	return crumbs
}

// fitCrumbs takes the crumbs to show in width: the deepest that fit, and
// more when some were left out, which the bar shows as an ellipsis.
func fitCrumbs(c *ui.Context, crumbs []crumb, width float32) (shown []crumb, more bool) {
	if width <= 0 || len(crumbs) <= 1 {
		return crumbs, false
	}
	size := c.Theme().FontSize
	cost := func(cr crumb, deepest bool) float32 {
		w, _ := c.MeasureText(0, ui.Span{Text: cr.name, Size: size})
		w += 10 // the room around the name
		if !deepest {
			w += 15 // its chevron and the bar's gaps
		}
		return w
	}
	ellipsis, _ := c.MeasureText(0, ui.Span{Text: "…", Size: size})
	budget := width - (ellipsis + 10 + 15)
	i, used := len(crumbs)-1, float32(0)
	for ; i >= 0; i-- {
		cr := cost(crumbs[i], i == len(crumbs)-1)
		if used+cr > budget {
			break
		}
		used += cr
	}
	switch {
	case i < 0:
		return crumbs, false
	case i == len(crumbs)-1:
		// Not even the deepest fits: it is the one shown, cut by the bar.
		return crumbs[i:], true
	}
	return crumbs[i+1:], true
}

// editPath shows the path as a field, its text selected, to type another.
func (a *App) editPath(f *fileState) {
	f.pathEdit = f.cwd
	f.pathEditing = true
	f.pathSelect = true
}

// pathBar is the panel's path, the field of crumbs: a click on an
// ancestor crumb opens its folder; a click on the current folder, past
// the crumbs, or on the ellipsis shows the whole path to type, as the
// field it replaces did.
func (a *App) pathBar(c *ui.Context, s *Session) {
	f := &s.files
	if f.pathEditing || f.cwd == "" {
		a.pathField(c, s)
		return
	}
	t := c.Theme()
	// The crumbs sit in the field the typed path was; the field's room
	// past them opens that path to type.
	field := ui.Box(c).Grow(1).MinWidth(0).Row().Clip().
		Padding(t.Space(1.5), t.Space(2.5)).Radius(t.Radius).
		Background(t.Surface).Border(1, t.Border).Cursor(ui.CursorText).
		Role(ui.RoleButton).Label(locales.EditPath()).Tooltip(locales.EditPath()).
		OnClick(func() { a.editPath(f) })
	field.Children(func() {
		row := ui.Row(c).Grow(1).MinWidth(0).Gap(1)
		width := row.Bounds().W
		shown, more := fitCrumbs(c, pathCrumbs(f.cwd), width)
		// nameLimit is the room the deepest crumb's name gets, which it
		// keeps its end in: the rest of the bar, past its chevron and the
		// ellipsis of the folders left out.
		nameLimit := float32(0)
		if width > 40 {
			nameLimit = width - 30
			if more {
				ellipsis, _ := c.MeasureText(0, ui.Span{Text: "…", Size: t.FontSize})
				nameLimit = max(nameLimit-(ellipsis+25), 24)
			}
		}
		row.Children(func() {
			chevron := func() {
				ui.Box(c).Width(12).Center().Shrink(0).Children(func() {
					ui.Icon(c, icons.Get("chevron-right")).FontSize(11).TextColor(t.TextMuted)
				})
			}
			seg := func(key, name, label string, color ui.Color, current bool) ui.Element {
				b := ui.Box(c.Key(key)).PaddingY(1).PaddingX(5).Radius(4).Shrink(0).Center().
					Role(ui.RoleButton).Label(label).Tooltip(label).Cursor(ui.CursorPointer)
				if b.Hovered() {
					b.Background(t.Text.Alpha(0.07))
				}
				b.Children(func() {
					text := ui.Text(c, name).FontSize(t.FontSize).SingleLine()
					if current {
						text.Bold()
					}
					text.TextColor(color)
				})
				return b
			}
			if more {
				// The folders left out open the field, as the rest of
				// it does, to see and type the path.
				if seg("path-more", "…", locales.EditPath(), t.TextMuted, false).Clicked() {
					a.editPath(f)
				}
				chevron()
			}
			for i, cr := range shown {
				if i > 0 {
					chevron()
				}
				current := i == len(shown)-1
				color := t.TextMuted
				name := cr.name
				if current {
					color = t.Text
					// The folder the panel is in keeps room for its end,
					// which tells it from the others.
					if nameLimit > 0 {
						name = widgets.MiddleEllipsis(c, name, nameLimit, t.FontSize)
					}
				}
				if seg(cr.dir, name, cr.dir, color, current).Clicked() {
					if current {
						// The folder the panel is in is the path to
						// type: a click on it shows the path, as one
						// past the crumbs does.
						a.editPath(f)
					} else {
						a.openDir(s, cr.dir)
					}
				}
			}
		})
	})
}

// pathField is the path bar as a field to type a path in.
func (a *App) pathField(c *ui.Context, s *Session) {
	f := &s.files
	field := ui.TextInput(c, &f.pathEdit).Grow(1)
	if f.pathEditing {
		// Editing is what the user asked for, and the field takes the
		// keyboard as it opens; a path still empty waits for the first
		// listing with the focus where it was.
		field.AutoFocus()
	}
	if f.pathSelect {
		field.SetTextSelection(0, len([]rune(f.pathEdit)))
		f.pathSelect = false
	}
	switch {
	case field.Submitted():
		f.pathEditing = false
		a.openDir(s, f.pathEdit)
	case field.Shortcut(0, ui.KeyEscape), field.FocusLeft():
		f.pathEdit, f.pathEditing = f.cwd, false
	}
}
