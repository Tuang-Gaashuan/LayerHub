package app

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCancelEndpointRequiresSessionAndCSRFAndPreservesBatchReceipt(t *testing.T) {
	s, u, d, _, sp, in, file, digest := prepareBatch(t)
	receipt, e := s.CreateImportBatch(u, in, "plates.3mf", file, digest, "", 100)
	if e != nil {
		t.Fatal(e)
	}
	token := securityLogin(t, s, "admin", "TestPassword123", "Cancel test")
	body := []byte(fmt.Sprintf(`{"jobId":%d}`, receipt.JobIDs[0]))
	call := func(auth, csrf bool) int {
		r := httptest.NewRequest(http.MethodPost, "/api/jobs/cancel", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if auth {
			r.AddCookie(&http.Cookie{Name: "layerhub_sid", Value: token})
		}
		if csrf {
			r.Header.Set("X-CSRF-Token", s.csrf(token))
		}
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		return w.Code
	}
	if call(false, false) != 401 || call(true, false) != 403 || call(true, true) != 200 {
		t.Fatal("authentication or cancellation failed")
	}
	replay, e := s.CreateImportBatch(u, in, "plates.3mf", file, digest, "", 100)
	if e != nil || !replay.Replayed || len(replay.JobIDs) != 2 || replay.JobIDs[0] != receipt.JobIDs[0] {
		t.Fatal("cancel invalidated recovery receipt", replay, e)
	}
	jobs, e := readJobs(s.DB)
	if e != nil || reserve(jobs, sp.ID, 0) != 12 || d.uploads != 0 {
		t.Fatal("wrong reserve or execution side effect", e)
	}
}

func TestCancelWaitingTaskPreservesOtherQueueStockFilesAndCommands(t *testing.T) {
	s, u, d := setup(t)
	p, sp := fixture(t, s, u)
	a := newJob(t, s, u, p, sp, 10)
	b := newJob(t, s, u, p, sp, 20)
	if s.CancelJob(User{ID: 999}, a) == nil {
		t.Fatal("ownership bypass")
	}
	if e := s.CancelJob(u, a); e != nil {
		t.Fatal(e)
	}
	if e := s.CancelJob(u, a); e != nil {
		t.Fatal("cancel is not idempotent", e)
	}
	jobs, e := readJobs(s.DB)
	if e != nil {
		t.Fatal(e)
	}
	if len(jobs) != 2 || reserve(jobs, sp.ID, 0) != 20 {
		t.Fatal("wrong reservation or deleted record", jobs)
	}
	for _, j := range jobs {
		if j.ID == a && j.State != "cancelled" || j.ID == b && j.State != "queued" {
			t.Fatal(j)
		}
	}
	var n, stock int
	s.DB.QueryRow("SELECT count(*) FROM printer_commands").Scan(&n)
	s.DB.QueryRow("SELECT remaining FROM spools WHERE id=?", sp.ID).Scan(&stock)
	if n != 0 || stock != sp.Remaining || d.uploads != 0 {
		t.Fatal("cancel caused side effects", n, stock, d.uploads)
	}
}

func TestCancelRejectsStartedUncertainAndTransferStates(t *testing.T) {
	for _, state := range []string{"uploading", "starting", "printing", "paused", "stopped", "done", "queued-error", "queued-pending", "assigned-start"} {
		t.Run(state, func(t *testing.T) {
			s, u, _ := setup(t)
			p, sp := fixture(t, s, u)
			id := newJob(t, s, u, p, sp, 10)
			raw := state
			if state == "queued-error" || state == "queued-pending" {
				raw = "queued"
			}
			if state == "assigned-start" {
				raw = "assigned"
			}
			if _, e := s.DB.Exec("UPDATE jobs SET state=? WHERE id=?", raw, id); e != nil {
				t.Fatal(e)
			}
			if state == "queued-error" {
				s.DB.Exec("UPDATE jobs SET error='unconfirmed' WHERE id=?", id)
			}
			if state == "queued-pending" || state == "assigned-start" {
				cmdState := "pending"
				if state == "assigned-start" {
					cmdState = "reviewed"
				}
				if _, e := s.DB.Exec("INSERT INTO printer_commands(printer_id,job_id,user_id,action,state,created_at) VALUES(?,?,?,'start',?,1)", p.ID, id, u.ID, cmdState); e != nil {
					t.Fatal(e)
				}
			}
			if s.CancelJob(u, id) == nil {
				t.Fatal(fmt.Sprintf("cancel accepted %s", state))
			}
		})
	}
}
