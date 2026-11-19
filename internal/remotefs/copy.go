package remotefs

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"strings"

	"github.com/pkg/sftp"
)

// ErrSelfCopy is a copy onto the file it copies.
var ErrSelfCopy = errors.New("cannot copy onto itself")

// ErrIntoItself is a folder copied into itself or into a folder it holds.
var ErrIntoItself = errors.New("cannot copy a folder into itself")

// Copy copies the remote file or folder src into the remote directory
// dstDir of the same server, as dstDir/<name of src>, replacing files that
// are there, and returns its path.
func Copy(ctx context.Context, c *sftp.Client, src, dstDir string, progress Progress) (string, error) {
	return CopyTo(ctx, c, c, src, dstDir, "", progress)
}

// CopyTo copies the remote file or folder src of one server into the
// remote directory dstDir of another (the same one, for a copy in place),
// as dstDir/as, or as its own name for "". It replaces files that are
// there and returns the path it copied to. Links are copied as links, not
// followed, so a link to a folder never loops.
func CopyTo(ctx context.Context, from, to *sftp.Client, src, dstDir, as string, progress Progress) (string, error) {
	var done int64
	cnt := &counter{ctx: ctx, done: &done, progress: progress}
	if as == "" {
		as = path.Base(src)
	}
	dst := path.Join(dstDir, as)
	if src == dst {
		return "", ErrSelfCopy
	}
	fi, err := from.Lstat(src)
	if err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if fi.Mode()&fs.ModeSymlink != 0 {
		return dst, copyLink(from, to, src, dst)
	}
	if fi.IsDir() {
		// A copy walked into a folder it holds would keep finding what
		// it copies, and never end.
		if from == to && (dst == src || strings.HasPrefix(dst, strings.TrimSuffix(src, "/")+"/")) {
			return "", ErrIntoItself
		}
		return dst, copyTree(ctx, from, to, src, dst, fi.Mode().Perm(), cnt)
	}
	return dst, copyRemoteFile(from, to, src, dst, fi, cnt)
}

// copyLink makes the link dst as the link src is, at the same target.
func copyLink(from, to *sftp.Client, src, dst string) error {
	target, err := from.ReadLink(src)
	if err != nil {
		return err
	}
	to.Remove(dst) // a link cannot replace a name that is taken
	if err := to.Symlink(target, dst); err != nil {
		return fmt.Errorf("%s: %w", dst, err)
	}
	return nil
}

// copyTree copies the folder src into the folder dst, which it makes.
func copyTree(ctx context.Context, from, to *sftp.Client, src, dst string, perm fs.FileMode, cnt *counter) error {
	if err := to.MkdirAll(dst); err != nil {
		return fmt.Errorf("%s: %w", dst, err)
	}
	to.Chmod(dst, perm) // best effort: some servers refuse it
	infos, err := from.ReadDir(src)
	if err != nil {
		return err
	}
	for _, fi := range infos {
		if err := ctx.Err(); err != nil {
			return err
		}
		sp, dp := path.Join(src, fi.Name()), path.Join(dst, fi.Name())
		switch {
		case fi.Mode()&fs.ModeSymlink != 0:
			if err := copyLink(from, to, sp, dp); err != nil {
				return err
			}
		case fi.IsDir():
			if err := copyTree(ctx, from, to, sp, dp, fi.Mode().Perm(), cnt); err != nil {
				return err
			}
		case fi.Mode().IsRegular():
			if err := copyRemoteFile(from, to, sp, dp, fi, cnt); err != nil {
				return err
			}
		}
	}
	return nil
}

// copyRemoteFile copies the file src into dst, and gives it the
// permissions of src.
func copyRemoteFile(from, to *sftp.Client, src, dst string, fi fs.FileInfo, cnt *counter) (err error) {
	in, err := from.Open(src)
	if err != nil {
		return fmt.Errorf("%s: %w", src, err)
	}
	defer in.Close()
	out, err := to.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return fmt.Errorf("%s: %w", dst, err)
	}
	defer func() {
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			to.Remove(dst) // what was written is a part of the file
		}
	}()
	if _, err = in.WriteTo(&countingWriter{w: out, counter: cnt}); err != nil {
		return err
	}
	to.Chmod(dst, fi.Mode().Perm()) // best effort: some servers refuse it
	return nil
}
