//go:build !windows

package rdp

// EmbedSupported tells whether a remote desktop can be drawn in a window of
// the app's: only Windows brings the control that draws one.
func EmbedSupported() bool { return false }

// Embed draws no desktop on this system: see ErrNoEmbed. The app opens the
// desktop in the system's client here (rdp.Launch).
func Embed(parent uintptr, c Config) (Desktop, error) { return nil, ErrNoEmbed }
