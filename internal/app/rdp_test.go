package app

import (
	"runtime"
	"strings"
	"testing"

	"finallyshell/internal/config"
	"finallyshell/internal/rdp"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// rdpHost is a saved remote desktop connection.
func rdpHost() *config.Host {
	return &config.Host{ID: "h1", Name: "win", Addr: "10.0.0.7", User: "Administrator",
		Auth: config.AuthPassword, Type: config.KindRDP,
		RDP: &config.RDPConfig{Domain: "CORP", Width: 1280, Height: 800}}
}

// rdpLauncher stands in for the system's RDP client, which a test does
// not start: it keeps the connection it was asked to open.
func rdpLauncher(t *testing.T, c *rdp.Config) func(rdp.Config) (rdp.Session, error) {
	t.Helper()
	return func(cfg rdp.Config) (rdp.Session, error) {
		*c = cfg
		return rdp.Session{Client: "Test Client"}, nil
	}
}

// fakeEmbedded stands in for the system's remote desktop control, which a
// test does not make: it keeps the connections it was asked to open, and
// what the app did with each.
type fakeEmbedded struct {
	cfg   rdp.Config
	desks []*fakeDesktop
	err   error
	opens int
}

func (p *fakeEmbedded) EmbedRDP(cfg rdp.Config) (rdp.Desktop, error) {
	if p.err != nil {
		return nil, p.err
	}
	p.cfg, p.opens = cfg, p.opens+1
	d := &fakeDesktop{connected: true}
	p.desks = append(p.desks, d)
	return d, nil
}

// desk is the desktop opened last, nil for none.
func (p *fakeEmbedded) desk() *fakeDesktop {
	if len(p.desks) == 0 {
		return nil
	}
	return p.desks[len(p.desks)-1]
}

// fakeDesktop is a desktop drawn in a tab: where the app placed it, and
// whether it shows.
type fakeDesktop struct {
	places    []rdp.Rect
	shown     bool
	closed    bool
	connected bool
	err       error
}

func (d *fakeDesktop) Place(r rdp.Rect) { d.places = append(d.places, r); d.shown = true }
func (d *fakeDesktop) Hide()            { d.shown = false }
func (d *fakeDesktop) Show()            { d.shown = true }
func (d *fakeDesktop) Close()           { d.closed = true }

func (d *fakeDesktop) State() (bool, error) { return d.connected, d.err }

// place is where the app put the desktop last.
func (d *fakeDesktop) place() rdp.Rect {
	if len(d.places) == 0 {
		return rdp.Rect{}
	}
	return d.places[len(d.places)-1]
}

// TestRDPSession opens a remote desktop: the client is started with the
// host's address, login and settings, and the tab shows where it opened.
func TestRDPSession(t *testing.T) {
	a, p := testApp(t)
	a.settings.RDPInWindow = true // the client's window, as the settings may ask
	var got rdp.Config
	a.launchRDP = rdpLauncher(t, &got)
	h := rdpHost()
	a.hosts.Add(h)
	a.passwords[h.ID] = "pw"
	a.openSession(h)
	s := a.current()
	p.until(t, "the desktop to open", func() bool { return s.status == statusConnected })

	want := rdp.Config{Host: "10.0.0.7", Port: 3389, User: "Administrator", Domain: "CORP", Password: "pw",
		Width: 1280, Height: 800, Clipboard: true, Drives: true}
	if got != want {
		t.Errorf("opened %+v, want %+v", got, want)
	}
	if s.desk == nil || s.desk.Client != "Test Client" || s.term() != nil {
		t.Errorf("desk %+v", s.desk)
	}
	if s.target() != "Administrator@10.0.0.7" {
		t.Errorf("target %q", s.target())
	}

	tt := ui.NewTester(a.View, 1280, 820)
	p.tt = tt
	tt.Frame()
	for _, text := range []string{locales.RdpOpenedIn("Test Client"), locales.RdpTabNote(), locales.RemoteDesktop()} {
		if !tt.HasText(text) {
			t.Errorf("no %q; texts %q", text, tt.Texts())
		}
	}
	// No bottom panel, and no command bar: a desktop has no shell.
	if tt.HasText(locales.TabFiles()) || tt.HasText(locales.TabCommands()) {
		t.Errorf("the desktop showed the bottom panel; texts %q", tt.Texts())
	}
	saveShot(t, tt, "rdp-session-en.png")

	// Open again opens it again, as Reconnect does.
	got = rdp.Config{}
	if err := tt.Click(locales.RdpOpenAgain()); err != nil {
		t.Fatal(err)
	}
	p.until(t, "the desktop to open again", func() bool { return s.status == statusConnected && got.Password == "pw" })
	if got.Host != "10.0.0.7" {
		t.Errorf("opened again %+v", got)
	}
}

// TestRDPInTab opens a remote desktop in its tab, where the settings say so
// and the system has the control: no client of the system's starts, the
// desktop is placed in the tab's area while the tab shows, and closing the
// tab closes it.
func TestRDPInTab(t *testing.T) {
	a, p := testApp(t)
	was := rdpEmbeds
	rdpEmbeds = true
	t.Cleanup(func() { rdpEmbeds = was })
	a.launchRDP = func(rdp.Config) (rdp.Session, error) {
		t.Fatal("the system's client started a desktop that belongs in its tab")
		return rdp.Session{}, nil
	}
	h := rdpHost()
	a.hosts.Add(h)
	a.passwords[h.ID] = "pw"
	tt := ui.NewTester(a.View, 1280, 820)
	p.tt = tt
	tt.Frame()
	a.openSession(h)
	s := a.current()
	p.until(t, "the desktop to open", func() bool { return s.embed != nil })
	d := p.desk()
	if d == nil {
		t.Fatal("no desktop")
	}
	want := rdp.Config{Host: "10.0.0.7", Port: 3389, User: "Administrator", Domain: "CORP", Password: "pw",
		Width: 1280, Height: 800, Clipboard: true, Drives: true}
	if p.cfg != want {
		t.Errorf("opened %+v, want %+v", p.cfg, want)
	}
	// It draws where the tab draws it, which takes a frame to know.
	p.until(t, "the desktop to be placed", func() bool { return d.place().W > 0 })
	if r := d.place(); r.H <= 0 || r.W < 200 {
		t.Errorf("placed at %+v", r)
	}
	if s.status != statusConnected {
		t.Errorf("status %v, error %v", s.status, s.err)
	}
	if s.desk != nil {
		t.Errorf("a desktop of the system's client: %+v", s.desk)
	}
	if tt.HasText(locales.RdpOpenedIn("Test Client")) {
		t.Errorf("the tab shows a client's desktop; texts %q", tt.Texts())
	}

	// Its tab alone shows it: another tab takes it out of sight, and back.
	a.NewTab()
	p.until(t, "the desktop to hide", func() bool { return !d.shown })
	a.active = 0
	p.until(t, "the desktop to show", func() bool { return d.shown })

	// A desktop that ends says so, and leaves the tab to the client's
	// place: the buttons open it again.
	d.connected = false
	p.until(t, "the desktop to end", func() bool { return s.status == statusFailed })
	if describeError(s.err) != locales.RdpEnded() || !d.closed || s.embed != nil {
		t.Errorf("error %q, closed %v, desktop %v", describeError(s.err), d.closed, s.embed)
	}
	if !tt.HasText(locales.RdpEnded()) || !tt.HasText(locales.RdpOpenAgain()) {
		t.Errorf("texts %q", tt.Texts())
	}

	// Closing the tab closes the desktop too.
	if err := tt.Click(locales.RdpOpenAgain()); err != nil {
		t.Fatal(err)
	}
	p.until(t, "the desktop to open again", func() bool { return s.embed != nil && p.opens == 2 })
	s.disconnect()
	if !p.desk().closed {
		t.Error("the desktop stayed open after its tab closed")
	}
}

// TestRDPNoClient fails the session on a computer without an RDP client,
// saying so, and offers Reconnect.
func TestRDPNoClient(t *testing.T) {
	a, p := testApp(t)
	a.settings.RDPInWindow = true // the client's window, as the settings may ask
	a.launchRDP = func(rdp.Config) (rdp.Session, error) { return rdp.Session{}, rdp.ErrNoClient }
	h := rdpHost()
	a.hosts.Add(h)
	a.passwords[h.ID] = "pw"
	a.openSession(h)
	s := a.current()
	p.until(t, "the failure", func() bool { return s.status == statusFailed })
	if describeError(s.err) != locales.ErrNoRDPClient() {
		t.Errorf("error %q", describeError(s.err))
	}
	tt := ui.NewTester(a.View, 1280, 820)
	p.tt = tt
	tt.Frame()
	if !tt.HasText(locales.ErrNoRDPClient()) || !tt.HasText(locales.Reconnect()) {
		t.Errorf("texts %q", tt.Texts())
	}
}

// TestRDPEditor makes a remote desktop connection: New connection asks
// the kind, and a desktop opens in a window of its own, its page alone,
// with its fields, the port of RDP, and what is saved.
func TestRDPEditor(t *testing.T) {
	a, p := testApp(t)
	saved := savedHosts(a)
	tt := newRDPEditor(t, a, p)
	if a.editor.kind != config.KindRDP || a.editor.port != "3389" {
		t.Fatalf("kind %q, port %q", a.editor.kind, a.editor.port)
	}
	// Its window is its page: no tree of pages, and no shell's fields.
	if spec := p.windows[WindowEditor].spec; spec.Title() != locales.NewRemoteDesktop() || spec.Width != 520 {
		t.Errorf("spec %+v, %q", spec, spec.Title())
	}
	note := locales.RdpClientNote() // what opens it, where it takes the rest
	if runtime.GOOS == "windows" {
		note = locales.RdpWindowsNote()
	}
	if !tt.HasText(locales.SectionScreen()) || !tt.HasText(note) ||
		!tt.HasText(locales.SectionGeneral()) || !tt.HasText(locales.FieldDomain()) {
		t.Errorf("texts %q", tt.Texts())
	}
	// Windows' client sets the colors, the clipboard and the drives
	// itself: the page shows those where a client takes them.
	if shells := runtime.GOOS != "windows"; shells != tt.HasText(locales.FieldClipboard()) ||
		shells != tt.HasText(locales.FieldRDPDepth()) {
		t.Errorf("client's own settings: %v; texts %q", shells, tt.Texts())
	}
	if tt.HasText(locales.EditorPageSSH()) || tt.HasText(locales.EditorPageTerminal()) || tt.HasText(locales.MonitorChannel()) {
		t.Errorf("a shell's page in a desktop's window; texts %q", tt.Texts())
	}
	if port := fieldValue(t, tt, locales.FieldPort()); port != "3389" {
		t.Errorf("port %q", port)
	}
	saveShot(t, tt, "editor-rdp-en.png")
	typeInto(t, tt, locales.FieldName(), "win")
	typeInto(t, tt, locales.FieldHost(), "10.0.0.7")
	typeInto(t, tt, locales.FieldUser(), "Administrator")
	typeInto(t, tt, locales.FieldPassword(), "pw")
	typeInto(t, tt, locales.FieldDomain(), "CORP")

	// A size, and no clipboard, down the page: a desktop's fields take
	// more room than the window's.
	tt.Scroll(340, 300, 0, 200)
	if err := tt.Click(locales.RdpClientSize()); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("1920 × 1080"); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if err := tt.Click(locales.FieldClipboard()); err != nil {
			t.Fatal(err)
		}
	}
	if a.editor.rdp.Width != 1920 || a.editor.rdp.Height != 1080 || a.editor.rdp.NoDrives {
		t.Fatalf("remote desktop %+v", a.editor.rdp)
	}

	if err := tt.Click(locales.Ok()); err != nil {
		t.Fatal(err)
	}
	if a.windowOpen(WindowEditor) {
		t.Fatal("editor still open")
	}
	hosts := a.hosts.Live()
	if len(hosts) != 1 || !hosts[0].IsRDP() || hosts[0].Port != 3389 || hosts[0].Type != config.KindRDP {
		t.Fatalf("hosts %+v", hosts)
	}
	o := hosts[0].RDPOf()
	// The clipboard is left to Windows' client there, and unchecked
	// elsewhere.
	if o.Domain != "CORP" || o.Width != 1920 || o.Height != 1080 || o.NoDrives || o.NoClipboard != (runtime.GOOS != "windows") {
		t.Errorf("saved %+v", o)
	}
	if a.passwords[hosts[0].ID] != "pw" {
		t.Errorf("password %q", a.passwords[hosts[0].ID])
	}
	if !strings.Contains(string(*saved), `"type":"rdp"`) ||
		runtime.GOOS != "windows" && !strings.Contains(string(*saved), `"noClipboard":true`) {
		t.Errorf("saved %s", *saved)
	}

	// Editing it again opens the desktop's window, titled so.
	a.editConnection(hosts[0])
	if a.editor.kind != config.KindRDP || a.editor.port != "3389" || a.editor.rdp.Width != 1920 {
		t.Fatalf("editor %+v", a.editor)
	}
	if err := p.show(t, WindowEditor).Click(locales.Cancel()); err != nil {
		t.Fatal(err)
	}
	a.editConnection(hosts[0])
	tt = editorWindow(t, a, p)
	if spec := p.windows[WindowEditor].spec; spec.Title() != locales.EditRemoteDesktop() || !tt.HasText(locales.SectionScreen()) {
		t.Errorf("title %q; texts %q", spec.Title(), tt.Texts())
	}
}

// newRDPEditor opens the chooser New connection opens and chooses a
// remote desktop, returning its editor's window.
func newRDPEditor(t *testing.T, a *App, p *fakePlatform) *ui.Tester {
	t.Helper()
	a.newConnection()
	q := p.show(t, WindowNewConnection)
	if err := q.Click(locales.NewRDPConnection()); err != nil {
		t.Fatal(err)
	}
	if a.windowOpen(WindowNewConnection) || !a.windowOpen(WindowEditor) || a.editor == nil {
		t.Fatalf("chooser %v, editor %v", a.windowOpen(WindowNewConnection), a.windowOpen(WindowEditor))
	}
	return editorWindow(t, a, p)
}

// TestRDPEditorError is an error of a desktop's fields shown in the
// desktop's window, which holds the address and the user, not on a
// shell's page.
func TestRDPEditorError(t *testing.T) {
	a, p := testApp(t)
	tt := newRDPEditor(t, a, p)
	if err := tt.Click(locales.Ok()); err != nil {
		t.Fatal(err)
	}
	if !a.windowOpen(WindowEditor) || a.editor.kind != config.KindRDP || !tt.HasText(locales.ErrHostRequired()) {
		t.Fatalf("open %v, kind %q; texts %q", a.windowOpen(WindowEditor), a.editor.kind, tt.Texts())
	}
}

// TestHostAddr is the address the lists show: with the port when it is
// not the protocol's own.
func TestHostAddr(t *testing.T) {
	h := &config.Host{Addr: "10.0.0.7", User: "root"}
	if got := hostAddr(h); got != "10.0.0.7" {
		t.Errorf("%q", got)
	}
	h.Port = 2222
	if got := hostAddr(h); got != "10.0.0.7:2222" {
		t.Errorf("%q", got)
	}
	h.Type, h.Port = config.KindRDP, 0
	if got, icon := hostAddr(h), hostIcon(h); got != "10.0.0.7" || icon != "monitor" {
		t.Errorf("%q, icon %q", got, icon)
	}
	h.Port = 3390
	if got := hostAddr(h); got != "10.0.0.7:3390" {
		t.Errorf("%q", got)
	}
	if icon := hostIcon(&config.Host{}); icon != "server" {
		t.Errorf("icon %q", icon)
	}
}
