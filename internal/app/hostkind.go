package app

import (
	"net"
	"strconv"

	"finallyshell/internal/config"
)

// hostIcon is the icon of h in the lists of connections and sessions: a
// screen for a remote desktop, a server for an SSH connection.
func hostIcon(h *config.Host) string {
	if h.IsRDP() {
		return "monitor"
	}
	return "server"
}

// hostAddr is h's address as the lists show it: with the port when it is
// not the protocol's own, 22 for SSH and 3389 for a remote desktop, and
// in brackets when the address is IPv6.
func hostAddr(h *config.Host) string {
	def := 22
	if h.IsRDP() {
		def = 3389
	}
	if port := h.ProtocolPort(); port != def {
		return net.JoinHostPort(h.Addr, strconv.Itoa(port))
	}
	return h.Addr
}
