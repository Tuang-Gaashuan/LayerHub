package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPrintNotificationLifecycleX2DAndDeduplication(t *testing.T) {
	s, u, d := setup(t)
	p, sp := fixture(t, s, u)
	id := newJob(t, s, u, p, sp, 38)
	other := notificationMember(t, s, u)
	bound(t, s, u, "owner@example.com")
	bound(t, s, other, "other@example.com")
	remote := fmt.Sprintf("job-%d-unique.3mf", id)
	if _, err := s.DB.Exec("UPDATE jobs SET state='printing',remote_path=? WHERE id=?", remote, id); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	telemetry := Telemetry{Connected: true, Updated: now, StateUpdated: now, TaskUpdated: now, State: "printing", RawState: "PREPARE", File: fmt.Sprintf("job-%d", id), GcodeFile: "/data/Metadata/plate_1.gcode"}
	collect := func(raw string) {
		t.Helper()
		telemetry.RawState = raw
		d.mu.Lock()
		d.statuses[p.ID] = telemetry
		d.mu.Unlock()
		if err := s.collectMail(now); err != nil {
			t.Fatal(err)
		}
	}
	count := func(want int) {
		t.Helper()
		var n int
		if err := s.DB.QueryRow("SELECT COUNT(*) FROM mail_outbox WHERE kind='notification'").Scan(&n); err != nil || n != want {
			t.Fatalf("mail count=%d want=%d err=%v", n, want, err)
		}
	}
	collect("PREPARE")
	count(0)
	collect("RUNNING")
	count(1)
	collect("RUNNING")
	// A resume, lost connection or repeated status cannot repeat the start mail.
	telemetry.Connected = false
	collect("RUNNING")
	telemetry.Connected = true
	collect("RUNNING")
	count(1)
	telemetry.State = "idle"
	collect("FINISH")
	count(2)
	collect("FINISH")
	telemetry.Connected = false
	collect("FINISH")
	telemetry.Connected = true
	collect("FINISH")
	count(2)
	var n int
	s.DB.QueryRow("SELECT COUNT(*) FROM user_notifications WHERE user_id=?", other.ID).Scan(&n)
	if n != 0 {
		t.Fatal("another member received private task notices")
	}
	for _, category := range []string{"started", "completed"} {
		s.DB.QueryRow("SELECT COUNT(*) FROM user_notifications WHERE user_id=? AND category=?", u.ID, category).Scan(&n)
		if n != 1 {
			t.Fatal("wrong milestone category", category, n)
		}
	}
	first, _, err := s.claimMail(true)
	if err != nil || first == nil || first.To != "owner@example.com" || !strings.Contains(first.Subject, "打印") || !strings.Contains(first.Text, p.Name) || !strings.Contains(first.Text, "same-name.gcode") {
		t.Fatal("mail lacks recipient, printer or task context", err)
	}
	var remaining int
	s.DB.QueryRow("SELECT remaining FROM spools WHERE id=?", sp.ID).Scan(&remaining)
	var state string
	s.DB.QueryRow("SELECT state FROM jobs WHERE id=?", id).Scan(&state)
	if state != "printing" || remaining != 1000 || d.uploads != 0 {
		t.Fatal("notifier changed job, stock or printer")
	}
}

func TestPrintNotificationsRejectStaleOrForeignTaskReports(t *testing.T) {
	now := time.Now()
	j := Job{ID: 7, Remote: "job-7-unique.3mf"}
	good := Telemetry{Connected: true, Updated: now, StateUpdated: now, TaskUpdated: now, File: "job-7", GcodeFile: "/data/Metadata/plate_1.gcode"}
	for _, tc := range []struct {
		name string
		edit func(*Telemetry)
		want bool
	}{
		{"X2D job name with inner plate path", func(t *Telemetry) {}, true},
		{"legacy archive name", func(t *Telemetry) { t.File = j.Remote }, true},
		{"legacy gcode only", func(t *Telemetry) { t.File = ""; t.GcodeFile = "/data/" + j.Remote }, true},
		{"generic inner plate path", func(t *Telemetry) { t.File = "" }, false},
		{"different job", func(t *Telemetry) { t.File = "job-8" }, false},
		{"contradictory old archive path", func(t *Telemetry) { t.File = "job-8"; t.GcodeFile = j.Remote }, false},
		{"disconnected", func(t *Telemetry) { t.Connected = false }, false},
		{"stale state", func(t *Telemetry) { t.StateUpdated = now.Add(-2 * time.Minute) }, false},
		{"stale task identity", func(t *Telemetry) { t.TaskUpdated = now.Add(-2 * time.Minute) }, false},
		{"stale telemetry", func(t *Telemetry) { t.Updated = now.Add(-2 * time.Minute) }, false},
		{"missing task timestamp", func(t *Telemetry) { t.TaskUpdated = time.Time{} }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := good
			tc.edit(&got)
			if mailTaskMatches(j, got) != tc.want {
				t.Fatal("task identity or freshness check failed")
			}
		})
	}
}

func TestPrintFinishedCatchesUpOnceButDoesNotRepeatLegacyNotice(t *testing.T) {
	for _, previous := range []string{"printing", "device_finished"} {
		t.Run(previous, func(t *testing.T) {
			s, u, d := setup(t)
			p, sp := fixture(t, s, u)
			id := newJob(t, s, u, p, sp, 5)
			bound(t, s, u, "owner@example.com")
			s.DB.Exec("UPDATE jobs SET state='printing',remote_path='unique.3mf' WHERE id=?", id)
			s.DB.Exec("INSERT INTO mail_observations(key,value,candidate,since) VALUES(?,?,?,0)", fmt.Sprintf("job:%d", id), previous, previous)
			now := time.Now()
			d.statuses[p.ID] = Telemetry{Connected: true, Updated: now, StateUpdated: now, TaskUpdated: now, State: "idle", RawState: "FINISH", File: fmt.Sprintf("job-%d", id), GcodeFile: "/data/Metadata/plate_1.gcode"}
			if err := s.collectMail(now); err != nil {
				t.Fatal(err)
			}
			// Durable milestone survives restarts and even notice retention cleanup.
			s.DB.Exec("DELETE FROM user_notifications")
			s.DB.Exec("UPDATE mail_observations SET value='printing' WHERE key=?", fmt.Sprintf("job:%d", id))
			if err := s.initMail(); err != nil {
				t.Fatal(err)
			}
			if err := s.collectMail(now); err != nil {
				t.Fatal(err)
			}
			want := 1
			if previous == "device_finished" {
				want = 0
			}
			var n int
			s.DB.QueryRow("SELECT COUNT(*) FROM mail_outbox").Scan(&n)
			if n != want {
				t.Fatalf("completion catch-up mail=%d want=%d", n, want)
			}
		})
	}
}

func TestStartedPreferenceMigrationAndCancellation(t *testing.T) {
	s, u, _ := setup(t)
	bound(t, s, u, "owner@example.com")
	// Reproduce the previous four-category schema with an existing opt-out.
	if _, err := s.DB.Exec("INSERT INTO notification_preferences(user_id,completed,attention,devices,stock,revision) VALUES(?,0,1,0,1,4)", u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec("ALTER TABLE notification_preferences DROP COLUMN started"); err != nil {
		t.Fatal(err)
	}
	if err := s.initMail(); err != nil {
		t.Fatal(err)
	}
	p, err := readNotificationPreferences(s.DB, u.ID)
	if err != nil || !p.Started || p.Completed || p.Devices || p.Revision != 4 {
		t.Fatal("migration changed existing preferences", p, err)
	}
	notice(t, s, u, "job:1:started", "LayerHub · 打印已开始")
	p.Started = false
	if err = s.saveNotificationPreferences(u, p); err != nil {
		t.Fatal(err)
	}
	var state string
	s.DB.QueryRow("SELECT state FROM mail_outbox").Scan(&state)
	if state != "cancelled" {
		t.Fatal("start opt-out did not cancel queued mail", state)
	}
	notice(t, s, u, "job:2:started", "LayerHub · 打印已开始")
	var n int
	s.DB.QueryRow("SELECT COUNT(*) FROM mail_outbox").Scan(&n)
	if n != 1 {
		t.Fatal("start opt-out still queues mail")
	}
	s.DB.QueryRow("SELECT COUNT(*) FROM user_notifications WHERE category='started'").Scan(&n)
	if n != 2 {
		t.Fatal("start opt-out lost station notifications")
	}
}

func TestOldPreferencePageDoesNotChangeStartedCategory(t *testing.T) {
	s, u, _ := setup(t)
	token := randomID(32)
	if _, err := s.DB.Exec("INSERT INTO sessions(hash,user_id,expires) VALUES(?,?,?)", tokenHash(token), u.ID, time.Now().Add(time.Hour).Unix()); err != nil {
		t.Fatal(err)
	}
	// Both true and false must survive a save from the previous four-category UI.
	for _, started := range []bool{true, false} {
		p, err := readNotificationPreferences(s.DB, u.ID)
		if err != nil {
			t.Fatal(err)
		}
		p.Started = started
		if err = s.saveNotificationPreferences(u, p); err != nil {
			t.Fatal(err)
		}
		p, _ = readNotificationPreferences(s.DB, u.ID)
		payload, _ := json.Marshal(map[string]any{"completed": false, "attention": true, "devices": true, "stock": true, "revision": p.Revision})
		r := httptest.NewRequest("POST", "/api/account/notification-preferences", strings.NewReader(string(payload)))
		r.AddCookie(&http.Cookie{Name: "layerhub_sid", Value: token})
		r.Header.Set("X-CSRF-Token", s.csrf(token))
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatal(w.Code, w.Body.String())
		}
		p, _ = readNotificationPreferences(s.DB, u.ID)
		if p.Started != started || p.Completed {
			t.Fatal("old browser changed unrelated start preference", p)
		}
	}
}
