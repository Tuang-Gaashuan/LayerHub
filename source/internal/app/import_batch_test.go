package app

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func multiPlateFixture(t *testing.T) []byte {
	return sliceFixture(t, map[string]string{
		"Metadata/project_settings.config": `{"printer_model":"Bambu Lab P1S"}`,
		"Metadata/plate_1.gcode":           ";fixture one", "Metadata/plate_7.gcode": ";fixture seven",
		"Metadata/slice_info.config": `<config><plate><metadata key="index" value="1"/><metadata key="prediction" value="61"/><filament id="1" type="PLA" color="#FFFFFF" used_g="9.2"/></plate><plate><metadata key="index" value="7"/><metadata key="prediction" value="181"/><filament id="1" type="PLA" color="#000000" used_g="11.3"/></plate></config>`,
	})
}

func prepareBatch(t *testing.T) (*Service, User, *fakeDevice, Printer, Spool, ImportBatch, string, string) {
	t.Helper()
	s, u, d := setup(t)
	p, sp := fixture(t, s, u)
	raw := multiPlateFixture(t)
	path := filepath.Join(s.Config.DataDir, "uploads", "plates.3mf")
	os.WriteFile(path, raw, 0o600)
	h := sha256.Sum256(raw)
	in := ImportBatch{RequestID: "fixture-request-id-0001", Items: []Schedule{{Printer: p.ID, Plate: 1, Estimate: 2, Grams: 10, SpoolID: sp.ID}, {Printer: p.ID, Plate: 7, Estimate: 4, Grams: 12, SpoolID: sp.ID}}}
	return s, u, d, p, sp, in, path, hex.EncodeToString(h[:])
}

func TestMultiPlateBatchAtomicReservationsReplayAndFileLifetime(t *testing.T) {
	s, u, d, _, sp, in, file, digest := prepareBatch(t)
	var wg sync.WaitGroup
	results := make(chan ImportReceipt, 3)
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			receipt, err := s.CreateImportBatch(u, in, "plates.3mf", file, digest, "", 100)
			if err != nil {
				t.Error(err)
			}
			results <- receipt
		}()
	}
	wg.Wait()
	close(results)
	var first ImportReceipt
	for result := range results {
		if len(result.JobIDs) != 2 {
			t.Fatal(result)
		}
		if first.JobIDs == nil {
			first = result
		} else if fmt.Sprint(first.JobIDs) != fmt.Sprint(result.JobIDs) {
			t.Fatal("replay created new jobs")
		}
	}
	jobs, err := readJobs(s.DB)
	if err != nil || len(jobs) != 2 || jobs[0].SelectedPlate != 1 || jobs[1].SelectedPlate != 7 || jobs[0].Path != jobs[1].Path || reserve(jobs, sp.ID, 0) != 22 || d.uploads != 0 {
		t.Fatal(jobs, err)
	}
	changed := in
	changed.Items = append([]Schedule(nil), in.Items...)
	changed.Items[0].Grams++
	if _, err := s.CreateImportBatch(u, changed, "plates.3mf", file, digest, "", 100); err == nil {
		t.Fatal("reused key accepted different content")
	}
	if err := s.DeleteJob(u, jobs[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatal("deleting one plate deleted the shared file", err)
	}
	if _, err := s.JobSlice(u, jobs[1].ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteJob(u, jobs[1].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("last reference did not release source file")
	}
	receipt, err := s.ImportReceipt(u, in.RequestID)
	if err != nil || receipt["found"] != true {
		t.Fatal(receipt, err)
	}
	foreign, err := s.ImportReceipt(User{ID: u.ID + 100}, in.RequestID)
	if err != nil || foreign["found"] != false {
		t.Fatal("receipt leaked across users", foreign, err)
	}
}

func TestBatchFailureRollsBackAllJobsAndReservations(t *testing.T) {
	for _, failure := range []string{"stock", "duplicate", "missing plate", "material", "wrong printer", "too many", "under reserve"} {
		t.Run(failure, func(t *testing.T) {
			s, u, _, _, sp, in, file, digest := prepareBatch(t)
			switch failure {
			case "stock":
				s.DB.Exec("UPDATE spools SET remaining=21 WHERE id=?", sp.ID)
			case "duplicate":
				in.Items[1].Plate = 1
			case "missing plate":
				in.Items[1].Plate = 2
			case "material":
				s.DB.Exec("UPDATE spools SET material='PETG' WHERE id=?", sp.ID)
			case "wrong printer":
				s.DB.Exec("UPDATE printers SET model_id='a1'")
			case "too many":
				for len(in.Items) < 17 {
					in.Items = append(in.Items, in.Items[0])
				}
			case "under reserve":
				in.Items[1].Grams = 1
			}
			if _, err := s.CreateImportBatch(u, in, "plates.3mf", file, digest, "", 100); err == nil {
				t.Fatal("invalid batch accepted")
			}
			for _, table := range []string{"jobs", "job_plate_selection", "job_import_batches", "printer_commands", "job_consumption"} {
				var n int
				s.DB.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n)
				if n != 0 {
					t.Fatal("partial batch persisted", table, n)
				}
			}
		})
	}
}

func TestExplicitImportPlateAndPreviewsAreBounded(t *testing.T) {
	if _, err := selectedImportPlate(SliceInfo{Plates: []SlicePlate{{ID: 1}, {ID: 7}}}, 0); err == nil {
		t.Fatal("multi-plate file silently chose first plate")
	}
	var thumb bytes.Buffer
	png.Encode(&thumb, image.NewNRGBA(image.Rect(0, 0, 512, 256)))
	files := intakeFixture()
	files["Metadata/plate_1.png"] = thumb.String()
	s, _, _ := setup(t)
	path := filepath.Join(s.Config.DataDir, "uploads", "preview.3mf")
	os.WriteFile(path, sliceFixture(t, files), 0o600)
	result, err := inspectPrintFile(t.Context(), path, strings.Repeat("a", 64), 100)
	if err != nil || !strings.HasPrefix(result.Sliced.Previews[1], "data:image/png;base64,") || len(result.Sliced.Previews[1]) > 140000 {
		t.Fatal(result, err)
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(result.Sliced.Previews[1], "data:image/png;base64,"))
	if err != nil {
		t.Fatal(err)
	}
	dimensions, err := png.DecodeConfig(bytes.NewReader(decoded))
	if err != nil || dimensions.Width != 160 || dimensions.Height != 80 {
		t.Fatal(dimensions, err)
	}
	files["Metadata/plate_1.png"] = "<svg onload='PRIVATE_SCRIPT'/>"
	os.WriteFile(path, sliceFixture(t, files), 0o600)
	result, err = inspectPrintFile(t.Context(), path, strings.Repeat("a", 64), 100)
	if err != nil || len(result.Sliced.Previews) != 0 {
		t.Fatal("invalid optional preview was exposed", result, err)
	}
}

func TestImportedPlateRequiresPreparationAndChangingPlateUpdatesEstimate(t *testing.T) {
	s, u, d, p, sp, in, file, digest := prepareBatch(t)
	sp.Mount = &Mount{p.ID, "1"}
	if err := s.SaveSpool(u, sp); err != nil {
		t.Fatal(err)
	}
	r, err := s.CreateImportBatch(u, in, "plates.3mf", file, digest, "", 100)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SendJob(u, r.JobIDs[0]); err == nil || !strings.Contains(err.Error(), "保存供料准备") {
		t.Fatal("unprepared selected plate was transferable", err)
	}
	o := PrintOptions{Plate: 7, UseAMS: true, Mapping: []AMSTarget{{Filament: 0, AMSID: 0, SlotID: 0, SpoolID: sp.ID, Grams: 12}}}
	if err = s.SavePrintOptions(u, r.JobIDs[0], o); err != nil {
		t.Fatal(err)
	}
	jobs, err := readJobs(s.DB)
	if err != nil || jobs[0].SelectedPlate != 7 || jobs[0].PrintOptions.Plate != 7 || jobs[0].Estimate != 4 || reserve(jobs, sp.ID, 0) != 24 {
		t.Fatal(jobs, err)
	}
	s.DB.Exec("UPDATE job_plate_selection SET plate=1 WHERE job_id=?", r.JobIDs[0])
	if err = s.SendJob(u, r.JobIDs[0]); err == nil || !strings.Contains(err.Error(), "打印盘选择已变化") {
		t.Fatal("mismatched selection was transferable", err)
	}
	if d.uploads != 0 {
		t.Fatal("test sent a file to the device")
	}
}

func TestPlateWorkspaceMigrationAndFutureVersionFailClosed(t *testing.T) {
	s, _, d := setup(t)
	cfg := s.Config
	if _, err := s.DB.Exec("DROP TABLE job_plate_selection;DROP TABLE job_import_batches;PRAGMA user_version=4"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	upgraded, err := Open(cfg, d)
	if err != nil {
		t.Fatal(err)
	}
	var version int
	upgraded.DB.QueryRow("PRAGMA user_version").Scan(&version)
	if version != 5 {
		t.Fatal(version)
	}
	if _, err = upgraded.DB.Exec("DROP TABLE job_plate_selection;PRAGMA user_version=6"); err != nil {
		t.Fatal(err)
	}
	upgraded.Close()
	if next, err := Open(cfg, d); err == nil {
		next.Close()
		t.Fatal("future workspace version opened")
	}
}

func TestA1BarePreparationUsesExternalSpoolAndRejectsAMS(t *testing.T) {
	s, u, _ := setup(t)
	p, sp := fixture(t, s, u)
	if _, err := s.DB.Exec("UPDATE printers SET model_id='a1',configuration='bare' WHERE id=?", p.ID); err != nil {
		t.Fatal(err)
	}
	sp.Mount = &Mount{p.ID, "external"}
	if err := s.SaveSpool(u, sp); err != nil {
		t.Fatal(err)
	}
	raw := sliceFixture(t, map[string]string{"Metadata/project_settings.config": `{"printer_model":"Bambu Lab A1"}`, "Metadata/plate_7.gcode": "; fixture only", "Metadata/slice_info.config": `<config><plate><metadata key="index" value="7"/><filament id="1" type="PLA" color="#FFFFFF" used_g="11.3"/></plate></config>`})
	file := filepath.Join(s.Config.DataDir, "uploads", "a1.3mf")
	os.WriteFile(file, raw, 0o600)
	hash := sha256.Sum256(raw)
	id, err := s.CreateJob(u, Schedule{Printer: p.ID, Plate: 7, Estimate: 4, Grams: 12, SpoolID: sp.ID}, "a1.3mf", file, hex.EncodeToString(hash[:]), "", int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	o := PrintOptions{Plate: 7, UseAMS: false, Mapping: []AMSTarget{{Filament: 0, SpoolID: sp.ID, Grams: 12}}}
	if err = s.SavePrintOptions(u, id, o); err != nil {
		t.Fatal(err)
	}
	o.UseAMS = true
	if err = s.SavePrintOptions(u, id, o); err == nil || !strings.Contains(err.Error(), "未配置 AMS") {
		t.Fatal("bare A1 accepted AMS", err)
	}
}

func TestMultiPlateBackupRestoreRetainsSelectionReceiptAndFiles(t *testing.T) {
	s, u, _, _, sp, in, file, digest := prepareBatch(t)
	stat, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.CreateImportBatch(u, in, "plates.3mf", file, digest, "", stat.Size())
	if err != nil {
		t.Fatal(err)
	}
	archive, cleanup, err := s.ExportBackup(u, "TestPassword123")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	f, err := os.Open(archive)
	if err != nil {
		t.Fatal(err)
	}
	_, ticket, err := s.stageRestore(u, f)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	if err = s.confirmRestore(u, ticket, "TestPassword123"); err != nil {
		t.Fatal(err)
	}
	cfg := s.Config
	s.Close()
	restored, err := Open(cfg, &fakeDevice{})
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	jobs, err := readJobs(restored.DB)
	if err != nil || len(jobs) != 2 || jobs[0].SelectedPlate != 1 || jobs[1].SelectedPlate != 7 || reserve(jobs, sp.ID, 0) != 22 {
		t.Fatal(jobs, err)
	}
	for _, j := range jobs {
		if _, err = restored.JobSlice(u, j.ID); err != nil {
			t.Fatal(err)
		}
	}
	receipt, err := restored.ImportReceipt(u, in.RequestID)
	if err != nil || receipt["found"] != true || fmt.Sprint(receipt["receipt"].(ImportReceipt).JobIDs) != fmt.Sprint(r.JobIDs) {
		t.Fatal(receipt, err)
	}
}

func TestBatchHTTPReplayCleansRetryUploadAndRequiresCSRF(t *testing.T) {
	s, u, _, _, _, in, _, _ := prepareBatch(t)
	token := securityLogin(t, s, "admin", "TestPassword123", "batch-test")
	rawFile := multiPlateFixture(t)
	request := func(csrf bool) *httptest.ResponseRecorder {
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		f, _ := mw.CreateFormFile("file", "plates.3mf")
		f.Write(rawFile)
		raw, _ := json.Marshal(in)
		mw.WriteField("batch", string(raw))
		mw.Close()
		r := httptest.NewRequest("POST", "/api/upload/batch", &body)
		r.Header.Set("Content-Type", mw.FormDataContentType())
		r.AddCookie(&http.Cookie{Name: "layerhub_sid", Value: token})
		if csrf {
			r.Header.Set("X-CSRF-Token", s.csrf(token))
		}
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		return w
	}
	if w := request(false); w.Code != 403 {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, want := range []int{201, 200} {
		if w := request(true); w.Code != want {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	jobs, _ := readJobs(s.DB)
	if len(jobs) != 2 || jobs[0].UserID != u.ID {
		t.Fatal(jobs)
	}
	pending, _ := filepath.Glob(filepath.Join(s.Config.DataDir, "uploads", "pending-*"))
	if len(pending) != 1 {
		t.Fatal("retry left an unreferenced upload", pending)
	}
}
