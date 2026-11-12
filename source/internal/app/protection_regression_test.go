package app

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestFinishedRestartOfflineIPConcurrentSettlementAndNextQueue(t *testing.T) {
	s, u, _, p, sp, id := finishedJob(t)
	next := newJob(t, s, u, p, sp, 10)
	cfg := s.Config
	s.Close()
	driver := &fakeDevice{}
	reopened, e := Open(cfg, driver)
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.Close()
	reopened.Config.DeviceMode = "live"
	driver.mu.Lock()
	driver.statuses[p.ID] = Telemetry{}
	driver.mu.Unlock()
	if e = reopened.SavePrinter(u, PrinterInput{ID: p.ID, IP: "192.0.2.19"}); e != nil {
		t.Fatal(e)
	}
	driver.mu.Lock()
	driver.statuses[p.ID] = Telemetry{}
	driver.mu.Unlock()
	input := SettlementWrite{JobID: id, Collected: true, Uses: map[string]SettlementUse{sp.ID: {Source: "measured", Grams: 20.25}}}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); results <- reopened.SettleJob(u, input) }()
	}
	wg.Wait()
	close(results)
	accepted := 0
	for err := range results {
		if err == nil {
			accepted++
		}
	}
	if accepted != 1 {
		t.Fatal("concurrent settlement accepted more than once", accepted)
	}
	check := func() error {
		return reopened.transaction(func(tx *sql.Tx) error { _, err := reopened.executable(tx, u, next, "queued"); return err })
	}
	if check() == nil {
		t.Fatal("offline next job executable")
	}
	driver.mu.Lock()
	driver.statuses[p.ID] = Telemetry{Connected: true, Updated: time.Now(), State: "idle", RawState: "IDLE"}
	driver.mu.Unlock()
	if e = check(); e != nil {
		t.Fatal("next queue remains locked after settlement and reconnect", e)
	}
	stock, _ := readSpools(reopened.DB)
	if stock[0].Remaining != 979 {
		t.Fatal(stock)
	}
	var n int
	reopened.DB.QueryRow("SELECT COUNT(*) FROM printer_commands").Scan(&n)
	if n != 0 || driver.uploads != 0 {
		t.Fatal("restart/recovery/settlement dispatched a command")
	}
}

func TestSettlementVersionFailureRollsBackAccounting(t *testing.T) {
	s, u, _, _, sp, id := finishedJob(t)
	if _, e := s.DB.Exec("CREATE TRIGGER fail_version BEFORE UPDATE ON record_versions BEGIN SELECT RAISE(ABORT,'injected version failure'); END;"); e != nil {
		t.Fatal(e)
	}
	if e := s.SettleJob(u, SettlementWrite{JobID: id, Collected: true, Uses: map[string]SettlementUse{sp.ID: {Source: "measured", Grams: 20}}}); e == nil {
		t.Fatal("injected failure ignored")
	}
	var remaining, n int
	s.DB.QueryRow("SELECT remaining FROM spools WHERE id=?", sp.ID).Scan(&remaining)
	s.DB.QueryRow("SELECT COUNT(*) FROM job_consumption").Scan(&n)
	if remaining != 1000 || n != 0 {
		t.Fatal("partial accounting committed", remaining, n)
	}
}

func TestSettlementInvalidatesPreviouslyOpenedSpoolEditor(t *testing.T) {
	s, u, _, _, sp, id := finishedJob(t)
	if e := s.SettleJob(u, SettlementWrite{JobID: id, Collected: true, Uses: map[string]SettlementUse{sp.ID: {Source: "measured", Grams: 20}}}); e != nil {
		t.Fatal(e)
	}
	sp.Label = "Edited after settlement"
	if e := s.SaveSpool(u, sp); e == nil {
		t.Fatal("BUG: stale spool editor restored consumed inventory")
	}
	stock, _ := readSpools(s.DB)
	if stock[0].Remaining != 980 {
		t.Fatal(stock)
	}
}
func TestOrphanSettlementInvalidatesPreviouslyOpenedSpoolEditor(t *testing.T) {
	s, u, _, p, sp, id := orphanFixture(t)
	if e := s.SettleOrphanCompletion(u, OrphanCompletionInput{PrinterID: p.ID, JobID: id, ConfirmTask: "job-1", Collected: true, Uses: map[string]float64{sp.ID: 12.125}, Note: "test actual usage"}); e != nil {
		t.Fatal(e)
	}
	if e := s.SaveSpool(u, sp); e == nil {
		t.Fatal("BUG: stale spool editor overwrote orphan consumption")
	}
}
func regressionPost(t *testing.T, s *Service, u User, path string, in any) *httptest.ResponseRecorder {
	t.Helper()
	token := randomID(32)
	_, e := s.DB.Exec("INSERT INTO sessions(hash,user_id,expires) VALUES(?,?,?)", tokenHash(token), u.ID, time.Now().Add(time.Hour).Unix())
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(in)
	r := httptest.NewRequest("POST", path, bytes.NewReader(raw))
	r.AddCookie(&http.Cookie{Name: "layerhub_sid", Value: token})
	r.Header.Set("X-CSRF-Token", s.csrf(token))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}
func TestLegacyCompletionCannotBypassFinishAndCollectionConfirmation(t *testing.T) {
	s, u, d := setup(t)
	p, sp := fixture(t, s, u)
	id := newJob(t, s, u, p, sp, 25)
	s.Config.DeviceMode = "live"
	s.DB.Exec("UPDATE jobs SET state='printing',remote_path='owned-task.3mf' WHERE id=?", id)
	now := time.Now()
	d.mu.Lock()
	d.statuses[p.ID] = Telemetry{Connected: true, Updated: now, StateUpdated: now, TaskUpdated: now, State: "idle", RawState: "FINISH", File: "unrelated-task"}
	d.mu.Unlock()
	w := regressionPost(t, s, u, "/api/jobs/complete", map[string]any{"jobId": id, "used": 20})
	if w.Code != http.StatusGone {
		t.Fatalf("BUG: legacy endpoint bypassed settlement: status=%d %s", w.Code, w.Body.String())
	}
	var count int
	s.DB.QueryRow("SELECT COUNT(*) FROM job_consumption").Scan(&count)
	if count != 0 {
		t.Fatal("stock altered")
	}
}
func TestExistingSpoolHTTPRequiresRevision(t *testing.T) {
	s, u, _ := setup(t)
	_, sp := fixture(t, s, u)
	sp.Revision = 0
	w := regressionPost(t, s, u, "/api/spools", sp)
	if w.Code != http.StatusConflict {
		t.Fatalf("BUG: unversioned edit accepted: %d %s", w.Code, w.Body.String())
	}
}
