//go:build !windows && !linux && !darwin

package rdp

// Launch fails with ErrNoClient: this package knows no remote desktop
// client of this system.
func Launch(c Config) (Session, error) { return Session{}, ErrNoClient }
