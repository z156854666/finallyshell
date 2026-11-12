package rdp

import (
	"fmt"
	"strings"
)

// connectionFileName is the name of the .rdp file of c: its address and
// user, which no file name holds as they are. Two connections of the
// same address and user have the same settings, so one file serves both.
func connectionFileName(c Config) string {
	part := func(s string) string {
		return strings.NewReplacer(":", "_", "[", "", "]", "", "*", "_", "?", "_",
			`\`, "_", "/", "_", " ", "_").Replace(s)
	}
	name := part(c.Address())
	if c.User != "" {
		name += "-" + part(c.User)
	}
	return name + ".rdp"
}

// connectionFile is the content of the .rdp file a client opens: the
// settings of c, as FinalShell's RDP links set them. Only macOS opens one:
// see Launch there. The password is not in it — a file on one computer
// cannot carry another's — so the file says to ask for it.
func connectionFile(c Config) string {
	var b strings.Builder
	line := func(key, value string) {
		b.WriteString(key)
		b.WriteString(":s:")
		b.WriteString(value)
		b.WriteString("\r\n")
	}
	num := func(key string, value int) {
		fmt.Fprintf(&b, "%s:i:%d\r\n", key, value)
	}
	line("full address", c.Address())
	if c.User != "" {
		line("username", c.User)
	}
	if c.Domain != "" {
		line("domain", c.Domain)
	}
	num("prompt for credentials", 1)
	if c.size() == "" {
		num("screen mode id", 2) // full screen
	} else {
		num("screen mode id", 1) // a window
		num("desktopwidth", c.Width)
		num("desktopheight", c.Height)
	}
	num("session bpp", c.depth())
	num("use multimon", 0)
	num("smart sizing", 1)
	num("authentication level", 2)
	num("enablecredsspsupport", 1)
	num("autoreconnection enabled", 1)
	num("audiomode", 0)
	num("redirectprinters", 0)
	num("redirectcomports", 0)
	num("redirectsmartcards", 0)
	num("audiocapturemode", 0)
	num("videoplaybackmode", 1)
	if c.Clipboard {
		num("redirectclipboard", 1)
	} else {
		num("redirectclipboard", 0)
	}
	if c.Drives {
		line("drivestoredirect", "*")
	} else {
		line("drivestoredirect", "")
	}
	return b.String()
}
