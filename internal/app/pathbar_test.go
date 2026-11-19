package app

import (
	"slices"
	"testing"

	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

func TestPathCrumbs(t *testing.T) {
	for _, tc := range []struct {
		dir  string
		want []crumb
	}{
		{"/", []crumb{{"/", "/"}}},
		{"/srv", []crumb{{"/", "/"}, {"srv", "/srv"}}},
		{"/srv/app/bin", []crumb{{"/", "/"}, {"srv", "/srv"}, {"app", "/srv/app"}, {"bin", "/srv/app/bin"}}},
		{"/srv/app/", []crumb{{"/", "/"}, {"srv", "/srv"}, {"app", "/srv/app"}}},
		{"/srv//app/./bin", []crumb{{"/", "/"}, {"srv", "/srv"}, {"app", "/srv/app"}, {"bin", "/srv/app/bin"}}},
	} {
		if got := pathCrumbs(tc.dir); !slices.Equal(got, tc.want) {
			t.Errorf("pathCrumbs(%q) = %v, want %v", tc.dir, got, tc.want)
		}
	}
}

// The crumbs left out for room are the leading ones: the deepest folder,
// the one the bar names, stays.
func TestFitCrumbs(t *testing.T) {
	crumbs := pathCrumbs("/srv/app/bin")
	var all, one []crumb
	var moreAll, moreOne bool
	ui.NewTester(func(c *ui.Context) {
		all, moreAll = fitCrumbs(c, crumbs, 1000)
		one, moreOne = fitCrumbs(c, crumbs, 1)
	}, 400, 100).Frame()
	if !slices.Equal(all, crumbs) || moreAll {
		t.Errorf("wide: %v, more %v; want every crumb", all, moreAll)
	}
	if !slices.Equal(one, crumbs[len(crumbs)-1:]) || !moreOne {
		t.Errorf("narrow: %v, more %v; want the deepest and more", one, moreOne)
	}
}

// A click on the folder the panel is in or on the field, and Escape: the
// path becomes a field to type in, and goes back to crumbs.
func TestPathBarEditing(t *testing.T) {
	a, _ := testApp(t)
	s := demoSession(a, "files", "10.0.0.21")
	a.active, a.bottomTab = 0, tabFiles
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Frame()
	if s.files.pathEditing {
		t.Fatal("the path opens as a field")
	}
	if err := tt.Click("/lib64"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !s.files.pathEditing {
		t.Fatal("the current folder did not show the path")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if s.files.pathEditing {
		t.Fatal("Escape did not close the field")
	}
	if err := tt.Click(locales.EditPath()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !s.files.pathEditing {
		t.Fatal("the field did not show the path")
	}
}
