package app

import (
	"encoding/json"
	"reflect"
	"testing"

	"finallyshell/internal/config"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// choose picks option in the select labeled label.
func choose(t *testing.T, tt *ui.Tester, label, option string) {
	t.Helper()
	sel, ok := tt.AccessByRole(ui.RolePopUpButton, label)
	if !ok {
		t.Fatalf("no select %q; texts %q", label, tt.Texts())
	}
	b := sel.Bounds
	tt.ClickAt(b.X+b.W/2, b.Y+b.H/2)
	tt.Frame()
	// The text may be shown elsewhere too, as the type of a proxy in the
	// table behind the popup; the popup's option has a role of its own.
	if item, ok := tt.AccessByRole(ui.RoleOption, option); ok {
		b = item.Bounds
		tt.ClickAt(b.X+b.W/2, b.Y+b.H/2)
	} else if err := tt.Click(option); err != nil {
		t.Fatalf("%s: %v; texts %q", label, err, tt.Texts())
	}
	tt.Frame()
}

// proxyEditorHosts are a host to edit, web-1, and others to jump
// through.
func proxyEditorHosts(a *App) *config.Host {
	h := &config.Host{ID: "h1", Name: "web-1", Addr: "10.0.1.20", Port: 22, User: "deploy", Auth: config.AuthPassword}
	a.hosts.Add(h)
	a.hosts.Add(&config.Host{ID: "bastion", Name: "bastion", Addr: "203.0.113.5", Port: 22, User: "jump", Auth: config.AuthKey, KeyPath: "~/.ssh/id_ed25519"})
	a.hosts.Add(&config.Host{ID: "db", Addr: "10.0.1.30", User: "root", Auth: config.AuthPassword})
	return h
}

// demoSavedProxies are saved proxies for the page, one of each kind, the
// SOCKS5 one with a user.
func demoSavedProxies(a *App) []*config.ProxyEntry {
	proxies := []*config.ProxyEntry{
		{ID: "p1", Name: "cdt", Kind: config.ProxySOCKS5, Addr: "cdt.inses.top", Port: 20394, User: "alice"},
		{ID: "p2", Name: "office", Kind: config.ProxyHTTP, Addr: "127.0.0.1", Port: 3128},
	}
	a.hosts.Proxies = proxies
	return proxies
}

// openProxyPage opens the editor on h at its Proxy page.
func openProxyPage(t *testing.T, a *App, p *fakePlatform, h *config.Host) *ui.Tester {
	t.Helper()
	a.editConnection(h)
	tt := editorWindow(t, a, p)
	if err := tt.Click(locales.EditorPageProxy()); err != nil {
		t.Fatal(err)
	}
	if a.editor.nav.Page != editorProxy {
		t.Fatalf("page %q", a.editor.nav.Page)
	}
	return tt
}

// TestEditorProxyPage chooses a proxy by its type, adds one of the other
// type with its password, and saves each in both languages.
func TestEditorProxyPage(t *testing.T) {
	for _, lang := range []string{"en", "zh-Hans"} {
		t.Run(lang, func(t *testing.T) {
			a, p := testApp(t)
			saved := savedHosts(a)
			p.SetLocale(lang)
			h := proxyEditorHosts(a)
			proxies := demoSavedProxies(a)
			a.proxySecrets()[proxySecretKey("p1")] = "pa55"
			tt := openProxyPage(t, a, p, h)
			// Nothing of a proxy or a jump host until a type is chosen.
			for _, want := range []string{locales.FieldProxyType(), locales.ProxyNoProxy(), locales.ProxyNoneHint()} {
				if !tt.HasText(want) {
					t.Errorf("missing %q; texts %q", want, tt.Texts())
				}
			}
			if tt.HasText(locales.FieldJumpHost()) {
				t.Errorf("a jump host without the type; texts %q", tt.Texts())
			}
			// No proxy has a list to show yet, and its password is the
			// dialog's, not the page's.
			if tt.HasText(locales.ColProxyName()) || tt.HasText(locales.FieldProxyServer()) {
				t.Errorf("a list without a type; texts %q", tt.Texts())
			}

			// SOCKS5: its own proxies only, one of which is chosen.
			choose(t, tt, locales.FieldProxyType(), "SOCKS5")
			pf := &a.editor.proxy
			if pf.kind != config.ProxySOCKS5 || pf.entry != "" {
				t.Fatalf("type %q, entry %q", pf.kind, pf.entry)
			}
			for _, want := range []string{locales.FieldProxyServer(), locales.ColProxyCurrent(), locales.ColProxyName(),
				locales.ColProxyType(), locales.ColProxyHost(), locales.ColProxyPort(), "cdt", "SOCKS5",
				"cdt.inses.top", "20394", locales.ProxyAdd(), locales.ProxyEdit(), locales.ProxyRemove(),
				locales.ProxySOCKS5Hint()} {
				if !tt.HasText(want) {
					t.Errorf("missing %q; texts %q", want, tt.Texts())
				}
			}
			if tt.HasText("office") || tt.HasText("3128") {
				t.Errorf("a HTTP proxy in the SOCKS5 list; texts %q", tt.Texts())
			}
			if tt.HasText(locales.FieldPassword()) {
				t.Errorf("a password of all the proxies; texts %q", tt.Texts())
			}
			// A proxy of the kind chosen is needed.
			if err := tt.Click(locales.Ok()); err != nil {
				t.Fatal(err)
			}
			if !a.windowOpen(WindowEditor) || !tt.HasText(locales.ErrProxyRequired()) {
				t.Fatalf("no error; texts %q", tt.Texts())
			}
			saveShot(t, tt, "editor-proxy-required-"+lang+".png")

			a.editor.proxy.entry = proxies[0].ID
			tt.Frame()
			if tt.HasText(locales.ErrProxyRequired()) {
				t.Errorf("error after choosing; texts %q", tt.Texts())
			}
			saveShot(t, tt, "editor-proxy-chosen-"+lang+".png")
			if err := tt.Click(locales.Ok()); err != nil {
				t.Fatal(err)
			}
			want := &config.ProxyConfig{ID: "p1", Kind: config.ProxySOCKS5, Addr: "cdt.inses.top", Port: 20394, User: "alice"}
			if !reflect.DeepEqual(h.Proxy, want) {
				t.Fatalf("saved %+v; texts %q", h.Proxy, tt.Texts())
			}
			var file struct {
				Proxies []config.ProxyEntry `json:"proxies"`
				Hosts   []struct {
					Proxy map[string]any `json:"proxy"`
				} `json:"hosts"`
			}
			if err := json.Unmarshal(*saved, &file); err != nil {
				t.Fatalf("saved %s: %v", *saved, err)
			}
			if got := file.Hosts[0].Proxy; len(got) != 5 || got["id"] != "p1" || got["kind"] != "socks5" {
				t.Errorf("saved proxy %v", got)
			}
			if len(file.Proxies) != 2 || file.Proxies[0].ID != "p1" {
				t.Errorf("saved proxies %+v", file.Proxies)
			}

			// A new one, of the other type, with its password: the page
			// follows it.
			a.editConnection(h)
			tt = editorWindow(t, a, p)
			if err := tt.Click(locales.EditorPageProxy()); err != nil {
				t.Fatal(err)
			}
			if a.editor.proxy.kind != config.ProxySOCKS5 || a.editor.proxy.entry != "p1" {
				t.Fatalf("reopened %+v", a.editor.proxy)
			}
			if err := tt.Click(locales.ProxyAdd()); err != nil {
				t.Fatal(err)
			}
			pf = &a.editor.proxy
			if !pf.dialog || !tt.HasText(locales.ProxyNew()) || fieldValue(t, tt, locales.FieldPort()) != "1080" ||
				!tt.HasText(locales.FieldPassword()) {
				t.Fatalf("no dialog; %+v; texts %q", pf, tt.Texts())
			}
			// The dialog is the page's too: its type is set, not clicked.
			pf.form.setKind(config.ProxyHTTP)
			tt.Frame()
			if fieldValue(t, tt, locales.FieldPort()) != "8080" || !tt.HasText(locales.ProxyHTTPHint()) {
				t.Fatalf("http; texts %q", tt.Texts())
			}
			typeInto(t, tt, locales.FieldProxyName(), "lab")
			typeInto(t, tt, locales.FieldHost(), "proxy.lab.example")
			typeInto(t, tt, locales.FieldUser(), "bob")
			typeInto(t, tt, locales.FieldPassword(), "lab-pw")
			saveShot(t, tt, "editor-proxy-new-"+lang+".png")
			// Enter in a field is the dialog's OK, not the editor's.
			tt.Key(0, ui.KeyEnter)
			if pf.dialog || !a.windowOpen(WindowEditor) || len(pf.proxies) != 3 {
				t.Fatalf("dialog %v, editor %v, %d proxies", pf.dialog, a.windowOpen(WindowEditor), len(pf.proxies))
			}
			added := pf.proxies[2]
			if want := (config.ProxyEntry{ID: added.ID, Name: "lab", Kind: config.ProxyHTTP, Addr: "proxy.lab.example", Port: 8080, User: "bob"}); !reflect.DeepEqual(*added, want) {
				t.Errorf("added %+v", *added)
			}
			if pf.kind != config.ProxyHTTP || pf.entry != added.ID || pf.passwords[added.ID] != "lab-pw" {
				t.Fatalf("page %+v", pf)
			}
			if err := tt.Click(locales.Ok()); err != nil {
				t.Fatal(err)
			}
			want = &config.ProxyConfig{ID: added.ID, Kind: config.ProxyHTTP, Addr: "proxy.lab.example", Port: 8080, User: "bob"}
			if !reflect.DeepEqual(h.Proxy, want) {
				t.Fatalf("saved %+v", h.Proxy)
			}
			// Each proxy has its own password, in the credential store.
			p.until(t, "the passwords stored", func() bool { return p.secrets()[proxySecretKey(added.ID)] == "lab-pw" })
			if got := p.secrets()[proxySecretKey(added.ID)]; got != "lab-pw" || p.secrets()[proxySecretKey("p1")] != "" {
				t.Errorf("stored %q, and %q for the first", got, p.secrets()[proxySecretKey("p1")])
			}

			// Changed: the dialog shows what the proxy says, its password
			// included.
			a.editConnection(h)
			tt = editorWindow(t, a, p)
			if err := tt.Click(locales.EditorPageProxy()); err != nil {
				t.Fatal(err)
			}
			if err := tt.Click(locales.ProxyEdit()); err != nil {
				t.Fatal(err)
			}
			if pf = &a.editor.proxy; !tt.HasText(locales.ProxyChange()) || fieldValue(t, tt, locales.FieldHost()) != "proxy.lab.example" ||
				fieldValue(t, tt, locales.FieldUser()) != "bob" || pf.form.password != "lab-pw" {
				t.Fatalf("edit dialog %+v; texts %q", pf.form, tt.Texts())
			}
			// Unchanged: the dialog's OK, which the editor's Cancel would
			// not be with the same word behind it.
			tt.Key(0, ui.KeyEnter)
			if pf.dialog || pf.entry != added.ID || pf.passwords[added.ID] != "lab-pw" || pf.changed[added.ID] {
				t.Fatalf("the dialog %v, page %+v", pf.dialog, pf)
			}

			// A jump host is the other type: choosing it gives up the proxy.
			choose(t, tt, locales.FieldProxyType(), locales.ProxyJump())
			if pf.kind != config.ProxyJump || pf.entry != "" || !tt.HasText(locales.ProxyJumpHint()) {
				t.Errorf("jump; %+v; texts %q", pf, tt.Texts())
			}
			choose(t, tt, locales.FieldJumpHost(), "bastion (jump@203.0.113.5)")
			if pf.jump != "bastion" || !tt.HasText(locales.JumpHostNote()) {
				t.Errorf("jump host; %+v; texts %q", pf, tt.Texts())
			}
			saveShot(t, tt, "editor-proxy-jump-"+lang+".png")
			if err := tt.Click(locales.Ok()); err != nil {
				t.Fatal(err)
			}
			if want := (&config.ProxyConfig{Kind: config.ProxyJump, JumpHost: "bastion"}); !reflect.DeepEqual(h.Proxy, want) {
				t.Fatalf("saved %+v", h.Proxy)
			}

			// None again: no proxy and no jump host.
			a.editConnection(h)
			tt = editorWindow(t, a, p)
			if err := tt.Click(locales.EditorPageProxy()); err != nil {
				t.Fatal(err)
			}
			choose(t, tt, locales.FieldProxyType(), locales.ProxyNoProxy())
			if a.editor.proxy.jump != "" || !tt.HasText(locales.ProxyNoneHint()) || tt.HasText(locales.FieldJumpHost()) {
				t.Errorf("none; %+v; texts %q", a.editor.proxy, tt.Texts())
			}
			if err := tt.Click(locales.Ok()); err != nil {
				t.Fatal(err)
			}
			if h.Proxy != nil {
				t.Errorf("proxy %+v", h.Proxy)
			}
		})
	}
}

func TestEditorProxyErrors(t *testing.T) {
	a, p := testApp(t)
	h := proxyEditorHosts(a)
	demoSavedProxies(a)
	tt := openProxyPage(t, a, p, h)
	pf := &a.editor.proxy

	// A new proxy takes a host and a port in range.
	choose(t, tt, locales.FieldProxyType(), "SOCKS5")
	if err := tt.Click(locales.ProxyAdd()); err != nil {
		t.Fatal(err)
	}
	typeInto(t, tt, locales.FieldPort(), "0")
	tt.Key(0, ui.KeyEnter)
	for _, want := range []string{locales.ErrHostRequired(), locales.ErrPortInvalid()} {
		if !tt.HasText(want) {
			t.Errorf("missing %q; texts %q", want, tt.Texts())
		}
	}
	if !pf.dialog || len(pf.proxies) != 2 {
		t.Fatal("a wrong proxy was taken")
	}
	saveShot(t, tt, "editor-proxy-errors-en.png")
	typeInto(t, tt, locales.FieldHost(), "proxy.example.com")
	typeInto(t, tt, locales.FieldPort(), "1080")
	if tt.HasText(locales.ErrHostRequired()) || tt.HasText(locales.ErrPortInvalid()) {
		t.Errorf("errors after fixing; texts %q", tt.Texts())
	}
	tt.Key(0, ui.KeyEnter)
	if pf.dialog || len(pf.proxies) != 3 || pf.entry != pf.proxies[2].ID {
		t.Fatalf("dialog %v, %d proxies, chosen %q", pf.dialog, len(pf.proxies), pf.entry)
	}

	// A jump host that comes back here is a loop: OK keeps the editor open
	// with its errors.
	a.hosts.Host("bastion").Proxy = &config.ProxyConfig{Kind: config.ProxyJump, JumpHost: "db"}
	a.hosts.Host("db").Proxy = &config.ProxyConfig{Kind: config.ProxyJump, JumpHost: "h1"}
	choose(t, tt, locales.FieldProxyType(), locales.ProxyJump())
	choose(t, tt, locales.FieldJumpHost(), "bastion (jump@203.0.113.5)")
	if err := tt.Click(locales.Ok()); err != nil {
		t.Fatal(err)
	}
	loop := locales.ErrJumpLoop("web-1 → bastion → 10.0.1.30 → web-1")
	if !a.windowOpen(WindowEditor) || a.editor.nav.Page != editorProxy || !tt.HasText(loop) || h.Proxy != nil {
		t.Fatalf("page %q; texts %q", a.editor.nav.Page, tt.Texts())
	}
	saveShot(t, tt, "editor-proxy-loop-en.png")
	p.SetLocale("zh-Hans")
	tt.Frame()
	if !tt.HasText("跳板机形成了循环：web-1 → bastion → 10.0.1.30 → web-1") {
		t.Errorf("not in Chinese; texts %q", tt.Texts())
	}
	saveShot(t, tt, "editor-proxy-loop-zh-Hans.png")
	p.SetLocale("")

	// Or one whose own jump host was deleted.
	a.hosts.Host("db").Proxy = &config.ProxyConfig{Kind: config.ProxyJump, JumpHost: "gone"}
	tt.Frame()
	if !tt.HasText(locales.ErrJumpHostMissing("10.0.1.30")) {
		t.Errorf("missing; texts %q", tt.Texts())
	}
	a.hosts.Host("db").Proxy = nil
	if err := tt.Click(locales.Ok()); err != nil {
		t.Fatal(err)
	}
	if a.windowOpen(WindowEditor) || h.Proxy == nil || h.Proxy.JumpHost != "bastion" {
		t.Fatalf("not saved: %+v", h.Proxy)
	}

	// A jump host without a jump host to choose is an error, said.
	a.editConnection(h)
	tt = editorWindow(t, a, p)
	if err := tt.Click(locales.EditorPageProxy()); err != nil {
		t.Fatal(err)
	}
	a.editor.proxy.jump = ""
	if err := tt.Click(locales.Ok()); err != nil {
		t.Fatal(err)
	}
	if !a.windowOpen(WindowEditor) || !tt.HasText(locales.ErrJumpHostRequired()) {
		t.Fatalf("no error; texts %q", tt.Texts())
	}
	choose(t, tt, locales.FieldProxyType(), locales.ProxyNoProxy())
	if err := tt.Click(locales.Ok()); err != nil {
		t.Fatal(err)
	}
	if a.windowOpen(WindowEditor) || h.Proxy != nil {
		t.Fatalf("not saved: %+v", h.Proxy)
	}

	// With no other host there is nothing to jump through, and no error.
	tt = newConnection(t, a, p)
	b := a.hosts
	a.hosts = config.NewStore()
	a.editor.proxy.hosts = a.hosts
	tt.Type("solo.example.com")
	if err := tt.Click(locales.EditorPageProxy()); err != nil {
		t.Fatal(err)
	}
	choose(t, tt, locales.FieldProxyType(), locales.ProxyJump())
	if !tt.HasText(locales.NoJumpHosts()) {
		t.Errorf("texts %q", tt.Texts())
	}
	choose(t, tt, locales.FieldProxyType(), locales.ProxyNoProxy())
	if err := tt.Click(locales.Ok()); err != nil {
		t.Fatal(err)
	}
	if a.windowOpen(WindowEditor) {
		t.Errorf("the editor stayed open; texts %q", tt.Texts())
	}
	a.hosts = b
}

// Removing a saved proxy: the hosts that connect through it connect
// directly, as the list saved without it says, and its password is gone.
func TestEditorProxyRemove(t *testing.T) {
	a, p := testApp(t)
	saved := savedHosts(a)
	h := proxyEditorHosts(a)
	proxies := demoSavedProxies(a)
	a.proxySecrets()[proxySecretKey("p1")] = "pa55"
	other := &config.Host{ID: "h2", Name: "web-2", Addr: "10.0.1.21", User: "deploy", Auth: config.AuthPassword, Proxy: proxies[0].Config()}
	a.hosts.Add(other)
	tt := openProxyPage(t, a, p, h)
	choose(t, tt, locales.FieldProxyType(), "SOCKS5")
	tt.Frame()
	a.editor.proxy.entry = proxies[0].ID
	tt.Frame()
	if err := tt.Click(locales.ProxyRemove()); err != nil {
		t.Fatal(err)
	}
	pf := &a.editor.proxy
	if pf.entry != "" || len(pf.proxies) != 1 {
		t.Fatalf("removed; %+v", pf)
	}
	if err := tt.Click(locales.Ok()); err != nil {
		t.Fatal(err)
	}
	if h.Proxy != nil || other.Proxy != nil {
		t.Fatalf("hosts still use it: %+v %+v", h.Proxy, other.Proxy)
	}
	p.until(t, "the password gone", func() bool { return p.secrets()[proxySecretKey("p1")] == "" })
	if _, known := a.proxySecrets()[proxySecretKey("p1")]; known {
		t.Error("the proxy's password is still known")
	}
	var file struct {
		Proxies []config.ProxyEntry `json:"proxies"`
	}
	json.Unmarshal(*saved, &file)
	if len(file.Proxies) != 1 || file.Proxies[0].ID != proxies[1].ID {
		t.Errorf("saved proxies %+v", file.Proxies)
	}
}

// A host whose proxy was saved for it alone shows it with the others, and
// the proxy keys this build does not know are kept as its proxy changes.
func TestEditorProxyKeepsUnknownKeys(t *testing.T) {
	a, p := testApp(t)
	saved := savedHosts(a)
	var h config.Host
	json.Unmarshal([]byte(`{"id": "h1", "addr": "10.0.0.1", "user": "root", "auth": "password",
		"proxy": {"kind": "socks5", "addr": "127.0.0.1", "port": 1080, "dns": "remote"}}`), &h)
	a.hosts.Add(&h)
	tt := openProxyPage(t, a, p, &h)
	pf := &a.editor.proxy
	if pf.kind != config.ProxySOCKS5 || len(pf.proxies) != 1 || pf.entry != pf.proxies[0].ID || !tt.HasText("127.0.0.1") {
		t.Fatalf("the host's proxy is not listed: %+v; texts %q", pf, tt.Texts())
	}
	if err := tt.Click(locales.ProxyEdit()); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText(locales.ProxyChange()) || fieldValue(t, tt, locales.FieldHost()) != "127.0.0.1" {
		t.Fatalf("no edit dialog; texts %q", tt.Texts())
	}
	typeInto(t, tt, locales.FieldPort(), "1090")
	tt.Key(0, ui.KeyEnter)
	if err := tt.Click(locales.Ok()); err != nil {
		t.Fatal(err)
	}
	var file struct {
		Proxies []config.ProxyEntry `json:"proxies"`
		Hosts   []struct {
			Proxy map[string]any `json:"proxy"`
		} `json:"hosts"`
	}
	if err := json.Unmarshal(*saved, &file); err != nil {
		t.Fatalf("saved %s: %v", *saved, err)
	}
	want := map[string]any{"id": pf.entry, "kind": "socks5", "addr": "127.0.0.1", "port": 1090.0, "dns": "remote"}
	if len(file.Hosts) != 1 || !reflect.DeepEqual(file.Hosts[0].Proxy, want) {
		t.Errorf("saved %s", *saved)
	}
	if len(file.Proxies) != 1 || file.Proxies[0].Port != 1090 {
		t.Errorf("saved proxies %+v", file.Proxies)
	}
}
