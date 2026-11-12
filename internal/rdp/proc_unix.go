//go:build !windows

package rdp

import (
	"os"
	"os/exec"
	"syscall"
)

// start runs the program name with args, in a session of its own, so
// that it outlives the app, and returns its process.
func start(name string, args []string) (*os.Process, error) {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd.Process, nil
}
