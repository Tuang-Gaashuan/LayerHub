package app

import (
	"database/sql"
	"testing"
	"time"
)

func TestMaintenancePermissionsAndOfflineDebounce(t *testing.T) {
	s, u, driver := setup(t)
	p, _ := fixture(t, s, u)
	bound(t, s, u, "one@example.com")
	member := notificationMember(t, s, u)
	if s.setMaintenance(member, 60, 0) == nil {
		t.Fatal("member changed global maintenance")
	}
	if _, err := s.maintenanceStatus(member); err == nil {
		t.Fatal("member read global maintenance")
	}
	if s.setMaintenance(u, 99999, 0) == nil {
		t.Fatal("unbounded maintenance")
	}
	if err := s.setMaintenance(u, 30, 0); err != nil {
		t.Fatal(err)
	}
	if s.setMaintenance(u, 60, 0) == nil {
		t.Fatal("stale overwrite")
	}
	m, err := readMaintenance(s.DB)
	if err != nil || m.Revision != 1 || m.AckAt != 0 {
		t.Fatal(m, err)
	}
	now := time.Now()
	driver.mu.Lock()
	driver.statuses[p.ID] = Telemetry{Connected: false}
	driver.mu.Unlock()
	for _, at := range []time.Time{now, now.Add(4 * time.Minute)} {
		if err = s.collectMail(at); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	s.DB.QueryRow("SELECT COUNT(*) FROM user_notifications WHERE event_key LIKE 'printer:%'").Scan(&count)
	if count != 0 {
		t.Fatal("maintenance outage reported", count)
	}
	// A real device fault still produces an alert during maintenance.
	driver.mu.Lock()
	driver.statuses[p.ID] = Telemetry{Connected: true, Updated: time.Now(), PrintError: 123}
	driver.mu.Unlock()
	for _, at := range []time.Time{now.Add(5 * time.Minute), now.Add(6 * time.Minute)} {
		if err = s.collectMail(at); err != nil {
			t.Fatal(err)
		}
	}
	s.DB.QueryRow("SELECT COUNT(*) FROM user_notifications WHERE event_key LIKE 'printer:%'").Scan(&count)
	if count != 1 {
		t.Fatal("fault suppressed", count)
	}
	driver.mu.Lock()
	driver.statuses[p.ID] = Telemetry{Connected: false}
	driver.mu.Unlock()
	if err = s.collectMail(now.Add(7 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	// Expiry starts a fresh 3 minute offline window, without replaying downtime.
	expiry := time.Unix(m.Until, 0)
	for _, at := range []time.Time{expiry.Add(time.Second), expiry.Add(182 * time.Second)} {
		if err = s.collectMail(at); err != nil {
			t.Fatal(err)
		}
	}
	s.DB.QueryRow("SELECT COUNT(*) FROM user_notifications WHERE event_key LIKE 'printer:%'").Scan(&count)
	if count != 2 {
		t.Fatal("expiry failed to restore offline notification", count)
	}
	if err = s.setMaintenance(u, 0, 1); err != nil {
		t.Fatal(err)
	}
}

func TestMaintenanceCancelsLeasedOfflineButKeepsCompletion(t *testing.T) {
	s, u, _ := setup(t)
	readyMail(t, s)
	bound(t, s, u, "one@example.com")
	err := s.transaction(func(tx *sql.Tx) error {
		if e := s.notifyUser(tx, u.ID, "printer:1:1", "LayerHub · 打印机持续离线", "offline", time.Now().Unix()); e != nil {
			return e
		}
		return s.notifyUser(tx, u.ID, "job:1:device_finished", "LayerHub · 打印完成", "done", time.Now().Unix())
	})
	if err != nil {
		t.Fatal(err)
	}
	s.DB.Exec("UPDATE mail_outbox SET state='leased',lease='test',lease_until=? WHERE event_key LIKE 'printer:%'", time.Now().Unix()+120)
	if err = s.setMaintenance(u, 60, 0); err != nil {
		t.Fatal(err)
	}
	var state string
	s.DB.QueryRow("SELECT state FROM mail_outbox WHERE event_key LIKE 'printer:%'").Scan(&state)
	if state != "cancelled" {
		t.Fatal("leased offline alert still sendable", state)
	}
	s.DB.QueryRow("SELECT state FROM mail_outbox WHERE event_key LIKE 'job:%'").Scan(&state)
	if state != "queued" {
		t.Fatal("completion notification muted", state)
	}
	if err = scrubMailBackup(s.DB); err != nil {
		t.Fatal(err)
	}
	m, err := readMaintenance(s.DB)
	if err != nil || m.Until != 0 {
		t.Fatal("backup retained maintenance", m, err)
	}
}
