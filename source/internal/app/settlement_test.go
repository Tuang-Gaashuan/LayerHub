package app

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestSettlementEstimateUsesSelectedPlateAndAggregatesBeforeRounding(t *testing.T) {
	s, u, _, _, sp, id := finishedJob(t)
	raw := sliceFixture(t, map[string]string{"Metadata/project_settings.config": `{"printer_model":"Bambu Lab P1S"}`, "Metadata/plate_2.gcode": ";test only\nG28\n", "Metadata/slice_info.config": `<config><plate><metadata key="index" value="2"/><filament id="1" type="PLA" color="#FFFFFF" used_g="12.1"/><filament id="3" type="PLA" color="#FFFFFF" used_g="6.2"/></plate></config>`})
	file := filepath.Join(s.Config.DataDir, "uploads", "estimate.3mf")
	if e := os.WriteFile(file, raw, 0600); e != nil {
		t.Fatal(e)
	}
	opts := PrintOptions{Plate: 2, UseAMS: true, Mapping: []AMSTarget{{Filament: 0, SpoolID: sp.ID, Grams: 13}, {Filament: 2, SpoolID: sp.ID, Grams: 7}}}
	encoded, _ := json.Marshal(opts)
	if _, e := s.DB.Exec("UPDATE jobs SET path=?,print_options=? WHERE id=?", file, string(encoded), id); e != nil {
		t.Fatal(e)
	}
	got, e := s.JobSettlement(u, id)
	if e != nil || got.Rolls[0].Estimate == nil || *got.Rolls[0].Estimate != 18.3 {
		t.Fatal(got, e)
	}
	// Ignore a client's supplied grams in estimate mode; derive it from the file.
	if e = s.SettleJob(u, SettlementWrite{JobID: id, Collected: true, Uses: map[string]SettlementUse{sp.ID: {Source: "estimated", Grams: 999}}}); e != nil {
		t.Fatal(e)
	}
	got, e = s.JobSettlement(u, id)
	if e != nil || *got.Rolls[0].Booked != 19 || *got.Rolls[0].Reported != 18.3 || got.Rolls[0].Source != "estimated" {
		t.Fatal(got, e)
	}
}

func TestSettlementHTTPEnvelopeAndForeignPermission(t *testing.T) {
	s, u, _, _, _, id := finishedJob(t)
	token := randomID(32)
	s.DB.Exec("INSERT INTO sessions(hash,user_id,expires) VALUES(?,?,?)", tokenHash(token), u.ID, time.Now().Add(time.Hour).Unix())
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/jobs/settlement?jobId=%d", id), nil)
	req.AddCookie(&http.Cookie{Name: "layerhub_sid", Value: token})
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	var result struct {
		OK  bool `json:"ok"`
		Job Job  `json:"job"`
	}
	json.Unmarshal(w.Body.Bytes(), &result)
	if w.Code != 200 || !result.OK || result.Job.ID != id || result.Job.DisplayState != "printed" {
		t.Fatal(w.Code, w.Body.String())
	}
	if e := s.SettleJob(User{ID: u.ID + 100}, SettlementWrite{JobID: id, Collected: true}); e == nil {
		t.Fatal("foreign user wrote settlement")
	}
}

func finishedJob(t *testing.T) (*Service, User, *fakeDevice, Printer, Spool, int64) {
	t.Helper()
	s, u, d := setup(t)
	p, sp := fixture(t, s, u)
	id := newJob(t, s, u, p, sp, 25)
	if _, err := s.DB.Exec("UPDATE jobs SET state='printing',remote_path='unique.3mf' WHERE id=?", id); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	d.statuses[p.ID] = Telemetry{Connected: true, Updated: now, StateUpdated: now, TaskUpdated: now, State: "idle", RawState: "FINISH", File: fmt.Sprintf("job-%d", id)}
	if err := s.collectMail(now); err != nil {
		t.Fatal(err)
	}
	return s, u, d, p, sp, id
}

func TestSettlementFinishPersistsAndDoesNotDeduct(t *testing.T) {
	s, u, d, p, sp, id := finishedJob(t)
	d.statuses[p.ID] = Telemetry{}
	if err := s.initMail(); err != nil {
		t.Fatal(err)
	}
	got, err := s.JobSettlement(u, id)
	if err != nil || got.Job.DisplayState != "printed" || got.Job.FinishedAt == 0 || got.Job.State != "printing" {
		t.Fatal(got, err)
	}
	var stock, n int
	s.DB.QueryRow("SELECT remaining FROM spools WHERE id=?", sp.ID).Scan(&stock)
	s.DB.QueryRow("SELECT COUNT(*) FROM job_consumption").Scan(&n)
	if stock != 1000 || n != 0 || got.Rolls[0].Reserved != 25 {
		t.Fatal("finish observation settled inventory")
	}
	if _, err = s.JobSettlement(User{ID: u.ID + 100}, id); err == nil {
		t.Fatal("foreign user read settlement")
	}
}

func TestSettlementMeasuredPrecisionAndConcurrentDuplicate(t *testing.T) {
	s, u, _, _, sp, id := finishedJob(t)
	in := SettlementWrite{JobID: id, Collected: true, Uses: map[string]SettlementUse{sp.ID: {Source: "measured", Grams: 24.16}}}
	var wg sync.WaitGroup
	errors := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errors <- s.SettleJob(u, in) }()
	}
	wg.Wait()
	close(errors)
	passed := 0
	for e := range errors {
		if e == nil {
			passed++
		}
	}
	if passed != 1 {
		t.Fatal("duplicate settlement accepted", passed)
	}
	got, e := s.JobSettlement(u, id)
	if e != nil {
		t.Fatal(e)
	}
	r := got.Rolls[0]
	if got.Job.State != "done" || got.Job.CollectedAt == 0 || got.Job.SettledAt == 0 || r.Source != "measured" || r.Reported == nil || *r.Reported != 24.16 || r.Booked == nil || *r.Booked != 25 {
		t.Fatal(got)
	}
	var stock int
	s.DB.QueryRow("SELECT remaining FROM spools WHERE id=?", sp.ID).Scan(&stock)
	if stock != 975 {
		t.Fatal(stock)
	}
}

func TestSettlementRejectsIncompleteUnsafeAndInvalidRequests(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*Service, *fakeDevice, Printer, *SettlementWrite)
	}{
		{"not collected", func(_ *Service, _ *fakeDevice, _ Printer, in *SettlementWrite) { in.Collected = false }},
		{"foreign user", func(_ *Service, _ *fakeDevice, _ Printer, in *SettlementWrite) { in.JobID = 999 }},
		{"extra spool", func(_ *Service, _ *fakeDevice, _ Printer, in *SettlementWrite) {
			in.Uses["foreign"] = SettlementUse{Source: "measured", Grams: 1}
		}},
		{"negative", func(_ *Service, _ *fakeDevice, _ Printer, in *SettlementWrite) {
			for id := range in.Uses {
				in.Uses[id] = SettlementUse{Source: "measured", Grams: -1}
			}
		}},
		{"too precise", func(_ *Service, _ *fakeDevice, _ Printer, in *SettlementWrite) {
			for id := range in.Uses {
				in.Uses[id] = SettlementUse{Source: "measured", Grams: 1.0001}
			}
		}},
		{"no slice estimate", func(_ *Service, _ *fakeDevice, _ Printer, in *SettlementWrite) {
			for id := range in.Uses {
				in.Uses[id] = SettlementUse{Source: "estimated", Grams: 0}
			}
		}},
		{"too much", func(_ *Service, _ *fakeDevice, _ Printer, in *SettlementWrite) {
			for id := range in.Uses {
				in.Uses[id] = SettlementUse{Source: "measured", Grams: 1001}
			}
		}},
		{"pending command", func(s *Service, _ *fakeDevice, p Printer, in *SettlementWrite) {
			s.DB.Exec("INSERT INTO printer_commands(printer_id,job_id,user_id,action,state,created_at) VALUES(?,?,1,'pause','uncertain',1)", p.ID, in.JobID)
		}},
		{"live printing", func(s *Service, d *fakeDevice, p Printer, _ *SettlementWrite) {
			s.Config.DeviceMode = "live"
			d.statuses[p.ID] = Telemetry{Connected: true, Updated: time.Now(), State: "printing"}
		}},
		{"no finish evidence", func(s *Service, d *fakeDevice, p Printer, in *SettlementWrite) {
			s.DB.Exec("DELETE FROM job_completion WHERE job_id=?", in.JobID)
			d.statuses[p.ID] = Telemetry{}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, u, d, p, sp, id := finishedJob(t)
			in := SettlementWrite{JobID: id, Collected: true, Uses: map[string]SettlementUse{sp.ID: {Source: "measured", Grams: 20}}}
			tc.edit(s, d, p, &in)
			if e := s.SettleJob(u, in); e == nil {
				t.Fatal("accepted invalid request")
			}
			var n, stock int
			s.DB.QueryRow("SELECT COUNT(*) FROM job_consumption").Scan(&n)
			s.DB.QueryRow("SELECT remaining FROM spools WHERE id=?", sp.ID).Scan(&stock)
			if n != 0 || stock != 1000 {
				t.Fatal("failed request modified stock")
			}
		})
	}
}

func TestSettlementBackfillPreservesOriginalTimestamp(t *testing.T) {
	s, _, _, _, _, id := finishedJob(t)
	s.DB.Exec("DELETE FROM job_completion")
	s.DB.Exec("UPDATE mail_observations SET since=12345 WHERE key=?", fmt.Sprintf("job:%d:device_finished", id))
	if err := s.initMail(); err != nil {
		t.Fatal(err)
	}
	var at int64
	s.DB.QueryRow("SELECT finished_at FROM job_completion WHERE job_id=?", id).Scan(&at)
	if at != 12345 {
		t.Fatal(at)
	}
	if err := s.transaction(func(tx *sql.Tx) error { return recordPrintCompletion(tx, Job{ID: id}, 99999) }); err != nil {
		t.Fatal(err)
	}
	s.DB.QueryRow("SELECT finished_at FROM job_completion WHERE job_id=?", id).Scan(&at)
	if at != 12345 {
		t.Fatal("timestamp changed", at)
	}
}
