//go:build darwin

package rdp

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Launch opens c in FreeRDP when it is installed, else in Microsoft
// Remote Desktop, which opens a .rdp file; that client asks for the
// password itself, as a file of another computer cannot carry it.
func Launch(c Config) (Session, error) {
	if path, err := exec.LookPath("xfreerdp"); err == nil {
		proc, err := start(path, freeRDPArgs(c, ""))
		if err != nil {
			return Session{}, err
		}
		return Session{Client: filepath.Base(path), proc: proc}, nil
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	dir = filepath.Join(dir, "Finallyshell", "rdp")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Session{}, err
	}
	path := filepath.Join(dir, connectionFileName(c))
	if err := os.WriteFile(path, []byte(connectionFile(c)), 0o600); err != nil {
		return Session{}, err
	}
	// The client is opened by the system, and stays once the app quits:
	// the session this returns has no process of ours to close.
	if _, err := start("open", []string{"-a", "Microsoft Remote Desktop", path}); err != nil {
		return Session{}, fmt.Errorf("%w: install FreeRDP (brew install freerdp) or Microsoft Remote Desktop", ErrNoClient)
	}
	return Session{Client: "Microsoft Remote Desktop"}, nil
}
