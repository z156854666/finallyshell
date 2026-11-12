//go:build linux

package rdp

import (
	"os/exec"
	"path/filepath"
)

// clients are the FreeRDP programs tried, the newest first: xfreerdp3 is
// FreeRDP 3, xfreerdp 2, wlfreerdp the Wayland build.
var clients = []string{"xfreerdp3", "xfreerdp", "wlfreerdp"}

// Launch opens c in FreeRDP, the first of clients the system has;
// ErrNoClient when it has none.
func Launch(c Config) (Session, error) {
	for _, name := range clients {
		path, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		proc, err := start(path, freeRDPArgs(c, ""))
		if err != nil {
			return Session{}, err
		}
		return Session{Client: filepath.Base(path), proc: proc}, nil
	}
	return Session{}, ErrNoClient
}
