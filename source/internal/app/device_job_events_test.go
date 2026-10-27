package app

import (
	"fmt"
	"testing"
	"time"
)

func interruptedJob(t *testing.T) (*Service, User, *fakeDevice, Printer, Spool, int64) {
	t.Helper()
	s, u, d := setup(t)
	p, sp := fixture(t, s, u)
	id := newJob(t, s, u, p, sp, 25)
	if _, err := s.DB.Exec("UPDATE jobs SET state='printing',remote_path='unique.3mf' WHERE id=?", id); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	d.statuses[p.ID] = Telemetry{Connected: true, Updated: now, StateUpdated: now, TaskUpdated: now, State: "failed", RawState: "FAILED", File: fmt.Sprintf("job-%d", id)}
	return s, u, d, p, sp, id
}

func TestPhysicalStopSyncKeepsReservationAndNeverSendsCompletionMail(t *testing.T) {
	s, u, _, _, sp, id := interruptedJob(t)
	bound(t, s, u, "owner@example.com")
	other := notificationMember(t, s, u)
	bound(t, s, other, "other@example.com")
	s.DB.Exec("INSERT INTO mail_observations(key,value,candidate,since) VALUES(?,'printing','printing',0)", fmt.Sprintf("job:%d", id))
	for i := 0; i < 3; i++ {
		if err := s.collectMail(time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	jobs, _ := readJobs(s.DB)
	if jobs[0].State != "stopped" || jobs[0].FinishedAt != 0 || reserve(jobs, sp.ID, 0) != 25 {
		t.Fatal("physical stop must remain awaiting collection/usage", jobs)
	}
	var stock, consumed, notices, foreign, completed int
	s.DB.QueryRow("SELECT remaining FROM spools WHERE id=?", sp.ID).Scan(&stock)
	s.DB.QueryRow("SELECT COUNT(*) FROM job_consumption").Scan(&consumed)
	s.DB.QueryRow("SELECT COUNT(*) FROM mail_outbox WHERE user_id=?", u.ID).Scan(&notices)
	s.DB.QueryRow("SELECT COUNT(*) FROM mail_outbox WHERE user_id=?", other.ID).Scan(&foreign)
	s.DB.QueryRow("SELECT COUNT(*) FROM mail_outbox WHERE event_key LIKE '%device_finished%'").Scan(&completed)
	if stock != 1000 || consumed != 0 || notices != 1 || foreign != 0 || completed != 0 {
		t.Fatal(stock, consumed, notices, foreign, completed)
	}
}

func TestStopSettlementCatchesUpBeforeObserverAndRequiresActualUsage(t *testing.T) {
	s, u, _, _, sp, id := interruptedJob(t)
	s.Config.DeviceMode = "live"
	view, err := s.JobSettlement(u, id)
	if err != nil || view.Job.State != "stopped" || view.Job.FinishedAt != 0 || view.Rolls[0].Estimate != nil {
		t.Fatal(view, err)
	}
	in := SettlementWrite{JobID: id, Uses: map[string]SettlementUse{sp.ID: {Source: "measured", Grams: 7.2}}}
	if s.SettleJob(u, in) == nil {
		t.Fatal("collection confirmation bypassed")
	}
	in.Collected = true
	in.Uses[sp.ID] = SettlementUse{Source: "estimated", Grams: 25}
	if s.SettleJob(u, in) == nil {
		t.Fatal("interrupted print used full-plate estimate")
	}
	in.Uses[sp.ID] = SettlementUse{Source: "measured", Grams: 7.2}
	if err = s.SettleJob(u, in); err != nil {
		t.Fatal(err)
	}
	if s.SettleJob(u, in) == nil {
		t.Fatal("settlement was duplicated")
	}
	jobs, _ := readJobs(s.DB)
	var stock int
	s.DB.QueryRow("SELECT remaining FROM spools WHERE id=?", sp.ID).Scan(&stock)
	if jobs[0].State != "cancelled" || stock != 992 || reserve(jobs, sp.ID, 0) != 0 || jobs[0].FinishedAt != 0 {
		t.Fatal(jobs, stock)
	}
}

func TestDirectSettlementSynchronizesPhysicalIdleAndNeverMarksItSuccessful(t *testing.T) {
	s, u, d, p, sp, id := interruptedJob(t)
	s.Config.DeviceMode = "live"
	v := d.statuses[p.ID]
	v.State, v.RawState = "idle", "IDLE"
	d.statuses[p.ID] = v
	in := SettlementWrite{JobID: id, Collected: true, Uses: map[string]SettlementUse{sp.ID: {Source: "measured", Grams: 2}}}
	if err := s.SettleJob(u, in); err != nil {
		t.Fatal(err)
	}
	jobs, _ := readJobs(s.DB)
	if jobs[0].State != "cancelled" || jobs[0].FinishedAt != 0 || jobs[0].ActualGrams == nil || *jobs[0].ActualGrams != 2 {
		t.Fatal(jobs)
	}
}

func TestExternalStopCannotBeInferredFromStaleForeignOrAlreadyFinishedTelemetry(t *testing.T) {
	for _, name := range []string{"offline", "stale telemetry", "stale state", "stale task", "foreign task", "cleared task", "pending", "uncertain", "finished", "unstarted", "normal finish"} {
		t.Run(name, func(t *testing.T) {
			s, u, d, p, _, id := interruptedJob(t)
			v := d.statuses[p.ID]
			switch name {
			case "offline":
				v.Connected = false
			case "stale telemetry":
				v.Updated = time.Now().Add(-2 * time.Minute)
			case "stale state":
				v.StateUpdated = time.Now().Add(-2 * time.Minute)
			case "stale task":
				v.TaskUpdated = time.Now().Add(-2 * time.Minute)
			case "foreign task":
				v.File = "job-999"
			case "cleared task":
				v.File, v.GcodeFile = "", "Metadata/plate_1.gcode"
			case "pending", "uncertain":
				s.DB.Exec("INSERT INTO printer_commands(printer_id,job_id,user_id,action,state,created_at) VALUES(?,?,?,'set:bed',?,?)", p.ID, id, u.ID, name, time.Now().UnixMilli())
			case "finished":
				s.DB.Exec("INSERT INTO job_completion(job_id,finished_at) VALUES(?,1)", id)
			case "unstarted":
				s.DB.Exec("UPDATE jobs SET state='assigned' WHERE id=?", id)
			case "normal finish":
				v.RawState, v.State, v.Percent = "FINISH", "idle", 100
			}
			d.statuses[p.ID] = v
			if err := s.collectMail(time.Now()); err != nil {
				t.Fatal(err)
			}
			jobs, _ := readJobs(s.DB)
			if jobs[0].State == "stopped" {
				t.Fatal("unproven physical stop inferred", name)
			}
		})
	}
}

func TestClearedExternalStopCanBeExplicitlyReviewedWithoutSendingAnotherStop(t *testing.T) {
	s, u, d, p, _, id := interruptedJob(t)
	s.Device = &controlFake{fakeDevice: d, controlError: true}
	v := d.statuses[p.ID]
	v.File, v.GcodeFile = "", "Metadata/plate_1.gcode"
	d.statuses[p.ID] = v
	in := ControlInput{PrinterID: p.ID, JobID: id, Action: "resolve"}
	if s.ControlPrinter(User{ID: 999}, in) == nil {
		t.Fatal("foreign user reviewed job")
	}
	if err := s.ControlPrinter(u, in); err != nil {
		t.Fatal(err)
	}
	jobs, _ := readJobs(s.DB)
	if jobs[0].State != "stopped" || jobs[0].FinishedAt != 0 {
		t.Fatal(jobs)
	}
	var physical, reviewed int
	s.DB.QueryRow("SELECT COUNT(*) FROM printer_commands WHERE action!='resolve'").Scan(&physical)
	s.DB.QueryRow("SELECT COUNT(*) FROM printer_commands WHERE action='resolve' AND state='succeeded'").Scan(&reviewed)
	if physical != 0 || reviewed != 1 {
		t.Fatal("review sent or repeated a physical command", physical, reviewed)
	}
}

func TestLegacyCompletionAlsoArchivesPhysicalStopAsCancelled(t *testing.T) {
	s, u, _, _, _, id := interruptedJob(t)
	if err := s.CompleteJob(u, id, 3); err != nil {
		t.Fatal(err)
	}
	jobs, _ := readJobs(s.DB)
	if jobs[0].State != "cancelled" || jobs[0].FinishedAt != 0 {
		t.Fatal(jobs)
	}
}
