package remotefs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCopy(t *testing.T) {
	c := newTestClient(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.txt"), []byte("hello"))
	writeFile(t, filepath.Join(dir, "sub", "b.txt"), []byte("world"))
	writeFile(t, filepath.Join(dir, "sub", "deep", "c.txt"), []byte("deeper"))
	out := filepath.Join(dir, "out")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}

	// A file, under its own name.
	dst, err := Copy(context.Background(), c, filepath.Join(dir, "a.txt"), out, nil)
	if err != nil || dst != filepath.Join(out, "a.txt") {
		t.Fatalf("copy a.txt: %q %v", dst, err)
	}
	if got, err := os.ReadFile(dst); err != nil || string(got) != "hello" {
		t.Errorf("a.txt holds %q %v", got, err)
	}

	// A folder with everything in it, and a name of its own.
	dst, err = CopyTo(context.Background(), c, c, filepath.Join(dir, "sub"), out, "sub2", nil)
	if err != nil || dst != filepath.Join(out, "sub2") {
		t.Fatalf("copy sub: %q %v", dst, err)
	}
	for name, want := range map[string]string{"b.txt": "world", "deep/c.txt": "deeper"} {
		p := filepath.Join(dst, filepath.FromSlash(name))
		if got, err := os.ReadFile(p); err != nil || string(got) != want {
			t.Errorf("%s holds %q %v", name, got, err)
		}
	}

	// A copy onto the file it copies is refused.
	if _, err := Copy(context.Background(), c, filepath.Join(dir, "a.txt"), dir, nil); !errors.Is(err, ErrSelfCopy) {
		t.Errorf("copy onto itself: %v", err)
	}

	// A canceled copy writes nothing more.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Copy(ctx, c, filepath.Join(dir, "sub"), out, nil); !errors.Is(err, context.Canceled) {
		t.Errorf("a canceled copy: %v", err)
	}
}
