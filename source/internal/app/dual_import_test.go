package app

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDualBackgroundImportAtomicReservationsReplayAndRollback(t *testing.T) {
	s, u, d := setup(t)
	p, pla := fixture(t, s, u)
	if e := s.SavePrinter(u, PrinterInput{ID: p.ID, ModelID: "x2d"}); e != nil {
		t.Fatal(e)
	}
	pla.Mount = &Mount{p.ID, "external"}
	if e := s.SaveSpool(u, pla); e != nil {
		t.Fatal(e)
	}
	petg := pla
	petg.ID = ""
	petg.Revision = 0
	petg.Material = "PETG"
	petg.Label = "white PETG"
	petg.Mount = &Mount{p.ID, "4"}
	if e := s.SaveSpool(u, petg); e != nil {
		t.Fatal(e)
	}
	spools, _ := readSpools(s.DB)
	for _, sp := range spools {
		if sp.Material == "PETG" {
			petg = sp
		}
	}
	raw := sliceFixture(t, dualSliceEntries())
	file := filepath.Join(s.Config.DataDir, "uploads", "dual.3mf")
	os.WriteFile(file, raw, 0600)
	h := sha256.Sum256(raw)
	digest := hex.EncodeToString(h[:])
	inspection, e := s.queueInspection(u, "dual.3mf", file, digest, int64(len(raw)))
	if e != nil {
		t.Fatal(e)
	}
	s.runInspection()
	options := dualOptions()
	options.Mapping[0].SpoolID = pla.ID
	options.Mapping[1].SpoolID = petg.ID
	batch := ImportBatch{RequestID: "dual-background-import-0001", Items: []Schedule{{Printer: p.ID, Plate: 1, Estimate: 83, Grams: 5, SpoolID: pla.ID}}, Options: []PrintOptions{options}}
	call := func(in ImportBatch) (map[string]any, error) {
		body, _ := json.Marshal(map[string]any{"inspectionId": inspection, "batch": in})
		return s.importInspection(u, httptest.NewRequest("POST", "/api/files/inspections/import", bytes.NewReader(body)))
	}
	bad := batch
	bad.Options = append([]PrintOptions{}, batch.Options...)
	bad.Options[0].Mapping = append([]AMSTarget{}, options.Mapping...)
	bad.Options[0].Mapping[1].SpoolID = pla.ID
	if _, e = call(bad); e == nil {
		t.Fatal("invalid second stock accepted")
	}
	jobs, _ := readJobs(s.DB)
	if len(jobs) != 0 {
		t.Fatal("failed import left a partially reserved job")
	}
	first, e := call(batch)
	if e != nil {
		t.Fatal(e)
	}
	again, e := call(batch)
	if e != nil {
		t.Fatal(e)
	}
	if !again["receipt"].(ImportReceipt).Replayed || first["receipt"].(ImportReceipt).JobIDs[0] != again["receipt"].(ImportReceipt).JobIDs[0] {
		t.Fatal("duplicate import")
	}
	jobs, _ = readJobs(s.DB)
	if len(jobs) != 1 || jobs[0].PrintOptions == nil || jobUses(jobs[0])[pla.ID] != 5 || jobUses(jobs[0])[petg.ID] != 34 || jobs[0].Grams != 39 {
		t.Fatal("independent reservations not saved", jobs)
	}
	changed := batch
	changed.Options = append([]PrintOptions{}, batch.Options...)
	changed.Options[0].BedLeveling = false
	if _, e = call(changed); e == nil {
		t.Fatal("replay with changed options accepted")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.uploads != 0 {
		t.Fatal("import transferred printer file")
	}
	var count int
	s.DB.QueryRow("SELECT COUNT(*) FROM printer_commands").Scan(&count)
	if count != 0 {
		t.Fatal("import created control commands")
	}
}
