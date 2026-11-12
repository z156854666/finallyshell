package app

import (
	"fmt"
	"runtime"
	"slices"
	"strconv"

	"finallyshell/internal/widgets"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// rdpSizes are the desktop sizes the editor suggests, the first being
// the client's own: full screen, on Windows, for a connection that names
// none. A host may have another one, from a file this build did not
// write: that size is listed too.
var rdpSizes = [][2]int{{0, 0}, {1920, 1080}, {1680, 1050}, {1600, 900}, {1440, 900}, {1366, 768}, {1280, 800}, {1280, 720}, {1024, 768}}

// rdpDepths are the color depths the editor offers, in bits.
var rdpDepths = []int{32, 24, 16}

// rdpPage is the editor's page of a Windows remote desktop, beside the
// SSH connection's: the address, the Windows account it logs in with,
// the size of its screen, its colors and what it shares with this
// computer. Choosing the page makes the connection one; the SSH
// connection's page makes it a shell again.
func (a *App) rdpPage(c *ui.Context, ed *connEditor, errs map[editorField]string, input func(ui.Element) ui.Element) {
	ui.Form(c, func() {
		a.connectionFields(c, ed, errs, input)
		ui.Fieldset(c, locales.SectionAuth(), func() {
			ui.Field(c, locales.FieldUser(), func() {
				input(ui.TextInput(c, &ed.user))
			}).Error(errs[fieldUser])
			ui.Field(c, locales.FieldPassword(), func() {
				input(ui.TextInput(c, &ed.password).Password())
			})
			a.rememberField(c, &ed.remember, false)
			ui.Field(c, locales.FieldDomain(), func() {
				input(ui.TextInput(c, &ed.rdp.Domain).Placeholder(locales.DomainPlaceholder()))
			}).Description(locales.DomainHint())
		})
		ui.Fieldset(c, locales.SectionScreen(), func() {
			ui.Field(c, locales.FieldRDPSize(), func() {
				sizes := rdpSizes
				cur := [2]int{ed.rdp.Width, ed.rdp.Height}
				if !slices.Contains(sizes, cur) {
					sizes = append(slices.Clone(sizes), cur)
				}
				names := make([]string, len(sizes))
				for i, s := range sizes {
					names[i] = rdpSizeName(s)
				}
				chosen := names[max(slices.Index(sizes, cur), 0)]
				if ui.Select(c, &chosen, names).Width(200).Changed() {
					at := max(slices.Index(names, chosen), 0)
					ed.rdp.Width, ed.rdp.Height = sizes[at][0], sizes[at][1]
				}
			}).Description(locales.FieldRDPSizeHint())
			// Windows' client takes a command line alone, which sets the
			// size and nothing else: the rest are its own settings.
			if runtime.GOOS != "windows" {
				ui.Field(c, locales.FieldRDPDepth(), func() {
					depth := ed.rdp.Depth
					if depth == 0 {
						depth = 32
					}
					names := make([]string, len(rdpDepths))
					for i, d := range rdpDepths {
						names[i] = strconv.Itoa(d)
					}
					chosen := names[max(slices.Index(rdpDepths, depth), 0)]
					if ui.Select(c, &chosen, names).Width(120).Changed() {
						ed.rdp.Depth = rdpDepths[max(slices.Index(names, chosen), 0)]
					}
				})
				ui.Field(c, "", func() {
					ui.Row(c).Gap(16).Children(func() {
						clipboard := !ed.rdp.NoClipboard
						if ui.Checkbox(c, &clipboard, locales.FieldClipboard()).Changed() {
							ed.rdp.NoClipboard = !clipboard
						}
						drives := !ed.rdp.NoDrives
						if ui.Checkbox(c, &drives, locales.FieldDrives()).Changed() {
							ed.rdp.NoDrives = !drives
						}
					})
				}).Description(locales.RdpShareHint())
			}
		})
	})
	if runtime.GOOS == "windows" {
		widgets.Muted(c, locales.RdpWindowsNote()).FontSize(12)
	} else {
		widgets.Muted(c, locales.RdpClientNote()).FontSize(12)
	}
}

// rdpSizeName is the editor's name of a desktop size: the client's own
// for 0, else the pixels.
func rdpSizeName(s [2]int) string {
	if s[0] <= 0 || s[1] <= 0 {
		return locales.RdpClientSize()
	}
	return fmt.Sprintf("%d × %d", s[0], s[1])
}
