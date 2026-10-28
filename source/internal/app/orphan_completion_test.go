package app

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func orphanFixture(t *testing.T) (*Service, User, *fakeDevice, Printer, Spool, int64) {
	t.Helper()
	s, u, d := setup(t)
	p, sp := fixture(t, s, u)
	id := newJob(t, s, u, p, sp, 10)
	if _, e := s.DB.Exec("INSERT INTO printer_commands(printer_id,job_id,user_id,action,state,created_at) VALUES(?,?,?,'start','reviewed',?)", p.ID, id, u.ID, time.Now().UnixMilli()); e != nil {
		t.Fatal(e)
	}
	if _, e := s.DB.Exec("DELETE FROM jobs WHERE id=?", id); e != nil {
		t.Fatal(e)
	}
	now := time.Now()
	d.statuses[p.ID] = Telemetry{Connected: true, Updated: now, StateUpdated: now, TaskUpdated: now, State: "idle", RawState: "FINISH", Percent: 100, File: fmt.Sprintf("job-%d", id)}
	return s, u, d, p, sp, id
}
func TestOrphanFinishedRequiresExplicitCollectionAndUsage(t *testing.T) {
	s, u, _, p, sp, id := orphanFixture(t)
	snap, e := s.Snapshot(u)
	if e != nil {
		t.Fatal(e)
	}
	v := snap["printers"].([]Printer)[0]
	if v.State != "confirming" || v.CompletionReview == nil {
		t.Fatal(v)
	}
	var stock, n int
	s.DB.QueryRow("SELECT remaining FROM spools WHERE id=?", sp.ID).Scan(&stock)
	if stock != 1000 {
		t.Fatal("automatic deduction", stock)
	}
	next := newJob(t, s, u, p, sp, 10)
	if e = s.SendJob(u, next); e == nil {
		t.Fatal("orphan completion allowed next transfer")
	}
	in := OrphanCompletionInput{PrinterID: p.ID, JobID: id, ConfirmTask: fmt.Sprintf("job-%d", id), Collected: true, Uses: map[string]float64{sp.ID: 12.125}, Note: "User entered actual use"}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- s.SettleOrphanCompletion(u, in) }()
	}
	wg.Wait()
	close(errs)
	ok := 0
	for e := range errs {
		if e == nil {
			ok++
		}
	}
	if ok != 1 {
		t.Fatal("not exactly once", ok)
	}
	s.DB.QueryRow("SELECT remaining FROM spools WHERE id=?", sp.ID).Scan(&stock)
	if stock != 987 {
		t.Fatal(stock)
	}
	s.DB.QueryRow("SELECT count(*) FROM jobs WHERE id=?", id).Scan(&n)
	if n != 0 {
		t.Fatal("invented original job")
	}
	s.DB.QueryRow("SELECT count(*) FROM job_consumption").Scan(&n)
	if n != 0 {
		t.Fatal("invented original consumption record")
	}
	s.DB.QueryRow("SELECT count(*) FROM mail_outbox").Scan(&n)
	if n != 0 {
		t.Fatal("unexpected email")
	}
	snap, e = s.Snapshot(u)
	if e != nil {
		t.Fatal(e)
	}
	v = snap["printers"].([]Printer)[0]
	if v.State != "idle" || v.CompletionReview != nil {
		t.Fatal(v)
	}
}
func TestOrphanSettlementRejectsUnconfirmedStaleAlarmAndBadUsage(t *testing.T) {
	for _, mode := range []string{"not-collected", "wrong-task", "stale", "other-task", "alarm", "pending-command", "negative", "precision", "missing-spool", "too-much", "member"} {
		t.Run(mode, func(t *testing.T) {
			s, u, d, p, sp, id := orphanFixture(t)
			in := OrphanCompletionInput{PrinterID: p.ID, JobID: id, ConfirmTask: fmt.Sprintf("job-%d", id), Collected: true, Uses: map[string]float64{sp.ID: 10}, Note: "Checked"}
			v := d.statuses[p.ID]
			switch mode {
			case "not-collected":
				in.Collected = false
			case "wrong-task":
				in.ConfirmTask = "job-999"
			case "stale":
				v.TaskUpdated = time.Now().Add(-time.Hour)
			case "other-task":
				v.File = "job-999"
			case "alarm":
				v.PrintError = 1
			case "pending-command":
				s.DB.Exec("INSERT INTO printer_commands(printer_id,job_id,user_id,action,state,created_at) VALUES(?,?,?,'set:bed','pending',?)", p.ID, 0, u.ID, time.Now().UnixMilli())
			case "negative":
				in.Uses[sp.ID] = -1
			case "precision":
				in.Uses[sp.ID] = 1.0001
			case "missing-spool":
				in.Uses["bad"] = 1
			case "too-much":
				in.Uses[sp.ID] = 1001
			case "member":
				u.Admin = false
				s.DB.Exec("UPDATE users SET admin=0 WHERE id=?", u.ID)
			}
			d.statuses[p.ID] = v
			if e := s.SettleOrphanCompletion(u, in); e == nil {
				t.Fatal("unsafe reconciliation accepted")
			}
			var stock, n int
			s.DB.QueryRow("SELECT remaining FROM spools WHERE id=?", sp.ID).Scan(&stock)
			s.DB.QueryRow("SELECT count(*) FROM orphan_completions").Scan(&n)
			if stock != 1000 || n != 0 {
				t.Fatal("failed write was not atomic", stock, n)
			}
		})
	}
}

func TestOrphanCompletionHoldSurvivesClearedDeviceTask(t *testing.T) {
	s, u, d, p, sp, id := orphanFixture(t)
	if e := s.collectMail(time.Now()); e != nil {
		t.Fatal(e)
	}
	now := time.Now()
	d.statuses[p.ID] = Telemetry{Connected: true, Updated: now, StateUpdated: now, TaskUpdated: now, State: "idle", RawState: "IDLE"}
	snap, e := s.Snapshot(u)
	if e != nil {
		t.Fatal(e)
	}
	v := snap["printers"].([]Printer)[0]
	if v.CompletionReview == nil || v.State != "confirming" {
		t.Fatal("orphan hold vanished", v)
	}
	next := newJob(t, s, u, p, sp, 10)
	if s.SendJob(u, next) == nil {
		t.Fatal("cleared telemetry bypassed orphan hold")
	}
	if s.SettleOrphanCompletion(u, OrphanCompletionInput{PrinterID: p.ID, JobID: id, ConfirmTask: fmt.Sprintf("job-%d", id), Collected: true, Uses: map[string]float64{sp.ID: 10}, Note: "Check"}) == nil {
		t.Fatal("unknown finish settled")
	}
}
