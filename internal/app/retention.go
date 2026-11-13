package app

import (
	"time"

	"finallyshell/internal/config"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// Deleted connections are deleted for good once their deletion is past
// config.DeletedRetention: at the first frame, before the windows show the
// hosts, and then once a day while the app runs.

// retentionCheckEvery is how often the app looks for them while it runs.
const retentionCheckEvery = 24 * time.Hour

// retentionState is when the deleted connections are next looked at.
type retentionState struct{ next time.Time }

// retentionTick deletes the connections past the retention, from the main
// window's view as syncTick runs the automatic sync: on the first frame,
// then once a day. It asks for the frame of its next check.
func (a *App) retentionTick(c *ui.Context) {
	now := c.Now()
	if wait := a.retention.next.Sub(now); wait > 0 {
		c.After(wait)
		return
	}
	a.purgeDeletedHosts(now)
	c.After(retentionCheckEvery)
}

// purgeDeletedHosts deletes for good the connections deleted longer ago
// than config.DeletedRetention, each as deleteHostForever does for one:
// with the passwords and the command history the app keeps for it. It
// saves once, and says in a toast how many went. It returns how many.
func (a *App) purgeDeletedHosts(now time.Time) int {
	a.retention.next = now.Add(retentionCheckEvery)
	expired := a.hosts.DeletedBefore(now.Add(-config.DeletedRetention))
	if len(expired) == 0 {
		return 0
	}
	for _, h := range expired {
		a.forgetHost(h)
	}
	// Their rows are gone: nothing stays chosen, and the keys move from no
	// row.
	for _, m := range []*managerState{&a.manager, &a.connTree} {
		m.sel.Clear()
		m.row = -1
	}
	if err := a.saveHosts(a.hosts); err != nil {
		a.notify(locales.ErrSaveFailed(err.Error()))
	} else {
		a.notify(locales.DeletedPurged(len(expired)))
	}
	a.redraw()
	return len(expired)
}
