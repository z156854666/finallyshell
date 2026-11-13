package app

// The way a host reaches its server, which the lists of hosts mark: one
// icon for the hosts that do not connect directly, and a line of their
// tooltip saying how they do, as FinalShell's lists mark those behind a
// proxy. The proxies and jump hosts themselves are in proxy.go and
// editor_proxy.go.

import (
	"finallyshell/internal/config"
	"finallyshell/internal/icons"
	"finallyshell/internal/sshx"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// maxRouteDepth is how many jump hosts hostRoute goes through: a chain that
// loops is an error of connecting, not something to draw forever.
const maxRouteDepth = 4

// hostRouteIcon is the icon of the row of h that says it does not connect
// directly: "" for one that does, and for a remote desktop, which opens no
// connection of its own.
func hostRouteIcon(h *config.Host) string {
	if h.IsRDP() || config.ProxyOf(h) == nil {
		return ""
	}
	return "route"
}

// hostRoute is how h reaches its server, in the language shown: "" for a
// host that connects directly, else its proxy, "cdt (SOCKS5
// cdt.inses.top:20394)", its jump host, "bastion", or both, the jump host
// first and its own way after it, "bastion → office (HTTP 127.0.0.1:3128)".
func (a *App) hostRoute(h *config.Host) string { return a.routeOf(h, 0) }

// routeOf is hostRoute for a jump host depth hops away.
func (a *App) routeOf(h *config.Host, depth int) string {
	if h == nil || h.IsRDP() || depth > maxRouteDepth {
		return ""
	}
	p := config.ProxyOf(h)
	if p == nil {
		return ""
	}
	if p.Kind != config.ProxyJump {
		return a.proxyLabel(p)
	}
	j := a.hosts.Host(p.JumpHost)
	name := p.JumpHost
	if j != nil {
		name = j.Title()
	}
	if own := a.routeOf(j, depth+1); own != "" {
		return name + " → " + own
	}
	return name
}

// proxyLabel is the proxy p, as a route says it: its saved name, with what
// it is in parentheses, or its address and port alone.
func (a *App) proxyLabel(p *config.ProxyConfig) string {
	kind, addr := proxyKindLabel(p.Kind), sshx.ProxyAddr(p)
	if e := a.hosts.Proxy(p.ID); e != nil && e.Name != "" {
		return e.Name + " (" + kind + " " + addr + ")"
	}
	return kind + " " + addr
}

// hostRouteLine is what a host's tooltip says of its way, "" for a host
// that connects directly.
func (a *App) hostRouteLine(h *config.Host) string {
	route := a.hostRoute(h)
	if route == "" {
		return ""
	}
	if p := config.ProxyOf(h); p != nil && p.Kind == config.ProxyJump {
		return locales.HostRouteJump(route)
	}
	return locales.HostRouteProxy(route)
}

// hostRouteMark draws, in color, the mark of the row of h: the icon of a way
// that is not direct, named tip for assistive technology, and nothing for a
// host that connects directly.
func hostRouteMark(c *ui.Context, h *config.Host, color ui.Color, tip string) {
	icon := hostRouteIcon(h)
	if icon == "" {
		return
	}
	ui.Icon(c, icons.Get(icon)).FontSize(13).TextColor(color).Shrink(0).Label(tip)
}
