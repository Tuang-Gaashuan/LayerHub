package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestInspectionImportCopiesFileAndReplaysWithoutPrinting(t *testing.T) {
	s, u, d, _, _, batch, path, digest := prepareBatch(t)
	info, _ := os.Stat(path)
	id, e := s.queueInspection(u, "plates.3mf", path, digest, info.Size())
	if e != nil {
		t.Fatal(e)
	}
	s.runInspection()
	body, _ := json.Marshal(map[string]any{"inspectionId": id, "batch": batch, "note": ""})
	call := func(actor User) (map[string]any, error) {
		return s.importInspection(actor, httptest.NewRequest("POST", "/api/files/inspections/import", bytes.NewReader(body)))
	}
	if _, e = call(User{ID: u.ID + 100}); e == nil {
		t.Fatal("foreign draft imported")
	}
	first, e := call(u)
	if e != nil {
		t.Fatal(e)
	}
	again, e := call(u)
	if e != nil {
		t.Fatal(e)
	}
	a, b := first["receipt"].(ImportReceipt), again["receipt"].(ImportReceipt)
	if len(a.JobIDs) != 2 || !b.Replayed || !reflect.DeepEqual(a.JobIDs, b.JobIDs) {
		t.Fatal(first, again)
	}
	if e = s.deleteInspection(u, id); e != nil {
		t.Fatal(e)
	}
	for _, jid := range a.JobIDs {
		var retained, state string
		s.DB.QueryRow("SELECT path,state FROM jobs WHERE id=?", jid).Scan(&retained, &state)
		if retained == path || state != "queued" {
			t.Fatal("draft reused as job storage", state)
		}
		if _, e = os.Stat(retained); e != nil {
			t.Fatal(e)
		}
	}
	if d.uploads != 0 {
		t.Fatal("draft import contacted printer")
	}
}

func TestBackgroundInspectionRetainsPrivateResultWithoutJobs(t *testing.T) {
	s, u, _ := setup(t)
	before := betaCounts(t, s)
	raw := sliceFixture(t, intakeFixture())
	path := filepath.Join(s.Config.DataDir, "uploads", "draft")
	os.WriteFile(path, raw, 0600)
	hash := sha256.Sum256(raw)
	id, e := s.queueInspection(u, "test.3mf", path, hex.EncodeToString(hash[:]), int64(len(raw)))
	if e != nil {
		t.Fatal(e)
	}
	s.runInspection()
	result, e := s.inspections(u, id)
	if e != nil {
		t.Fatal(e)
	}
	rows := result["inspections"].([]map[string]any)
	if len(rows) != 1 || rows[0]["state"] != "ready" || rows[0]["review"] == nil {
		t.Fatal(result)
	}
	other, e := s.inspections(User{ID: u.ID + 100}, id)
	if e != nil || len(other["inspections"].([]map[string]any)) != 0 {
		t.Fatal("cross-account inspection", other, e)
	}
	if e = s.deleteInspection(User{ID: u.ID + 100}, id); e == nil {
		t.Fatal("cross-account deletion")
	}
	if !reflect.DeepEqual(before, betaCounts(t, s)) {
		t.Fatal("inspection modified jobs/inventory/commands")
	}
	if e = s.deleteInspection(u, id); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(path); !os.IsNotExist(e) {
		t.Fatal("draft was not removed")
	}
}
func TestBackgroundInspectionSurvivesServiceReopen(t *testing.T) {
	s, u, _ := setup(t)
	raw := sliceFixture(t, intakeFixture())
	p := filepath.Join(s.Config.DataDir, "uploads", "restart-draft")
	os.WriteFile(p, raw, 0600)
	h := sha256.Sum256(raw)
	id, e := s.queueInspection(u, "restart.3mf", p, hex.EncodeToString(h[:]), int64(len(raw)))
	if e != nil {
		t.Fatal(e)
	}
	s.DB.Exec("UPDATE file_inspections SET state='parsing' WHERE id=?", id)
	cfg := s.Config
	s.Close()
	next, e := Open(cfg, &fakeDevice{})
	if e != nil {
		t.Fatal(e)
	}
	defer next.Close()
	next.runInspection()
	r, e := next.inspections(u, id)
	if e != nil || r["inspections"].([]map[string]any)[0]["state"] != "ready" {
		t.Fatal(r, e)
	}
}

// A cancelled service must leave the file eligible for the next startup.
func TestInspectionCancelledServiceDoesNotDiscardDraft(t *testing.T) {
	s, u, _ := setup(t)
	p := filepath.Join(s.Config.DataDir, "uploads", "cancelled")
	os.WriteFile(p, []byte("invalid"), 0600)
	id, e := s.queueInspection(u, "bad.3mf", p, "", 7)
	if e != nil {
		t.Fatal(e)
	}
	old := s.ctx
	ctx, cancel := context.WithCancel(old)
	cancel()
	s.ctx = ctx
	s.runInspection()
	s.ctx = old
	var state string
	s.DB.QueryRow("SELECT state FROM file_inspections WHERE id=?", id).Scan(&state)
	if state != "queued" {
		t.Fatal(state)
	}
}
