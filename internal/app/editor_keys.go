package app

// The connection editor's private keys: the ones saved in the app
// (config.KeyStore, keys.json), which the private key field's Browse button
// lists as FinalShell's does, to import a key file into the app, rename and
// delete the saved ones, and choose one for the host. A host that logs in
// with a saved key names it (config.Host.Key) rather than a file, so that
// the key stays where the app keeps it, and travels with the connections
// when they sync.

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"finallyshell/internal/config"
	"finallyshell/internal/sshx"
	"finallyshell/internal/widgets"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// keyFields is a dialog of the saved keys, the connection editor's or the
// import window's: whether it shows, the row chosen, and the name of the key
// being renamed.
type keyFields struct {
	dialog bool
	list   ui.ListState
	row    int // the row chosen, -1 for none
	// renaming is set while the chosen key's name is edited, in name.
	renaming bool
	name     string
	// err says why importing, renaming or deleting failed.
	err func() string
}

// key is the saved key the dialog has chosen: nil for none.
func (kf *keyFields) key(ks *config.KeyStore) *config.SSHKey {
	if kf.row < 0 || kf.row >= len(ks.Keys) {
		return nil
	}
	return ks.Keys[kf.row]
}

// open shows the dialog on the saved key with the ID, none when there is
// none.
func (kf *keyFields) open(ks *config.KeyStore, id string) {
	kf.dialog, kf.renaming, kf.err = true, false, nil
	kf.row = ks.Find(id)
}

// keyOwner is whose key the dialog of the saved keys chooses: the connection
// editor's private key field, or the import window's key for the connections
// it lists. It says where the dialog shows, and what choosing, renaming and
// deleting a key do to the owner.
type keyOwner struct {
	kf     *keyFields
	window WindowKind    // the window the key file chooser is of
	dir    func() string // where the chooser starts
	alive  func() bool   // whether the dialog is still the one showing
	// choose is OK: the key chosen, nil for none.
	choose func(k *config.SSHKey)
	// renamed and deleted follow a new name, and a key deleted, for the
	// owner's key.
	renamed func(k *config.SSHKey)
	deleted func(k *config.SSHKey)
}

// editorKeys is the dialog of the saved keys for ed's private key field.
func (a *App) editorKeys(ed *connEditor) keyOwner {
	kf := &ed.keys
	return keyOwner{
		kf:     kf,
		window: WindowEditor,
		dir: func() string {
			if a.savedKeyOf(ed) != nil {
				return keyDir("")
			}
			return keyDir(ed.keyPath)
		},
		alive: func() bool { return a.editor == ed },
		choose: func(k *config.SSHKey) {
			if k != nil {
				ed.key, ed.keyPath = k.ID, k.Name
			}
			kf.dialog, kf.renaming, kf.err = false, false, nil
		},
		renamed: func(k *config.SSHKey) {
			if ed.key == k.ID {
				ed.keyPath = k.Name
			}
		},
		deleted: func(k *config.SSHKey) {
			if ed.key == k.ID {
				ed.key, ed.keyPath = "", ""
			}
		},
	}
}

// savedKeyName is the name of the saved key with the ID, "" when there is
// none.
func (a *App) savedKeyName(id string) string {
	if k := a.keys.Key(id); k != nil {
		return k.Name
	}
	return ""
}

// savedKeyOf is the saved key the private key field shows: nil when the
// field holds a file path, or nothing, rather than a saved key's name.
func (a *App) savedKeyOf(ed *connEditor) *config.SSHKey {
	if ed.key == "" || strings.TrimSpace(ed.keyPath) != a.savedKeyName(ed.key) {
		return nil
	}
	return a.keys.Key(ed.key)
}

// saveSavedKeys writes keys.json. What fails to be written is written at
// the next save, as hosts.json is.
func (a *App) saveSavedKeys() { a.saveKeys(a.keys) }

// keyColumns are the columns of the dialog's table, as FinalShell's are:
// the key's name, its type, and the length of its public part.
func keyColumns() []ui.TableColumn {
	return []ui.TableColumn{
		{ID: "name", Title: locales.ColKeyName(), MinWidth: 150},
		{ID: "type", Title: locales.ColKeyType(), Width: 110},
		{ID: "length", Title: locales.ColKeyLength(), Width: 80, Align: ui.End},
	}
}

// keyBits is the length of the key's public part, as the table shows it.
func keyBits(k *config.SSHKey) string {
	if k.Bits <= 0 {
		return ""
	}
	return strconv.Itoa(k.Bits)
}

// keyUseCount is how many connections log in with the key.
func (a *App) keyUseCount(k *config.SSHKey) int {
	if k == nil {
		return 0
	}
	n := 0
	for _, h := range a.hosts.Live() {
		if h.Key == k.ID {
			n++
		}
	}
	return n
}

// keyDialog is the dialog the private key field's Browse button, and the
// import window's, open: the keys saved in the app, the buttons that import
// a key file, rename and delete the chosen key, and OK, which makes the
// chosen key the owner's. Enter is OK, and Escape, as a click outside,
// Cancel.
func (a *App) keyDialog(c *ui.Context, o keyOwner) {
	kf := o.kf
	if kf.renaming {
		a.keyRenameDialog(c, o)
		return
	}
	keys := a.keys.Keys
	kf.list.Selected = &kf.row
	kf.list.Label = func(row int) string {
		if row < 0 || row >= len(keys) {
			return ""
		}
		return keys[row].Name
	}
	// Enter where no field takes it: the window's waits while the dialog
	// shows.
	submit := c.Shortcut(0, ui.KeyEnter)
	ui.Column(c).Width(540).Gap(10).Children(func() {
		ui.Text(c, locales.KeyDialogTitle()).FontSize(15).Bold()
		ui.Row(c).Gap(12).AlignItems(ui.Start).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Children(func() {
				table := ui.Table(c, &kf.list, keyColumns(), len(keys), func(row, col int) {
					k := keys[row]
					value := k.Name
					switch col {
					case 1:
						value = k.Type
					case 2:
						value = keyBits(k)
					}
					ui.Text(c, value).SingleLine().MinWidth(0)
				}).Height(200).Border(1, theme.Border).Radius(6)
				if len(keys) == 0 {
					widgets.Muted(c, locales.KeysEmpty()).FontSize(12)
				}
				submit = table.Submitted() || submit
			})
			ui.Column(c).Gap(8).Width(96).Children(func() {
				chosen := kf.key(a.keys)
				if ui.Button(c, locales.KeyImport()).Clicked() {
					a.importSavedKey(o)
				}
				if ui.Button(c, locales.Edit()).Disabled(chosen == nil).Clicked() && chosen != nil {
					kf.renaming, kf.name, kf.err = true, chosen.Name, nil
				}
				if ui.Button(c, locales.Delete()).Disabled(chosen == nil).Clicked() && chosen != nil {
					a.deleteSavedKey(o)
				}
			})
		})
		chosen := kf.key(a.keys)
		switch n := a.keyUseCount(chosen); {
		case kf.err != nil:
			ui.Text(c, kf.err()).TextColor(theme.Danger).FontSize(12)
		case chosen != nil && chosen.Encrypted:
			widgets.Muted(c, locales.KeyNeedsPassphrase()).FontSize(12)
		case n > 0:
			widgets.Muted(c, locales.KeyUsedBy(n)).FontSize(12)
		}
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if ui.Button(c, locales.Cancel()).Clicked() {
				kf.dialog = false
			}
			if ui.PrimaryButton(c, locales.Ok()).Clicked() {
				submit = true
			}
		})
	})
	if submit {
		o.choose(kf.key(a.keys))
	}
}

// keyRenameDialog is the dialog that renames the key chosen in the keys
// dialog.
func (a *App) keyRenameDialog(c *ui.Context, o keyOwner) {
	kf := o.kf
	submit := c.Shortcut(0, ui.KeyEnter)
	ui.Column(c).Width(360).Gap(10).Children(func() {
		ui.Text(c, locales.KeyEditTitle()).FontSize(15).Bold()
		ui.Form(c, func() {
			ui.Field(c, locales.FieldName(), func() {
				submit = ui.TextInput(c, &kf.name).AutoFocus().Submitted() || submit
			})
		})
		if kf.err != nil {
			ui.Text(c, kf.err()).TextColor(theme.Danger).FontSize(12)
		}
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if ui.Button(c, locales.Cancel()).Clicked() {
				kf.renaming, kf.err = false, nil
			}
			if ui.PrimaryButton(c, locales.Ok()).Clicked() {
				submit = true
			}
		})
	})
	if !submit {
		return
	}
	name := strings.TrimSpace(kf.name)
	k := kf.key(a.keys)
	switch {
	case k == nil:
		kf.renaming, kf.err = false, nil
	case name == "":
		kf.err = locales.ErrKeyNameRequired
	default:
		if name != k.Name {
			k.Name = name
			if o.renamed != nil {
				o.renamed(k)
			}
			a.saveSavedKeys()
		}
		kf.renaming, kf.err = false, nil
	}
}

// deleteSavedKey deletes the key chosen in the dialog, and the key of the
// connections that logged in with it, which then need another one.
func (a *App) deleteSavedKey(o keyOwner) {
	kf := o.kf
	k := kf.key(a.keys)
	if k == nil {
		return
	}
	a.keys.Remove(k.ID)
	if o.deleted != nil {
		o.deleted(k)
	}
	changed := false
	for _, h := range a.hosts.Hosts {
		if h.Key == k.ID {
			h.Key, changed = "", true
		}
	}
	if changed {
		a.saveHosts(a.hosts)
	}
	a.saveSavedKeys()
	if kf.row >= len(a.keys.Keys) {
		kf.row = len(a.keys.Keys) - 1
	}
	kf.err = nil
}

// importSavedKey asks for a private key file and saves it as a key of the
// app's, choosing it in the dialog: a key imported twice, as its
// fingerprint tells, is one key.
func (a *App) importSavedKey(o keyOwner) {
	kf := o.kf
	dir, window := o.dir(), o.window
	title := locales.ChooseKeyFile()
	go func() {
		path, err := a.plat.OpenFile(window, title, dir)
		if err != nil || path == "" {
			return
		}
		data, err := os.ReadFile(path)
		if err != nil {
			a.post(func() {
				if o.alive() {
					kf.err = func() string { return locales.ErrReadKey(err.Error()) }
				}
			})
			return
		}
		info, err := sshx.KeyInfoOf(data)
		if err != nil {
			a.post(func() {
				if o.alive() {
					kf.err = func() string { return locales.ErrParseKey(err.Error()) }
				}
			})
			return
		}
		a.post(func() {
			if !o.alive() {
				return
			}
			if i := a.keys.FindFingerprint(info.Fingerprint); i >= 0 {
				kf.row, kf.err = i, nil
				return
			}
			a.keys.Keys = append(a.keys.Keys, &config.SSHKey{
				ID: config.NewKeyID(), Name: keyNameFromPath(path),
				Type: info.Type, Bits: info.Bits,
				Fingerprint: info.Fingerprint, Encrypted: info.Encrypted,
				Key: string(data), AddedAt: time.Now(),
			})
			a.saveSavedKeys()
			kf.row, kf.err = len(a.keys.Keys)-1, nil
		})
	}()
}

// keyNameFromPath is the name a key imported from path shows: the file's
// name alone.
func keyNameFromPath(path string) string {
	name := strings.TrimSpace(filepath.Base(path))
	if name == "" || name == "." || name == string(filepath.Separator) {
		return locales.KeyDefaultName()
	}
	return name
}

// keyDir is where the key file chooser starts: the folder of the key file at
// path, else ~/.ssh.
func keyDir(path string) string {
	if p := strings.TrimSpace(path); p != "" {
		return filepath.Dir(sshx.ExpandHome(p))
	}
	if home, err := os.UserHomeDir(); err == nil {
		if fi, err := os.Stat(filepath.Join(home, ".ssh")); err == nil && fi.IsDir() {
			return filepath.Join(home, ".ssh")
		}
	}
	return ""
}
