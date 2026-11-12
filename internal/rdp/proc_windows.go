//go:build windows

package rdp

import (
	"os"
	"os/exec"
)

// start runs the program name with args and returns its process, which
// lives on in a window of its own.
func start(name string, args []string) (*os.Process, error) {
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd.Process, nil
}
