package app

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"finallyshell/locales"

	"github.com/egoist/mygo/transfer"
)

// copied waits for the transfer that copies a file to p to end.
func (f *editorFixture) copied(t *testing.T, p string) {
	t.Helper()
	f.p.until(t, "the copy of "+filepath.Base(p), func() bool {
		if f.a.transfers.active() > 0 {
			return false
		}
		_, err := os.Stat(p)
		return err == nil
	})
}

// Copy and Paste: a file into another folder of the tree, a copy beside
// the file itself, and files copied on this computer, uploaded.
func TestFilePanelCopyPaste(t *testing.T) {
	f := newEditorFixture(t)
	a, tt, s := f.a, f.tt, f.s
	sub := filepath.Join(f.dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	a.openDir(s, f.dir)
	f.settled(t)

	// A file copied in the table, pasted into a folder of the tree.
	f.menu(t, "app.yaml", locales.Copy())
	if a.fileClip == nil || a.fileClip.dir != f.dir || !slices.Equal(a.fileClip.names, []string{"app.yaml"}) {
		t.Fatalf("the clip %+v", a.fileClip)
	}
	if formats := tt.ClipboardData().Formats(); !slices.Contains(formats, fileClipFormat) {
		t.Fatalf("the clipboard's formats %v", formats)
	}
	f.treeMenu(t, sub, locales.Paste())
	f.copied(t, filepath.Join(sub, "app.yaml"))
	if got, err := os.ReadFile(filepath.Join(sub, "app.yaml")); err != nil || string(got) != appYAML {
		t.Fatalf("sub/app.yaml holds %q %v", got, err)
	}

	// Pasted into the folder it is in, a copy takes a name of its own.
	dup := filepath.Join(f.dir, locales.CopyOfName("app")+".yaml")
	f.menu(t, "app.yaml", locales.Copy())
	f.menu(t, "deploy.sh", locales.Paste())
	f.copied(t, dup)
	if got, err := os.ReadFile(dup); err != nil || string(got) != appYAML {
		t.Fatalf("the copy holds %q %v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(f.dir, "app.yaml")); err != nil || string(got) != appYAML {
		t.Errorf("app.yaml holds %q %v", got, err)
	}

	// Files copied on this computer paste as an upload, and drop what a
	// panel copied.
	local := filepath.Join(t.TempDir(), "from-host.txt")
	if err := os.WriteFile(local, []byte("host"), 0o644); err != nil {
		t.Fatal(err)
	}
	data, err := transfer.FileData(local)
	if err != nil {
		t.Fatal(err)
	}
	tt.SetClipboardData(data)
	f.menu(t, "deploy.sh", locales.Paste())
	f.copied(t, filepath.Join(f.dir, "from-host.txt"))
	if got, err := os.ReadFile(filepath.Join(f.dir, "from-host.txt")); err != nil || string(got) != "host" {
		t.Fatalf("from-host.txt holds %q %v", got, err)
	}
	if a.fileClip != nil {
		t.Errorf("the panel's clip survived a copy made elsewhere: %+v", a.fileClip)
	}
}

// Copy in the panel also fetches the files to this computer, so that a
// file manager pastes them, while the panel's Paste still copies on the
// server.
func TestFilePanelCopyToComputer(t *testing.T) {
	f := newEditorFixture(t)
	a, tt := f.a, f.tt
	f.menu(t, "app.yaml", locales.Copy())
	clip := a.fileClip
	if clip == nil || clip.stageDir == "" {
		t.Fatalf("the clip %+v", clip)
	}
	local := filepath.Join(clip.stageDir, "app.yaml")
	f.p.until(t, "the copy for this computer", func() bool {
		if _, err := os.Stat(local); err != nil {
			return false
		}
		return slices.Contains(tt.ClipboardData().Formats(), transfer.FileList)
	})
	if got, err := os.ReadFile(local); err != nil || string(got) != appYAML {
		t.Fatalf("the copy for this computer holds %q %v", got, err)
	}
	if files, err := tt.ClipboardData().Files(); err != nil || !slices.Equal(files, []string{local}) {
		t.Fatalf("the clipboard's files %q %v", files, err)
	}

	// Paste in the folder it came from still copies on the server.
	f.menu(t, "app.yaml", locales.Paste())
	f.copied(t, filepath.Join(f.dir, locales.CopyOfName("app")+".yaml"))
}
