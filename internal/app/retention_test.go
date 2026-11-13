package app

import (
	"encoding/json"
	"testing"
	"time"

	"finallyshell/internal/config"

	"github.com/egoist/mygo/ui"
)

// TestPurgeDeletedHosts deletes for good only what was deleted longer ago
// than the retention, with the password and the history kept for it.
func TestPurgeDeletedHosts(t *testing.T) {
	a, _ := testApp(t)
	saved := savedHosts(a)
	now := time.Now()
	old := &config.Host{ID: "old", Name: "old-box", Addr: "10.0.0.1", DeletedAt: now.Add(-config.DeletedRetention - time.Hour)}
	recent := &config.Host{ID: "recent", Name: "recent-box", Addr: "10.0.0.2", DeletedAt: now.Add(-config.DeletedRetention + time.Hour)}
	live := &config.Host{ID: "live", Name: "live-box", Addr: "10.0.0.3"}
	for _, h := range []*config.Host{old, recent, live} {
		a.hosts.Add(h)
	}
	a.passwords["old"] = "pw"
	if err := config.AddHistory("old", "uptime"); err != nil {
		t.Fatal(err)
	}
	a.history.pending.Wait()

	if n := a.purgeDeletedHosts(now); n != 1 {
		t.Errorf("purged %d", n)
	}
	if a.hosts.Host("old") != nil {
		t.Error("the host past the retention is still there")
	}
	if a.hosts.Host("recent") == nil || a.hosts.Host("live") == nil {
		t.Errorf("hosts past: recent %v live %v", a.hosts.Host("recent"), a.hosts.Host("live"))
	}
	if _, ok := a.passwords["old"]; ok {
		t.Error("the password is kept")
	}
	a.history.pending.Wait()
	if list, err := config.LoadHistory("old"); err != nil || len(list) != 0 {
		t.Errorf("history %q, %v", list, err)
	}
	var file struct {
		Hosts []struct {
			ID string `json:"id"`
		} `json:"hosts"`
	}
	if err := json.Unmarshal(*saved, &file); err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, h := range file.Hosts {
		ids = append(ids, h.ID)
	}
	if len(ids) != 2 || ids[0] != "recent" || ids[1] != "live" {
		t.Errorf("saved %v", ids)
	}
	if !a.retention.next.After(now) {
		t.Errorf("the next check is at %v", a.retention.next)
	}
}

// TestRetentionAtFirstFrame purges what is past the retention before the
// main window lists the hosts, and says how many went.
func TestRetentionAtFirstFrame(t *testing.T) {
	a, p := testApp(t)
	now := time.Now()
	a.hosts.Add(&config.Host{ID: "old", Name: "old-box", Addr: "10.0.0.1", DeletedAt: now.Add(-config.DeletedRetention - time.Hour)})
	a.hosts.Add(&config.Host{ID: "live", Name: "live-box", Addr: "10.0.0.2"})
	tt := ui.NewTester(a.View, 1280, 820)
	p.tt = tt
	tt.Frame()
	if a.hosts.Host("old") != nil || a.hosts.Host("live") == nil {
		t.Errorf("hosts %v", a.hosts.Hosts)
	}
	// The next check is a day on, not the next frame.
	a.hosts.Add(&config.Host{ID: "older", Name: "older-box", Addr: "10.0.0.3", DeletedAt: now.Add(-config.DeletedRetention - time.Hour)})
	tt.Frame()
	if a.hosts.Host("older") == nil {
		t.Error("purged again before the next check")
	}
}
