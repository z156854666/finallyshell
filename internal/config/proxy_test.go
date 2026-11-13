package config

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestProxyJSON(t *testing.T) {
	// Every field is read, and keys this build does not know are kept.
	in := `{"id": "h1", "addr": "x", "proxy": {"kind": "http", "addr": "proxy.example.com", "port": 3128, "user": "u", "jumpHost": "h2", "timeout": 5}}`
	var h Host
	if err := json.Unmarshal([]byte(in), &h); err != nil {
		t.Fatal(err)
	}
	want := &ProxyConfig{Kind: ProxyHTTP, Addr: "proxy.example.com", Port: 3128, User: "u", JumpHost: "h2"}
	got := *h.Proxy
	got.extra = nil
	if !reflect.DeepEqual(&got, want) {
		t.Errorf("proxy %+v, want %+v", got, want)
	}
	data, _ := json.Marshal(h)
	var back struct {
		Proxy map[string]any `json:"proxy"`
	}
	json.Unmarshal(data, &back)
	if back.Proxy["timeout"] != 5.0 || back.Proxy["port"] != 3128.0 || back.Proxy["kind"] != "http" || back.Proxy["jumpHost"] != "h2" {
		t.Errorf("written %s", data)
	}
	// A proxy of no kind has none of the keys.
	data, _ = json.Marshal(ProxyConfig{})
	if string(data) != "{}" {
		t.Errorf("empty proxy %s", data)
	}
}

func TestProxyPorts(t *testing.T) {
	for _, c := range []struct {
		p    ProxyConfig
		want int
	}{
		{ProxyConfig{Kind: ProxySOCKS5}, 1080},
		{ProxyConfig{Kind: ProxyHTTP}, 8080},
		{ProxyConfig{Kind: ProxyHTTP, Port: 3128}, 3128},
		{ProxyConfig{Kind: ProxyJump}, 0},
	} {
		if got := c.p.PortOrDefault(); got != c.want {
			t.Errorf("%+v: port %d, want %d", c.p, got, c.want)
		}
	}
	if ProxyOf(&Host{}) != nil || ProxyOf(&Host{Proxy: &ProxyConfig{}}) != nil {
		t.Error("a host without a proxy has one")
	}
}

func TestProxyEntryJSON(t *testing.T) {
	// Every field is read, and keys this build does not know are kept.
	in := `{"id": "p1", "name": "cdt", "kind": "socks5", "addr": "cdt.example.com", "port": 20394, "user": "alice", "dns": "remote"}`
	var e ProxyEntry
	if err := json.Unmarshal([]byte(in), &e); err != nil {
		t.Fatal(err)
	}
	want := ProxyEntry{ID: "p1", Name: "cdt", Kind: ProxySOCKS5, Addr: "cdt.example.com", Port: 20394, User: "alice"}
	got := e
	got.extra = nil
	if !reflect.DeepEqual(got, want) {
		t.Errorf("entry %+v, want %+v", got, want)
	}
	if e.Title() != "cdt" || e.PortOrDefault() != 20394 {
		t.Errorf("title %q, port %d", e.Title(), e.PortOrDefault())
	}
	if c := e.Config(); c.ID != "p1" || c.Kind != ProxySOCKS5 || c.Addr != "cdt.example.com" || c.Port != 20394 || c.User != "alice" {
		t.Errorf("config %+v", c)
	}
	data, _ := json.Marshal(e)
	var back map[string]any
	json.Unmarshal(data, &back)
	if back["dns"] != "remote" || back["id"] != "p1" || back["port"] != 20394.0 {
		t.Errorf("written %s", data)
	}
	// One without a name is named by its address and port.
	if got := (&ProxyEntry{Kind: ProxyHTTP, Addr: "127.0.0.1"}).Title(); got != "127.0.0.1:8080" {
		t.Errorf("title %q", got)
	}
}

func TestShareProxies(t *testing.T) {
	s := NewStore()
	// The same proxy twice: at the usual port, which the entry writes
	// out, and with another user, which is another proxy.
	s.Add(&Host{ID: "a", Proxy: &ProxyConfig{Kind: ProxySOCKS5, Addr: "p", Port: 1080, User: "u"}})
	s.Add(&Host{ID: "b", Proxy: &ProxyConfig{Kind: ProxySOCKS5, Addr: "p", User: "u"}})
	s.Add(&Host{ID: "c", Proxy: &ProxyConfig{Kind: ProxySOCKS5, Addr: "p", User: "w"}})
	s.Add(&Host{ID: "d"})
	s.Add(&Host{ID: "e", Proxy: &ProxyConfig{Kind: ProxyJump, JumpHost: "a"}})
	if !s.ShareProxies() {
		t.Fatal("nothing shared")
	}
	if len(s.Proxies) != 2 {
		t.Fatalf("proxies %+v", s.Proxies)
	}
	if a, b, c := s.Host("a").Proxy, s.Host("b").Proxy, s.Host("c").Proxy; a.ID == "" || a.ID != b.ID || c.ID == a.ID {
		t.Errorf("shared %q %q %q", a.ID, b.ID, c.ID)
	}
	// The proxy is not changed of its address: the entry has it.
	if p := s.Host("a").Proxy; p.Kind != ProxySOCKS5 || p.Addr != "p" || p.Port != 1080 || p.User != "u" {
		t.Errorf("copy %+v", p)
	}
	if s.Host("d").Proxy != nil || s.Host("e").Proxy.ID != "" || s.Host("e").Proxy.JumpHost != "a" {
		t.Errorf("hosts %+v %+v", s.Host("d").Proxy, s.Host("e").Proxy)
	}
	// Nothing left to share the second time.
	if s.ShareProxies() {
		t.Error("shared again")
	}
}

func TestSyncProxies(t *testing.T) {
	s := NewStore()
	e := &ProxyEntry{ID: "p1", Kind: ProxySOCKS5, Addr: "p", Port: 1080, User: "u"}
	s.Proxies = []*ProxyEntry{e}
	s.Add(&Host{ID: "a", Proxy: e.Config()})
	s.Add(&Host{ID: "b", Proxy: &ProxyConfig{ID: "p1", Kind: ProxySOCKS5, Addr: "old", Port: 1080}})
	s.Add(&Host{ID: "c", Proxy: &ProxyConfig{ID: "gone", Kind: ProxySOCKS5, Addr: "lost", Port: 1080}})
	s.Add(&Host{ID: "d", Proxy: &ProxyConfig{Kind: ProxySOCKS5, Addr: "own", Port: 1080}})

	// A change of the entry reaches the hosts that use it, and only them.
	e.Addr, e.Port, e.User = "new", 3128, "alice"
	if !s.SyncProxies() {
		t.Fatal("nothing synced")
	}
	if p := s.Host("a").Proxy; p.Addr != "new" || p.Port != 3128 || p.User != "alice" {
		t.Errorf("a %+v", p)
	}
	if p := s.Host("b").Proxy; p.Addr != "new" || p.Port != 3128 {
		t.Errorf("b %+v", p)
	}
	// One whose entry is gone keeps its copy: it still connects through it.
	if p := s.Host("c").Proxy; p.Addr != "lost" {
		t.Errorf("c %+v", p)
	}
	if p := s.Host("d").Proxy; p.Addr != "own" || p.ID != "" {
		t.Errorf("d %+v", p)
	}
	if s.SyncProxies() {
		t.Error("synced again")
	}

	// Removing the entry takes the hosts' proxies with it.
	if !s.ClearProxy("p1") || s.Host("a").Proxy != nil || s.Host("b").Proxy != nil || s.Host("c").Proxy == nil {
		t.Errorf("cleared %+v %+v %+v", s.Host("a").Proxy, s.Host("b").Proxy, s.Host("c").Proxy)
	}
}

func TestStoreProxiesJSON(t *testing.T) {
	s := NewStore()
	s.Proxies = []*ProxyEntry{{ID: "p1", Name: "office", Kind: ProxyHTTP, Addr: "127.0.0.1", Port: 3128}}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	back, _, err := ParseHosts(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Proxies) != 1 || !reflect.DeepEqual(*back.Proxies[0], *s.Proxies[0]) {
		t.Errorf("read %+v", back.Proxies)
	}
	// A file without the list has none, and saves without it.
	plain, _, err := ParseHosts([]byte(`{"version": 2, "folders": [], "hosts": []}`))
	if err != nil || plain.Proxies != nil {
		t.Fatalf("%+v %v", plain, err)
	}
	data, _ = json.Marshal(plain)
	if strings.Contains(string(data), "proxies") {
		t.Errorf("written %s", data)
	}
}

func TestJumpHosts(t *testing.T) {
	s := NewStore()
	jump := func(id string) *ProxyConfig { return &ProxyConfig{Kind: ProxyJump, JumpHost: id} }
	s.Add(&Host{ID: "direct", Name: "direct"})
	s.Add(&Host{ID: "socks", Name: "socks", Proxy: &ProxyConfig{Kind: ProxySOCKS5, Addr: "p"}})
	s.Add(&Host{ID: "a", Name: "a", Proxy: jump("b")})
	s.Add(&Host{ID: "b", Name: "b", Proxy: jump("socks")})
	s.Add(&Host{ID: "x", Name: "x", Proxy: jump("y")})
	s.Add(&Host{ID: "y", Name: "y", Proxy: jump("x")})
	s.Add(&Host{ID: "self", Name: "self", Proxy: jump("self")})
	s.Add(&Host{ID: "lost", Name: "lost", Proxy: jump("gone")})
	s.Add(&Host{ID: "old", Name: "old", DeletedAt: time.Now()})
	s.Add(&Host{ID: "toOld", Name: "toOld", Proxy: jump("old")})
	s.Add(&Host{ID: "intoLoop", Name: "intoLoop", Proxy: jump("x")})

	for _, c := range []struct {
		id    string
		jumps []string
		loop  []string
		from  string // the host whose jump host is missing
	}{
		{id: "direct"},
		{id: "socks"},
		{id: "a", jumps: []string{"b", "socks"}},
		{id: "b", jumps: []string{"socks"}},
		{id: "x", loop: []string{"x", "y", "x"}},
		{id: "self", loop: []string{"self", "self"}},
		{id: "intoLoop", loop: []string{"intoLoop", "x", "y", "x"}},
		{id: "lost", from: "lost"},
		{id: "toOld", from: "toOld"},
	} {
		got, err := s.JumpHosts(s.Host(c.id))
		var loop *JumpLoopError
		var missing *JumpError
		switch {
		case c.loop != nil:
			if !errors.As(err, &loop) || !slices.Equal(loop.Names, c.loop) {
				t.Errorf("%s: %v, want a loop %v", c.id, err, c.loop)
			}
		case c.from != "":
			if !errors.As(err, &missing) || missing.From != c.from || !errors.Is(err, ErrJumpHostMissing) {
				t.Errorf("%s: %v, want a missing jump host", c.id, err)
			}
		case err != nil || !slices.Equal(hostIDs(got), c.jumps):
			t.Errorf("%s: jumps %v %v, want %v", c.id, hostIDs(got), err, c.jumps)
		}
	}

	// A host being edited, not saved yet, whose new jump host leads back
	// to it.
	edited := &Host{ID: "socks", Name: "socks", Proxy: jump("a")}
	var loop *JumpLoopError
	if _, err := s.JumpHosts(edited); !errors.As(err, &loop) || !slices.Equal(loop.Names, []string{"socks", "a", "b", "socks"}) {
		t.Errorf("edited: %v", err)
	}
	// A new host has no ID: nothing leads to it.
	if got, err := s.JumpHosts(&Host{Proxy: jump("a")}); err != nil || !slices.Equal(hostIDs(got), []string{"a", "b", "socks"}) {
		t.Errorf("new: %v %v", hostIDs(got), err)
	}
}
