package app

import (
	"testing"

	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// TestNewConnectionChooser asks what to add first, as FinalShell does,
// and Cancel adds nothing.
func TestNewConnectionChooser(t *testing.T) {
	a, p := testApp(t)
	tt := ui.NewTester(a.View, 1280, 820)
	p.tt = tt
	tt.Frame()
	a.newConnection()
	q := p.show(t, WindowNewConnection)
	if !q.HasText(locales.NewSSHConnection()) || !q.HasText(locales.NewRDPConnection()) || !q.HasText(locales.Cancel()) {
		t.Errorf("texts %q", q.Texts())
	}
	saveShot(t, q, "new-connection-en.png")
	if err := q.Click(locales.Cancel()); err != nil {
		t.Fatal(err)
	}
	if a.windowOpen(WindowNewConnection) || a.editor != nil || a.windowOpen(WindowEditor) {
		t.Errorf("chooser %v, editor %v", a.windowOpen(WindowNewConnection), a.editor != nil)
	}

	// From the manager it is a child of it, as the editor it opens is.
	a.openManager()
	if !a.windowOpen(WindowManager) {
		t.Fatal("manager not open")
	}
	a.newConnectionIn(&a.manager, "")
	if !a.windowOpen(WindowNewConnection) {
		t.Fatal("no chooser from the manager")
	}
	if spec := p.windows[WindowNewConnection].spec; spec.Parent != WindowManager || spec.Modal {
		t.Errorf("chooser %+v", spec)
	}
	if err := p.show(t, WindowNewConnection).Click(locales.NewRDPConnection()); err != nil {
		t.Fatal(err)
	}
	spec := p.windows[WindowEditor].spec
	if spec.Title() != locales.NewRemoteDesktop() || spec.Parent != WindowManager || !spec.Modal {
		t.Errorf("desktop's editor %+v", spec)
	}
}
