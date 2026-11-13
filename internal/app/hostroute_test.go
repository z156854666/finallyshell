package app

import (
	"testing"

	"finallyshell/internal/config"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// routeHosts are a host of each way: directly, through a named saved proxy,
// through one without a name, through a jump host, which goes through a
// proxy itself, through a jump host that is gone, and a remote desktop,
// which opens no connection of its own.
func routeHosts(a *App) map[string]*config.Host {
	socks := &config.ProxyEntry{ID: "p1", Name: "cdt", Kind: config.ProxySOCKS5, Addr: "cdt.inses.top", Port: 20394, User: "alice"}
	plain := &config.ProxyEntry{ID: "p2", Kind: config.ProxyHTTP, Addr: "127.0.0.1", Port: 3128}
	a.hosts.Proxies = []*config.ProxyEntry{socks, plain}
	hosts := map[string]*config.Host{
		"direct":  {ID: "direct", Name: "direct", Addr: "10.0.0.1", User: "root", Auth: config.AuthPassword},
		"proxied": {ID: "proxied", Name: "proxied", Addr: "10.0.0.2", User: "root", Auth: config.AuthPassword, Proxy: socks.Config()},
		"nameless": {ID: "nameless", Name: "nameless", Addr: "10.0.0.3", User: "root", Auth: config.AuthPassword,
			Proxy: plain.Config()},
		"bastion": {ID: "bastion", Name: "bastion", Addr: "203.0.113.5", User: "jump", Auth: config.AuthPassword, Proxy: socks.Config()},
		"viaJump": {ID: "viaJump", Name: "web", Addr: "10.0.0.4", User: "root", Auth: config.AuthPassword,
			Proxy: &config.ProxyConfig{Kind: config.ProxyJump, JumpHost: "bastion"}},
		"lost": {ID: "lost", Name: "lost", Addr: "10.0.0.5", User: "root", Auth: config.AuthPassword,
			Proxy: &config.ProxyConfig{Kind: config.ProxyJump, JumpHost: "gone"}},
		"rdp": {ID: "rdp", Name: "rdp", Addr: "10.0.0.6", User: "root", Auth: config.AuthPassword,
			Type: config.KindRDP, Proxy: socks.Config()},
	}
	for _, h := range hosts {
		a.hosts.Add(h)
	}
	return hosts
}

func TestHostRoute(t *testing.T) {
	a, _ := testApp(t)
	h := routeHosts(a)
	socks := "cdt (SOCKS5 cdt.inses.top:20394)"
	for _, c := range []struct {
		host, route, icon string
	}{
		{"direct", "", ""},
		{"proxied", socks, "route"},
		{"nameless", "HTTP 127.0.0.1:3128", "route"},
		{"bastion", socks, "route"},
		{"viaJump", "bastion → " + socks, "route"},
		{"lost", "gone", "route"},
		{"rdp", "", ""},
	} {
		if got := a.hostRoute(h[c.host]); got != c.route {
			t.Errorf("%s: route %q, want %q", c.host, got, c.route)
		}
		if got := hostRouteIcon(h[c.host]); got != c.icon {
			t.Errorf("%s: icon %q, want %q", c.host, got, c.icon)
		}
	}

	// The line says whether it is a proxy or a jump host.
	for _, c := range []struct {
		host, line string
	}{
		{"direct", ""},
		{"proxied", locales.HostRouteProxy(socks)},
		{"viaJump", locales.HostRouteJump("bastion → " + socks)},
		{"lost", locales.HostRouteJump("gone")},
	} {
		if got := a.hostRouteLine(h[c.host]); got != c.line {
			t.Errorf("%s: line %q, want %q", c.host, got, c.line)
		}
	}
	if got, want := a.hostTooltip(h["proxied"]), "root@10.0.0.2:22\n"+locales.HostRouteProxy(socks); got != want {
		t.Errorf("tooltip %q, want %q", got, want)
	}

	// A chain of jump hosts that loops stops: connecting says so.
	h["bastion"].Proxy = &config.ProxyConfig{Kind: config.ProxyJump, JumpHost: "viaJump"}
	if got := a.hostRoute(h["viaJump"]); got == "" || len(got) > 200 {
		t.Errorf("loop: %q", got)
	}
}

// The lists of hosts mark those that do not connect directly, in both
// languages, naming the way in the mark, which the tooltip says too.
func TestHostRouteMarks(t *testing.T) {
	a, p, tt := openTestManager(t)
	h := routeHosts(a)
	p.SetLocale("zh-Hans")
	tt.Frame()
	for _, name := range []string{"proxied", "viaJump", "nameless", "bastion", "lost"} {
		if !tt.HasText(a.hostRouteLine(h[name])) {
			t.Errorf("%s: no mark; texts %q", name, tt.Texts())
		}
	}
	if !tt.HasText("跳板机：bastion → cdt (SOCKS5 cdt.inses.top:20394)") || tt.HasText("Jump host:") {
		t.Errorf("the mark is not in Chinese; texts %q", tt.Texts())
	}
	saveShot(t, tt, "manager-routes-zh-Hans.png")
	p.SetLocale("en")
	tt.Frame()
	if !tt.HasText("Proxy: cdt (SOCKS5 cdt.inses.top:20394)") {
		t.Errorf("no mark in English; texts %q", tt.Texts())
	}
	saveShot(t, tt, "manager-routes-en.png")

	// The panel under the server monitor, and the quick connect list, mark
	// them too: the same rows, in the main window.
	a2, p2 := testApp(t)
	managerHosts(a2)
	h2 := routeHosts(a2)
	a2.connTree.tree.Open.Clear()
	a2.restoreLeftPanel()
	main := ui.NewTester(a2.View, 1280, 820)
	p2.tt = main
	main.Frame()
	for _, name := range []string{"proxied", "viaJump"} {
		if !main.HasText(a2.hostRouteLine(h2[name])) {
			t.Errorf("%s: no mark in the main window; texts %q", name, main.Texts())
		}
	}
	saveShot(t, main, "conntree-routes-en.png")

	// The proxies gone, the marks go with them.
	line := locales.HostRouteProxy("cdt (SOCKS5 cdt.inses.top:20394)")
	h2["proxied"].Proxy, h2["viaJump"].Proxy, h2["bastion"].Proxy, h2["nameless"].Proxy = nil, nil, nil, nil
	main.Frame()
	if main.HasText(line) {
		t.Error("a mark outlived its proxy")
	}
	// A jump host that is gone keeps its own, and says so.
	if !main.HasText(a2.hostRouteLine(h2["lost"])) {
		t.Errorf("the lost jump host has no mark; texts %q", main.Texts())
	}
}
