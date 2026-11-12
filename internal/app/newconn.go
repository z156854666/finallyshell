package app

import (
	"finallyshell/internal/config"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// WindowNewConnection is the small window New connection opens first: it
// asks what to add, an SSH connection or a Windows remote desktop, as
// FinalShell's New connection does, and the editor of that kind follows.
const WindowNewConnection WindowKind = "newConnection"

// newConnectionSpec is the window of the chooser: the two kinds of
// connection, and Cancel.
var newConnectionSpec = WindowSpec{
	Kind:  WindowNewConnection,
	Title: locales.NewConnection,
	Width: 380, Height: 165,
	MinWidth: 320, MinHeight: 150,
}

// newConnChoice is what the chooser holds while it is open: the window
// the connection is added from, the folder it goes in, and the tree that
// shows it once saved.
type newConnChoice struct {
	from   WindowKind
	folder string
	tree   *managerState
}

// newConnection asks what kind of connection to add, from the main
// window, as FinalShell's New connection does.
func (a *App) newConnection() { a.chooseConnection(WindowMain, "", nil) }

// chooseConnection opens the chooser of the kinds, from the window of
// kind from, for a connection in folder, which tree shows the host once
// saved: nil for the manager's.
func (a *App) chooseConnection(from WindowKind, folder string, tree *managerState) {
	if a.windowOpen(WindowEditor) {
		a.plat.FocusWindow(WindowEditor) // one editor at a time
		return
	}
	a.newConn = &newConnChoice{from: from, folder: folder, tree: tree}
	spec := newConnectionSpec
	if from != WindowMain && a.windowOpen(from) {
		// A child of the manager, as the editor it opens is.
		spec.Parent = from
	}
	spec.Closed = func() { a.newConn = nil }
	a.openWindow(spec, a.newConnectionView)
}

// newConnectionOfKind closes the chooser and opens the editor of kind,
// which adds a host.
func (a *App) newConnectionOfKind(kind config.Kind) {
	q := a.newConn
	a.newConn = nil
	a.closeWindow(WindowNewConnection)
	if q == nil {
		q = &newConnChoice{from: WindowMain}
	}
	a.openEditorAs(nil, kind, q.from, q.folder, q.tree)
}

// newConnectionView is the chooser: the two kinds of connection, and
// Cancel.
func (a *App) newConnectionView(c *ui.Context) {
	ui.Column(c).Fill().Padding(16).Gap(10).Children(func() {
		if ui.Button(c, locales.NewSSHConnection()).FillWidth().Clicked() {
			a.newConnectionOfKind(config.KindSSH)
		}
		if ui.Button(c, locales.NewRDPConnection()).FillWidth().Clicked() {
			a.newConnectionOfKind(config.KindRDP)
		}
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if ui.Button(c, locales.Cancel()).Clicked() {
				a.newConn = nil
				a.closeWindow(WindowNewConnection)
			}
		})
	})
}
