package app

import (
	"io/fs"
	"slices"
	"testing"
	"time"

	"finallyshell/internal/remotefs"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

func TestFileTableSort(t *testing.T) {
	a, _ := testApp(t)
	s := demoSession(a, "files", "10.0.0.21")
	a.active, a.bottomTab = 0, tabFiles
	mod := func(day int) time.Time { return time.Date(2026, 3, day, 9, 30, 0, 0, time.Local) }
	s.files.cwd, s.files.pathEdit = "/data", "/data"
	s.files.entries = []remotefs.Entry{
		{Name: "logs", Mode: fs.ModeDir | 0o750, ModTime: mod(1)},
		{Name: "zeta", Mode: fs.ModeDir | 0o755, ModTime: mod(2)},
		{Name: "aa.log", Size: 1_024, Mode: 0o644, ModTime: mod(5)},
		{Name: "notes.txt", Size: 4_096, Mode: 0o644, ModTime: mod(4)},
		{Name: "run.sh", Size: 512, Mode: 0o755, ModTime: mod(3)},
	}
	tt := ui.NewTester(a.View, 1280, 820)
	names := func() []string {
		var out []string
		for _, e := range s.files.shown() {
			out = append(out, e.Name)
		}
		return out
	}
	check := func(step string, want ...string) {
		t.Helper()
		if got := names(); !slices.Equal(got, want) {
			t.Errorf("%s: %v, want %v", step, got, want)
		}
	}
	tt.Frame()
	// With no header clicked, the listing's order stands: folders first.
	check("the listing's order", "logs", "zeta", "aa.log", "notes.txt", "run.sh")

	// A click on a header sorts by that column, folders first still.
	if err := tt.Click(locales.ColSize()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if s.files.sort != (ui.SortOrder{Column: "size"}) {
		t.Fatalf("sort %+v", s.files.sort)
	}
	check("by size", "logs", "zeta", "run.sh", "aa.log", "notes.txt")

	// A second click reverses it.
	if err := tt.Click(locales.ColSize()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if s.files.sort != (ui.SortOrder{Column: "size", Descending: true}) {
		t.Fatalf("sort %+v", s.files.sort)
	}
	// Reversed is the ascending order, reversed: the folders stay first,
	// their sizes equal, so their names go the other way too.
	check("by size, reversed", "zeta", "logs", "notes.txt", "aa.log", "run.sh")

	// The chosen row follows its file as another column moves it.
	s.files.chosen.Clear()
	s.files.chosen.Add("run.sh")
	s.files.row, s.files.rowName = 4, "run.sh"
	if err := tt.Click(locales.ColModified()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	check("by modified", "logs", "zeta", "run.sh", "notes.txt", "aa.log")
	tt.Frame()
	if s.files.row != 2 || s.files.shown()[s.files.row].Name != "run.sh" {
		t.Errorf("the choice at row %d", s.files.row)
	}
	if got := s.files.chosenNames(); !slices.Equal(got, []string{"run.sh"}) {
		t.Errorf("chosen %v", got)
	}
}
