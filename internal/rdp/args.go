package rdp

import (
	"os"
	"strconv"
)

// freeRDPArgs are the arguments that open c in FreeRDP, as its command
// line takes them: the address first, then the login, the size of the
// desktop, its depth and what is shared with it. home is the folder its
// drive redirection shares, "" for none.
func freeRDPArgs(c Config, home string) []string {
	args := []string{"/v:" + c.Address()}
	if c.User != "" {
		args = append(args, "/u:"+c.User)
	}
	if c.Domain != "" {
		args = append(args, "/d:"+c.Domain)
	}
	if c.Password != "" {
		args = append(args, "/p:"+c.Password)
	}
	if size := c.size(); size != "" {
		args = append(args, "/size:"+size)
	} else {
		args = append(args, "/f")
	}
	args = append(args, "/bpp:"+strconv.Itoa(c.depth()), "+auto-reconnect")
	if c.Clipboard {
		args = append(args, "+clipboard")
	} else {
		args = append(args, "-clipboard")
	}
	if c.Drives {
		if home == "" {
			home, _ = os.UserHomeDir()
		}
		if home != "" {
			args = append(args, "/drive:Home,"+home)
		}
	}
	return args
}
