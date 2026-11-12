package app

import (
	"testing"
	"time"
)

func TestFinishedOfflineJobAllowsIPRecoveryAndManualSettlement(t *testing.T) {
	s, u, d, p, sp, id := finishedJob(t)
	s.Config.DeviceMode = "live"
	d.mu.Lock()
	d.statuses[p.ID] = Telemetry{}
	d.mu.Unlock()
	if e := s.SavePrinter(u, PrinterInput{ID: p.ID, IP: "192.0.2.9"}); e != nil {
		t.Fatal(e)
	}
	jobs, _ := readJobs(s.DB)
	stock, _ := readSpools(s.DB)
	if jobs[0].State != "printing" || jobs[0].FinishedAt == 0 || jobs[0].SettledAt != 0 || stock[0].Remaining != sp.Remaining {
		t.Fatal("IP recovery altered job/accounting", jobs)
	}
	// Still offline after the reconnect attempt: the durable finish belongs to
	// this job, while collection and consumption remain explicit user input.
	d.mu.Lock()
	d.statuses[p.ID] = Telemetry{}
	d.mu.Unlock()
	input := SettlementWrite{JobID: id, Uses: map[string]SettlementUse{sp.ID: {Source: "measured", Grams: 20.25}}}
	if s.SettleJob(u, input) == nil {
		t.Fatal("collection confirmation bypassed")
	}
	input.Collected = true
	if e := s.SettleJob(u, input); e != nil {
		t.Fatal(e)
	}
	if s.SettleJob(u, input) == nil {
		t.Fatal("double settlement")
	}
	stock, _ = readSpools(s.DB)
	if stock[0].Remaining != sp.Remaining-21 {
		t.Fatal("incorrect stock deduction")
	}
	var commands int
	s.DB.QueryRow("SELECT COUNT(*) FROM printer_commands").Scan(&commands)
	if commands != 0 || d.uploads != 0 {
		t.Fatal("recovery controlled printer")
	}
}

func TestOfflineUnfinishedRecoveryDoesNotInventCompletion(t *testing.T) {
	s, u, d := setup(t)
	p, sp := fixture(t, s, u)
	id := newJob(t, s, u, p, sp, 25)
	s.DB.Exec("UPDATE jobs SET state='printing' WHERE id=?", id)
	s.Config.DeviceMode = "live"
	d.mu.Lock()
	d.statuses[p.ID] = Telemetry{}
	d.mu.Unlock()
	if e := s.SavePrinter(u, PrinterInput{ID: p.ID, IP: "192.0.2.9"}); e != nil {
		t.Fatal(e)
	}
	if s.SettleJob(u, SettlementWrite{JobID: id, Collected: true, Uses: map[string]SettlementUse{sp.ID: {Source: "measured", Grams: 20}}}) == nil {
		t.Fatal("offline recovery invented finish evidence")
	}
	jobs, _ := readJobs(s.DB)
	if jobs[0].FinishedAt != 0 || jobs[0].SettledAt != 0 {
		t.Fatal("completion synthesized")
	}
}

func TestIPRecoveryDoesNotAllowIdentityChangesOrExecutionRaces(t *testing.T) {
	for _, state := range []string{"uploading", "starting", "assigned", "checking", "printing", "paused"} {
		t.Run(state, func(t *testing.T) {
			s, u, d := setup(t)
			p, sp := fixture(t, s, u)
			id := newJob(t, s, u, p, sp, 25)
			s.DB.Exec("UPDATE jobs SET state=? WHERE id=?", state, id)
			d.mu.Lock()
			d.statuses[p.ID] = Telemetry{Connected: true, Updated: time.Now(), State: "printing", RawState: "RUNNING"}
			d.mu.Unlock()
			if s.SavePrinter(u, PrinterInput{ID: p.ID, IP: "192.0.2.9"}) == nil {
				t.Fatal("changed IP during execution", state)
			}
		})
	}
	for _, change := range []string{"identity", "credentials", "configuration", "disabled", "pending", "uncertain", "member"} {
		t.Run(change, func(t *testing.T) {
			s, u, d, p, _, _ := finishedJob(t)
			d.mu.Lock()
			d.statuses[p.ID] = Telemetry{}
			d.mu.Unlock()
			in := PrinterInput{ID: p.ID, IP: "192.0.2.9"}
			actor := u
			switch change {
			case "identity":
				in.Serial = "OTHER-DEVICE"
			case "credentials":
				in.Code = "replacement"
			case "configuration":
				in.Configuration = "bare"
			case "disabled":
				in.Enabled = ptr(false)
			case "member":
				actor.Admin = false
			case "pending", "uncertain":
				s.DB.Exec("INSERT INTO printer_commands(printer_id,job_id,user_id,action,state,created_at) VALUES(?,0,?,'set:light',?,?)", p.ID, u.ID, change, time.Now().UnixMilli())
			}
			if s.SavePrinter(actor, in) == nil {
				t.Fatal("unsafe recovery accepted", change)
			}
		})
	}
}
