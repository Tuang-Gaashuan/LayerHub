package app

import (
	"database/sql"
	"testing"
	"time"
)

func TestSettledStopReadinessRequiresSpecificClearedJobAndNoAlarm(t *testing.T) {
	now := time.Now()
	base := Telemetry{Connected: true, Updated: now, StateUpdated: now, TaskUpdated: now, State: "failed", RawState: "FAILED", File: "job-9"}
	prior := Job{ID: 9, Printer: 1, State: "cancelled", Remote: "job-9-unique.3mf", CollectedAt: 1, SettledAt: 1}
	for _, name := range []string{"clear", "queued", "assigned", "not collected", "not settled", "stopped", "done", "foreign task", "stale state", "stale task", "offline", "print error", "HMS", "switching", "authorization", "connection error", "another execution"} {
		t.Run(name, func(t *testing.T) {
			v, j := base, prior
			var extra []Job
			switch name {
			case "queued", "assigned":
				extra = []Job{{ID: 10, Printer: 1, State: name}}
			case "not collected":
				j.CollectedAt = 0
			case "not settled":
				j.SettledAt = 0
			case "stopped", "done":
				j.State = name
			case "foreign task":
				v.File = "job-8"
			case "stale state":
				v.StateUpdated = now.Add(-2 * time.Minute)
			case "stale task":
				v.TaskUpdated = now.Add(-2 * time.Minute)
			case "offline":
				v.Connected = false
			case "print error":
				v.PrintError = 42
			case "HMS":
				v.Equipment.HMS = []string{"fault"}
			case "switching":
				v.Equipment.Switching = true
			case "authorization":
				v.Equipment.ControlIssue = "authorization_required"
			case "connection error":
				v.Error = "connection uncertain"
			case "another execution":
				extra = []Job{{ID: 10, Printer: 1, State: "printing"}}
			}
			want := name == "clear" || name == "queued" || name == "assigned"
			if got := settledStopReady(append([]Job{j}, extra...), 1, v); got != want {
				t.Fatal(name, got, want)
			}
		})
	}
}

func TestNextPreflightAndSnapshotAgreeAfterCancelledJobSettlement(t *testing.T) {
	s, u, d, p, sp, id := interruptedJob(t)
	if err := s.SettleJob(u, SettlementWrite{JobID: id, Collected: true, Uses: map[string]SettlementUse{sp.ID: {Source: "measured", Grams: 2}}}); err != nil {
		t.Fatal(err)
	}
	next := newJob(t, s, u, p, sp, 20)
	snapshot, err := s.Snapshot(u)
	if err != nil {
		t.Fatal(err)
	}
	printers := snapshot["printers"].([]Printer)
	if printers[0].State != "idle" || printers[0].RawState != "FAILED" || d.Status(p.ID).State != "failed" {
		t.Fatal("ready display must preserve the raw device state", printers)
	}
	preflight := func(expected string) error {
		return s.transaction(func(tx *sql.Tx) error { _, err := s.executable(tx, u, next, expected); return err })
	}
	if err = preflight("queued"); err != nil {
		t.Fatal("next queued task was blocked by the old cancellation", err)
	}
	s.DB.Exec("UPDATE jobs SET state='assigned' WHERE id=?", next)
	if err = preflight("assigned"); err != nil {
		t.Fatal("post-transfer/start preflight disagreed", err)
	}
	s.DB.Exec("INSERT INTO printer_commands(printer_id,job_id,user_id,action,state,created_at) VALUES(?,?,?,'start','uncertain',0)", p.ID, next, u.ID)
	if preflight("assigned") == nil {
		t.Fatal("unconfirmed command guard was bypassed")
	}
	if d.uploads != 0 {
		t.Fatal("readiness checking uploaded or started a file")
	}
}

func TestAdministratorSettingUsesSameAcknowledgedTerminalReadiness(t *testing.T) {
	s, u, d, p, sp, id := interruptedJob(t)
	if err := s.SettleJob(u, SettlementWrite{JobID: id, Collected: true, Uses: map[string]SettlementUse{sp.ID: {Source: "measured", Grams: 0}}}); err != nil {
		t.Fatal(err)
	}
	v := d.Status(p.ID)
	v.Equipment.Settings = []SettingReading{{Key: "nozzle", Value: 0, Min: 0, Max: 300, Updated: time.Now()}}
	d.statuses[p.ID] = v
	s.Device = &equipmentFake{fakeDevice: d}
	if err := s.SetEquipment(u, SettingInput{PrinterID: p.ID, Key: "nozzle", Value: 0}); err != nil {
		t.Fatal(err)
	}
	waitCommand(t, s, "succeeded")
}
