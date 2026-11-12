package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSpoolActivityAndReceiptCommitAtomically(t *testing.T) {
	s, u, _ := setup(t)
	_, sp := fixture(t, s, u)
	history, e := s.SpoolHistory(u, sp.ID, 0)
	if e != nil {
		t.Fatal(e)
	}
	initial := len(history["events"].([]SpoolEvent))
	sp.Remaining = 712
	key := OperationKey{strings.Repeat("a", 32), "digest", "/api/spools"}
	if e = s.SaveSpool(u, sp, key); e != nil {
		t.Fatal(e)
	}
	history, e = s.SpoolHistory(u, sp.ID, 0)
	if e != nil {
		t.Fatal(e)
	}
	events := history["events"].([]SpoolEvent)
	if len(events) != initial+1 || events[0].Before.Remaining != 1000 || events[0].After.Remaining != 712 || events[0].Action != "adjust" || events[0].ActorID != u.ID {
		t.Fatalf("wrong activity: %+v", events)
	}
	result, e := s.OperationResult(u, key.ID)
	if e != nil || result["state"] != "committed" {
		t.Fatal(result, e)
	}
	if e = s.SaveSpool(u, sp, key); e == nil {
		t.Fatal("duplicate operation accepted")
	}
	after, _ := s.SpoolHistory(u, sp.ID, 0)
	if len(after["events"].([]SpoolEvent)) != len(events) {
		t.Fatal("duplicate history")
	}
	var total int
	s.DB.QueryRow("SELECT COUNT(*) FROM operation_receipts").Scan(&total)
	if total != 1 {
		t.Fatal(total)
	}
}

func TestFailedAuditRollsBackStockAndReceipt(t *testing.T) {
	s, u, _ := setup(t)
	_, sp := fixture(t, s, u)
	if _, e := s.DB.Exec(`CREATE TRIGGER fail_activity BEFORE INSERT ON spool_events BEGIN SELECT RAISE(ABORT,'injected activity failure'); END`); e != nil {
		t.Fatal(e)
	}
	key := OperationKey{strings.Repeat("b", 32), "digest", "/api/spools"}
	sp.Remaining = 123
	if e := s.SaveSpool(u, sp, key); e == nil {
		t.Fatal("failed log accepted")
	}
	spools, _ := readSpools(s.DB)
	if spools[0].Remaining != 1000 {
		t.Fatal("stock committed without log")
	}
	r, e := s.OperationResult(u, key.ID)
	if e != nil || r["state"] != "unknown" {
		t.Fatal(r, e)
	}
}

func TestSpoolHistoryPrivacyAndPagination(t *testing.T) {
	s, u, _ := setup(t)
	_, sp := fixture(t, s, u)
	sp.Owner = u.Username
	sp.Shared = false
	if e := s.SaveSpool(u, sp); e != nil {
		t.Fatal(e)
	}
	if e := s.SaveUser(u, "other_history", "Other", "test-password",false); e != nil {
		t.Fatal(e)
	}
	var other User
	s.DB.QueryRow("SELECT id,username FROM users WHERE username='other_history'").Scan(&other.ID, &other.Username)
	if _, e := s.SpoolHistory(other, sp.ID, 0); e == nil {
		t.Fatal("private history leaked")
	}
	for i := 0; i < 55; i++ {
		spools, _ := readSpools(s.DB)
		next := spools[0]
		next.Label = "Edited " + strings.Repeat("x", i%20)
		if e := s.SaveSpool(u, next); e != nil {
			t.Fatal(e)
		}
	}
	r, e := s.SpoolHistory(u, sp.ID, 0)
	if e != nil {
		t.Fatal(e)
	}
	events := r["events"].([]SpoolEvent)
	if len(events) != 50 || r["next"].(int64) == 0 {
		t.Fatal("pagination missing")
	}
	next, e := s.SpoolHistory(u, sp.ID, r["next"].(int64))
	if e != nil || len(next["events"].([]SpoolEvent)) == 0 {
		t.Fatal(next, e)
	}
	if next["events"].([]SpoolEvent)[0].ID >= events[len(events)-1].ID {
		t.Fatal("page overlap")
	}
}

func TestConsumptionHistoryAndNoDuplicateSettlement(t *testing.T) {
	s, u, _, _, sp, id := finishedJob(t)
	in := SettlementWrite{JobID: id, Collected: true, Uses: map[string]SettlementUse{sp.ID: {Source: "measured", Grams: 20.25}}}
	key := OperationKey{strings.Repeat("c", 32), "settle", "/api/jobs/settle"}
	if e := s.SettleJob(u, in, key); e != nil {
		t.Fatal(e)
	}
	if e := s.SettleJob(u, in, key); e == nil {
		t.Fatal("settled twice")
	}
	r, e := s.SpoolHistory(u, sp.ID, 0)
	if e != nil {
		t.Fatal(e)
	}
	count := 0
	for _, v := range r["events"].([]SpoolEvent) {
		if v.Action == "consume" {
			count++
			if v.JobID != id || v.Before.Remaining-v.After.Remaining != 21 {
				t.Fatal(v)
			}
		}
	}
	if count != 1 {
		t.Fatal(count)
	}
}

func TestHTTPReceiptWorksAfterLosingMutationResponse(t *testing.T) {
	s, u, _ := setup(t)
	_, sp := fixture(t, s, u)
	sp.Remaining = 808
	token := randomID(32)
	if _, e := s.DB.Exec("INSERT INTO sessions(hash,user_id,expires) VALUES(?,?,?)", tokenHash(token), u.ID, time.Now().Add(time.Hour).Unix()); e != nil {
		t.Fatal(e)
	}
	id := strings.Repeat("d", 32)
	raw, _ := json.Marshal(sp)
	req := httptest.NewRequest("POST", "/api/spools", bytes.NewReader(raw))
	req.AddCookie(&http.Cookie{Name: "layerhub_sid", Value: token})
	req.Header.Set("X-CSRF-Token", s.csrf(token))
	req.Header.Set("X-LayerHub-Operation", id)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	read := httptest.NewRequest("GET", "/api/operations/result?id="+id, nil)
	read.AddCookie(&http.Cookie{Name: "layerhub_sid", Value: token})
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, read)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"state":"committed"`) {
		t.Fatal(w.Code, w.Body.String())
	}
	other := u
	other.ID++
	result, e := s.OperationResult(other, id)
	if e != nil || result["state"] != "unknown" {
		t.Fatal("receipt leaked", result, e)
	}
}
