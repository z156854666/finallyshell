package config

import (
	"errors"
	"fmt"
	"maps"
	"net"
	"reflect"
	"strconv"
	"strings"
)

// ProxyKind is what a host connects through.
type ProxyKind string

const (
	// ProxyNone connects directly, as a host without a proxy.
	ProxyNone ProxyKind = ""
	// ProxySOCKS5 connects through a SOCKS5 proxy, which resolves the
	// host's name itself.
	ProxySOCKS5 ProxyKind = "socks5"
	// ProxyHTTP connects through an HTTP proxy with CONNECT.
	ProxyHTTP ProxyKind = "http"
	// ProxyJump connects through another saved host, a jump host, as
	// OpenSSH's ProxyJump: its SSH server opens the connection.
	ProxyJump ProxyKind = "jump"
)

// DefaultProxyPort is the usual port of a proxy of kind k: 1080 for
// SOCKS5, 8080 for HTTP, 0 for the others.
func DefaultProxyPort(k ProxyKind) int {
	switch k {
	case ProxySOCKS5:
		return 1080
	case ProxyHTTP:
		return 8080
	}
	return 0
}

// IsProxyServer tells whether a proxy of kind k is a SOCKS5 or HTTP proxy:
// one of those a host can share with others, saved in the store's proxies.
func IsProxyServer(k ProxyKind) bool { return k == ProxySOCKS5 || k == ProxyHTTP }

// ProxyConfig is a host's proxy. A SOCKS5 or HTTP proxy has an address,
// a port, and a user when it asks for one, whose password is kept as the
// host's: not in hosts.json. Such a proxy is a copy of one of the store's
// saved proxies, which ID names: changing that one changes this. A jump
// host is another saved host, which may have a proxy of its own.
type ProxyConfig struct {
	// ID is the saved proxy of Store.Proxies this host connects through,
	// for a SOCKS5 or HTTP proxy: the kind, address, port and user are
	// the entry's. "" for a proxy saved for the host alone, as hosts
	// saved before proxies had a list of their own.
	ID   string    `json:"id,omitempty"`
	Kind ProxyKind `json:"kind,omitempty"`
	Addr string    `json:"addr,omitempty"`
	Port int       `json:"port,omitempty"`
	User string    `json:"user,omitempty"`
	// JumpHost is the ID of the jump host, for ProxyJump.
	JumpHost string `json:"jumpHost,omitempty"`

	extra extras
}

// PortOrDefault is the proxy's port, the usual one for its kind when
// unset.
func (p *ProxyConfig) PortOrDefault() int {
	if p.Port == 0 {
		return DefaultProxyPort(p.Kind)
	}
	return p.Port
}

// ProxyOf is the proxy h connects through: nil when it connects directly.
func ProxyOf(h *Host) *ProxyConfig {
	if h.Proxy == nil || h.Proxy.Kind == ProxyNone {
		return nil
	}
	return h.Proxy
}

type proxyJSON ProxyConfig

var proxyKeys = knownKeys(reflect.TypeFor[proxyJSON]())

func (p ProxyConfig) MarshalJSON() ([]byte, error) { return encodeObject(proxyJSON(p), p.extra) }

func (p *ProxyConfig) UnmarshalJSON(data []byte) error {
	var v proxyJSON
	ex, err := decodeObject(data, &v, proxyKeys)
	if err != nil {
		return err
	}
	*p = ProxyConfig(v)
	p.extra = ex
	return nil
}

// ProxyEntry is a saved SOCKS5 or HTTP proxy, one of Store.Proxies, which
// hosts connect through by the ID in their proxy (ProxyConfig.ID): they
// keep a copy of what it says, so that connecting needs no lookup, and
// Store.SyncProxies brings the copies up to date as it changes.
type ProxyEntry struct {
	ID string `json:"id"`
	// Name is what the user calls the proxy; "" for one named by its
	// address and port (Title).
	Name string `json:"name,omitempty"`
	// Kind is ProxySOCKS5 or ProxyHTTP.
	Kind ProxyKind `json:"kind"`
	// Addr, Port and User are the proxy's host, its port (0 for the
	// kind's usual one) and the user it asks for, if any. Password is
	// never saved here: it is the host's, as a proxy's password is.
	Addr string `json:"addr,omitempty"`
	Port int    `json:"port,omitempty"`
	User string `json:"user,omitempty"`

	extra extras
}

// PortOrDefault is the proxy's port, the usual one for its kind when
// unset.
func (e *ProxyEntry) PortOrDefault() int {
	if e.Port == 0 {
		return DefaultProxyPort(e.Kind)
	}
	return e.Port
}

// Title is the proxy's name, else its address and port, as the Proxy page
// lists it.
func (e *ProxyEntry) Title() string {
	if e.Name != "" {
		return e.Name
	}
	return net.JoinHostPort(e.Addr, strconv.Itoa(e.PortOrDefault()))
}

// Config is the proxy a host that uses e connects through: a copy, which
// names e, so that sessions read it as it was when they connected.
func (e *ProxyEntry) Config() *ProxyConfig {
	return &ProxyConfig{ID: e.ID, Kind: e.Kind, Addr: e.Addr, Port: e.Port, User: e.User}
}

// Entry is e as a saved proxy, for one saved for a host alone, as hosts
// saved before proxies had a list of their own have: the Proxy page shows
// it with the others rather than lose it.
func (p *ProxyConfig) Entry() *ProxyEntry {
	return &ProxyEntry{ID: p.ID, Kind: p.Kind, Addr: p.Addr, Port: p.Port, User: p.User}
}

// Clone is a copy of e, sharing nothing with it.
func (e *ProxyEntry) Clone() *ProxyEntry {
	c := *e
	c.extra = maps.Clone(e.extra)
	return &c
}

// CloneProxies are copies of the proxies, nil for none.
func CloneProxies(entries []*ProxyEntry) []*ProxyEntry {
	if len(entries) == 0 {
		return nil
	}
	out := make([]*ProxyEntry, len(entries))
	for i, e := range entries {
		out[i] = e.Clone()
	}
	return out
}

// NewProxyID returns an ID for a new saved proxy.
func NewProxyID() string { return fmt.Sprintf("p%d", newID()) }

type proxyEntryJSON ProxyEntry

var proxyEntryKeys = knownKeys(reflect.TypeFor[proxyEntryJSON]())

func (e ProxyEntry) MarshalJSON() ([]byte, error) { return encodeObject(proxyEntryJSON(e), e.extra) }

func (e *ProxyEntry) UnmarshalJSON(data []byte) error {
	var v proxyEntryJSON
	ex, err := decodeObject(data, &v, proxyEntryKeys)
	if err != nil {
		return err
	}
	*e = ProxyEntry(v)
	e.extra = ex
	return nil
}

// Proxy is the saved proxy with the ID; nil for "" or when there is none.
func (s *Store) Proxy(id string) *ProxyEntry {
	if id == "" {
		return nil
	}
	for _, e := range s.Proxies {
		if e.ID == id {
			return e
		}
	}
	return nil
}

// proxyKey tells saved proxies apart: their kind, address, port and user.
func proxyKey(k ProxyKind, addr string, port int, user string) string {
	return strings.Join([]string{string(k), addr, strconv.Itoa(port), user}, "\x00")
}

// SyncProxies makes each host connect through the saved proxy its proxy
// names (ProxyConfig.ID), as that proxy is now: its kind, address, port
// and user. A host whose saved proxy is not in the store keeps the copy it
// has, so that a hosts.json whose list lacks it, as a build that does not
// sync the proxies writes, still connects through it. It reports whether
// it changed a host.
func (s *Store) SyncProxies() bool {
	changed := false
	for _, h := range s.Hosts {
		p := h.Proxy
		if p == nil || p.ID == "" || !IsProxyServer(p.Kind) {
			continue
		}
		e := s.Proxy(p.ID)
		if e == nil || (p.Kind == e.Kind && p.Addr == e.Addr && p.Port == e.Port && p.User == e.User) {
			continue
		}
		np := *p
		np.Kind, np.Addr, np.Port, np.User = e.Kind, e.Addr, e.Port, e.User
		h.Proxy = &np
		changed = true
	}
	return changed
}

// ClearProxy makes the hosts that connect through the saved proxy with the
// ID connect directly, as it is removed for good (the Proxy page's Remove
// does this). It reports whether it changed a host.
func (s *Store) ClearProxy(id string) bool {
	changed := false
	for _, h := range s.Hosts {
		if h.Proxy != nil && h.Proxy.ID == id {
			h.Proxy = nil
			changed = true
		}
	}
	return changed
}

// ShareProxies gives each host whose SOCKS5 or HTTP proxy was saved for it
// alone, as hosts saved before proxies had a list of their own, the saved
// proxy of the same kind, address, port and user, adding it to the store
// when there is none yet, so that the proxies of a file read before this
// build are the ones the Proxy page lists. It reports whether it changed a
// host.
func (s *Store) ShareProxies() bool {
	byKey := map[string]*ProxyEntry{}
	for _, e := range s.Proxies {
		byKey[proxyKey(e.Kind, e.Addr, e.PortOrDefault(), e.User)] = e
	}
	changed := false
	for _, h := range s.Hosts {
		p := h.Proxy
		if p == nil || p.ID != "" || !IsProxyServer(p.Kind) {
			continue
		}
		key := proxyKey(p.Kind, p.Addr, p.PortOrDefault(), p.User)
		e := byKey[key]
		if e == nil {
			e = &ProxyEntry{ID: NewProxyID(), Kind: p.Kind, Addr: p.Addr, Port: p.Port, User: p.User}
			s.Proxies = append(s.Proxies, e)
			byKey[key] = e
		}
		np := *p
		np.ID = e.ID
		h.Proxy = &np
		changed = true
	}
	return changed
}

// ErrJumpHostMissing is a jump host that is not saved, or was deleted.
var ErrJumpHostMissing = errors.New("the jump host is missing")

// JumpLoopError is a chain of jump hosts that comes back to one of them.
type JumpLoopError struct {
	// Names are the hosts' titles along the chain, the one met again
	// last.
	Names []string
}

func (e *JumpLoopError) Error() string {
	return "jump hosts loop: " + strings.Join(e.Names, " → ")
}

// JumpError is a jump host on the way to a host that cannot be used.
type JumpError struct {
	// From is the title of the host whose jump host it is.
	From string
	Err  error
}

func (e *JumpError) Error() string { return "jump host of " + e.From + ": " + e.Err.Error() }
func (e *JumpError) Unwrap() error { return e.Err }

// JumpHosts are the jump hosts on the way to h, the one h connects
// through first and the one that connects first last: none when h does
// not go through a jump host. It fails with a *JumpError for a jump
// host that is missing or deleted, and a *JumpLoopError for a chain that
// comes back to a host already on it, h included.
func (s *Store) JumpHosts(h *Host) ([]*Host, error) {
	var jumps []*Host
	seen := map[string]bool{}
	names := []string{h.Title()}
	if h.ID != "" {
		seen[h.ID] = true
	}
	for cur := h; ; {
		p := ProxyOf(cur)
		if p == nil || p.Kind != ProxyJump {
			return jumps, nil
		}
		next := s.Host(p.JumpHost)
		if next == nil || next.Deleted() {
			return nil, &JumpError{From: cur.Title(), Err: ErrJumpHostMissing}
		}
		names = append(names, next.Title())
		if seen[next.ID] {
			return nil, &JumpLoopError{Names: names}
		}
		seen[next.ID] = true
		jumps = append(jumps, next)
		cur = next
	}
}
