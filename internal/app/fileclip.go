package app

import (
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"finallyshell/internal/remotefs"
	"finallyshell/locales"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	"github.com/pkg/sftp"
)

// fileClipFormat marks the system clipboard as holding what a file panel
// copied. It is the app's own format, so that files copied on this
// computer before don't win at Paste, and a copy made elsewhere drops the
// clip, as the clipboard holds one thing at a time.
const fileClipFormat transfer.Format = "application/x-finallyshell-file-clip"

// fileClip is what Copy took from a file panel: names in the folder dir of
// a session's server, and the copies it makes of them for this computer,
// in stageDir, so that a file manager can paste them too.
type fileClip struct {
	sess  *Session
	dir   string
	names []string

	stageDir string
	stage    []*transferJob
}

// copyNames copies the names in the remote folder dir to the clipboard, to
// paste into a folder of any session, and copies them to this computer in
// the background, to paste into a file manager.
func (a *App) copyNames(c *ui.Context, s *Session, dir string, names []string) {
	if s.sftp == nil || dir == "" || len(names) == 0 {
		return
	}
	// What the clipboard held before needs no copy for this computer now.
	if old := a.fileClip; old != nil {
		for _, t := range old.stage {
			if !t.finished() {
				t.stop()
			}
		}
	}
	a.fileClip = &fileClip{sess: s, dir: dir, names: slices.Clone(names)}
	clip := a.fileClip
	if c != nil {
		c.Services().WriteClipboardData(transfer.New(transfer.NewItem(
			transfer.Bytes(fileClipFormat, []byte(strings.Join(names, "\n"))))))
		// Without a system clipboard there is nowhere to paste them.
		if slices.Contains(c.Services().ClipboardFormats(), fileClipFormat) {
			a.stageClip(clip)
		}
	}
}

// copyChosen copies the rows chosen in the file panel of s.
func (a *App) copyChosen(c *ui.Context, s *Session) {
	a.copyNames(c, s, s.files.cwd, s.files.chosenNames())
}

// clipAvailable reports whether what a panel copied can still be read: its
// session is open, with SFTP.
func (a *App) clipAvailable() bool {
	clip := a.fileClip
	return clip != nil && slices.Contains(a.sessions, clip.sess) && clip.sess.sftp != nil
}

// canPaste reports whether a paste into a file panel has anything to
// paste: files a panel copied, or files copied on this computer.
func (a *App) canPaste(c *ui.Context) bool {
	formats := c.Services().ClipboardFormats()
	if slices.Contains(formats, transfer.FileList) {
		return true // the copies a panel fetched, or an app of this computer's
	}
	if len(formats) == 0 || slices.Contains(formats, fileClipFormat) {
		return a.clipAvailable()
	}
	return false
}

// clipboardFiles are the files copied on this computer, if the clipboard
// holds a file list, as Explorer and Finder write.
func clipboardFiles(c *ui.Context) []string {
	data := c.Services().ReadClipboardData(transfer.FileList)
	if len(data.Items()) == 0 {
		return nil
	}
	files, err := data.Files()
	if err != nil {
		return nil
	}
	return files
}

// pasteInto pastes into the remote folder dir what the clipboard holds:
// the files a panel copied, which are sent from the server that holds
// them, or files of this computer, which are uploaded: those another app
// copied, or those a panel copied and fetched here.
func (a *App) pasteInto(c *ui.Context, s *Session, dir string) {
	if s.sftp == nil || dir == "" {
		return
	}
	formats := c.Services().ClipboardFormats()
	marked := len(formats) == 0 || slices.Contains(formats, fileClipFormat)
	if marked && a.clipAvailable() {
		a.copyInto(s, a.fileClip, dir)
		return
	}
	// What a panel copied is gone, or the clipboard went on: what is
	// there is a list of files of this computer.
	a.fileClip = nil
	if files := clipboardFiles(c); len(files) > 0 {
		a.upload(s, files, dir)
		return
	}
	if marked {
		a.notify(locales.PasteUnavailable())
	}
}

// copyInto queues a copy of each name of clip into dir, sent through the
// app from the server that holds them, whichever session that is, so that
// it shows in the Transfers tab with the other transfers. A copy into the
// folder it comes from takes a name of its own, as Finder and Explorer
// give one, rather than being asked to replace itself.
func (a *App) copyInto(s *Session, clip *fileClip, dir string) {
	sc, src := s.sftp, clip.sess
	from := src.sftp
	inPlace := src == s && clip.dir == dir
	names := slices.Clone(clip.names)
	go func() {
		as := make([]string, len(names))
		for i, n := range names {
			as[i] = n
			if inPlace {
				as[i] = copyName(sc, dir, n)
			}
		}
		var taken []string
		if !inPlace {
			taken = remotefs.RemoteExisting(sc, dir, as)
		}
		a.post(func() {
			start := func() {
				for i, n := range names {
					a.enqueue(&transferJob{
						sess: s, sftp: sc, from: from, srcSess: src,
						dir: remoteCopy, src: path.Join(clip.dir, n), dstDir: dir, as: as[i],
					})
				}
			}
			if len(taken) > 0 {
				a.replace = &replaceQuestion{count: len(taken), dir: dir, yes: start}
				return
			}
			start()
		})
	}()
}

// stageClip copies what clip holds into a folder of the app's, for this
// computer's file manager to paste: a file manager pastes files of this
// computer, where the names are of a server. It goes as transfers of its
// own, so that the Transfers tab shows how far it is, and can cancel it.
func (a *App) stageClip(clip *fileClip) {
	sc := clip.sess.sftp
	if sc == nil {
		return
	}
	root := a.plat.TempDir()
	if root == "" {
		return
	}
	removeOld(root, 24*time.Hour)
	dir := filepath.Join(root, strconv.FormatInt(time.Now().UnixNano(), 10))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	clip.stageDir = dir
	for _, n := range clip.names {
		clip.stage = append(clip.stage, &transferJob{
			sess: clip.sess, sftp: sc, dir: download, clip: clip,
			src: path.Join(clip.dir, n), dstDir: dir,
		})
	}
	for _, t := range clip.stage {
		a.enqueue(t)
	}
}

// clipStaged, as the last of a clip's copies for this computer ends, puts
// the file list of them on the clipboard, which a file manager pastes,
// with the app's mark, which keeps the app's own Paste a copy on the
// server. A copy that is no longer the clipboard's, or a clipboard taken
// by another app, keeps it as it is.
func (a *App) clipStaged(t *transferJob) {
	clip := t.clip
	paths := make([]string, 0, len(clip.stage))
	for _, j := range clip.stage {
		if !j.finished() {
			return
		}
		if j.state != transferDone {
			return
		}
		paths = append(paths, j.local)
	}
	if clip != a.fileClip || len(paths) == 0 {
		return
	}
	if !slices.Contains(a.services.ClipboardFormats(), fileClipFormat) {
		return
	}
	data := clipData(paths)
	if len(data.Items()) == 0 {
		return
	}
	a.services.WriteClipboardData(data)
	a.notify(locales.CopyToComputerReady())
}

// clipData is what the clipboard holds for the files at paths: the file
// list a file manager pastes, and the app's mark.
func clipData(paths []string) transfer.Data {
	files, err := transfer.FileData(paths...)
	if err != nil {
		return transfer.Data{}
	}
	items := append(files.Items(), transfer.NewItem(transfer.Bytes(fileClipFormat, nil)))
	return transfer.New(items...)
}

// copyName is the name a copy of name takes in dir, where name already
// is: the name with the copy marker of the locale before its extension,
// " (copy 2)" and so on when those are taken too.
func copyName(sc *sftp.Client, dir, name string) string {
	stem, ext := name, ""
	if e := path.Ext(name); e != name {
		stem, ext = strings.TrimSuffix(name, e), e
	}
	for n := 1; ; n++ {
		as := locales.CopyOfName(stem)
		if n > 1 {
			as = locales.CopyOfNameN(stem, n)
		}
		as += ext
		if _, err := sc.Lstat(path.Join(dir, as)); err != nil {
			return as
		}
	}
}
