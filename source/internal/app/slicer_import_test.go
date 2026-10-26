package app

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
	"time"
)

func importCounts(t *testing.T, s *Service) [5]int {
	t.Helper()
	var counts [5]int
	for i, table := range []string{"model_sources", "slicer_projects", "slicer_import_receipts", "slice_tasks", "jobs"} {
		if e := s.DB.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&counts[i]); e != nil {
			t.Fatal(e)
		}
	}
	return counts
}
func importHTTP(t *testing.T, s *Service, token string, raw []byte, fields map[string]string, csrf bool) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	file, e := mw.CreateFormFile("file", "sample.3mf")
	if e != nil {
		t.Fatal(e)
	}
	file.Write(raw)
	for k, v := range fields {
		mw.WriteField(k, v)
	}
	mw.Close()
	r := httptest.NewRequest("POST", "/api/slicing/import-3mf", &body)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	if token != "" {
		r.AddCookie(&http.Cookie{Name: "layerhub_sid", Value: token})
	}
	if csrf {
		r.Header.Set("X-CSRF-Token", s.csrf(token))
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}
func TestImport3MFReviewAuthConsentAndIdempotence(t *testing.T) {
	s, u, _ := setup(t)
	token := randomID(32)
	s.DB.Exec("INSERT INTO sessions(hash,user_id,expires) VALUES(?,?,?)", tokenHash(token), u.ID, time.Now().Add(time.Hour).Unix())
	raw := mfTestZIP(t, mfFixtureFiles())
	before := importCounts(t, s)
	for _, c := range []struct {
		token  string
		csrf   bool
		status int
	}{{"", false, 401}, {token, false, 403}, {token, true, 200}} {
		w := importHTTP(t, s, c.token, raw, map[string]string{"mode": "review"}, c.csrf)
		if w.Code != c.status {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if got := importCounts(t, s); got != before {
		t.Fatal("review wrote business data", got, before)
	}
	p, e := parseModel3MF(context.Background(), raw)
	if e != nil {
		t.Fatal(e)
	}
	fields := map[string]string{"mode": "commit", "sha256": p.Digest, "requestKey": "testing-import-0001"}
	if w := importHTTP(t, s, token, raw, fields, true); w.Code != 400 {
		t.Fatal("consent not required", w.Code)
	}
	fields["acknowledge"] = "geometry-only"
	fields["sha256"] = randomID(32)
	if w := importHTTP(t, s, token, raw, fields, true); w.Code != 409 {
		t.Fatal("different file accepted", w.Code, w.Body.String())
	}
	if got := importCounts(t, s); got != before {
		t.Fatal("rejected commit wrote models", got)
	}
	fields["sha256"] = p.Digest
	var response struct {
		Project  SlicerProject `json:"project"`
		Replayed bool          `json:"replayed"`
	}
	for i := 0; i < 2; i++ {
		w := importHTTP(t, s, token, raw, fields, true)
		if w.Code != 201 {
			t.Fatal(w.Code, w.Body.String())
		}
		if e = json.Unmarshal(w.Body.Bytes(), &response); e != nil {
			t.Fatal(e)
		}
		if response.Replayed != (i == 1) {
			t.Fatal("idempotence receipt missing")
		}
	}
	if got := importCounts(t, s); got[0] != before[0]+2 || got[1] != before[1]+1 || got[2] != before[2]+1 || got[3] != before[3] || got[4] != before[4] {
		t.Fatal(got, before)
	}
	project := response.Project
	if len(project.Plates) != 3 || len(project.Plates[0].Options.Copies) != 2 || project.Plates[2].ModelID != 0 {
		t.Fatal(project)
	}
	for _, b := range project.Plates {
		if b.Options.Layer != .2 || b.Options.Infill != 15 || b.Options.Color != "#FFFFFF" || b.Options.Advanced != nil {
			t.Fatal("untrusted config inherited", b.Options)
		}
	}
	var draft int
	s.DB.QueryRow("SELECT COUNT(*) FROM slicer_drafts").Scan(&draft)
	if draft != 0 {
		t.Fatal("import replaced draft")
	}
	if e = s.deleteSliceModel(u, project.Plates[1].ModelID); e == nil {
		t.Fatal("inactive import source deleted")
	}
	other := u
	other.ID += 999
	if _, e = s.slicerProject(other, project.ID); e == nil {
		t.Fatal("import project leaked")
	}
	if e = s.deleteSlicerProject(u, project.ID, project.Revision); e != nil {
		t.Fatal(e)
	}
	var receipts int
	s.DB.QueryRow("SELECT COUNT(*) FROM slicer_import_receipts").Scan(&receipts)
	if receipts != 0 {
		t.Fatal("import receipt not cleaned with project")
	}
}
func TestImport3MFQuotaAndLateFailureAreAtomic(t *testing.T) {
	s, u, _ := setup(t)
	p, e := parseModel3MF(context.Background(), mfTestZIP(t, mfFixtureFiles()))
	if e != nil {
		t.Fatal(e)
	}
	// A failure after source insertion must roll the entire transaction back.
	bad := p
	bad.Boards = append([]importBoard{}, p.Boards...)
	bad.Boards[0].Placements = append([]ModelPlacement{}, p.Boards[0].Placements...)
	bad.Boards[0].Placements[0].X = 0
	before := importCounts(t, s)
	if _, _, e = s.commitModelImport(u, "broken.3mf", bad, "late-failure-test-01"); e == nil {
		t.Fatal("invalid placement accepted")
	}
	if importCounts(t, s) != before {
		t.Fatal("partial imported sources escaped rollback")
	}
	for i := 0; i < 29; i++ {
		storeWorkbenchModel(t, s, u)
	}
	before = importCounts(t, s)
	if _, _, e = s.commitModelImport(u, "no-space.3mf", p, "quota-failure-test-01"); e == nil {
		t.Fatal("model quota bypass")
	}
	if importCounts(t, s) != before {
		t.Fatal("quota failure changed data")
	}
}
func TestImport3MFBackupRestoresProjectGeometryAndReceipt(t *testing.T) {
	s, u, _ := setup(t)
	p, e := parseModel3MF(context.Background(), mfTestZIP(t, mfFixtureFiles()))
	if e != nil {
		t.Fatal(e)
	}
	saved, _, e := s.commitModelImport(u, "backed-up.3mf", p, "backup-import-test-01")
	if e != nil {
		t.Fatal(e)
	}
	archive, cleanup, e := s.ExportBackup(u, "TestPassword123")
	if e != nil {
		t.Fatal(e)
	}
	defer cleanup()
	f, e := os.Open(archive)
	if e != nil {
		t.Fatal(e)
	}
	_, ticket, e := s.stageRestore(u, f)
	f.Close()
	if e != nil {
		t.Fatal(e)
	}
	if e = s.confirmRestore(u, ticket, "TestPassword123"); e != nil {
		t.Fatal(e)
	}
	cfg := s.Config
	s.Close()
	restored, e := Open(cfg, &fakeDevice{})
	if e != nil {
		t.Fatal(e)
	}
	defer restored.Close()
	before := importCounts(t, restored)
	again, replayed, e := restored.commitModelImport(u, "same-retry.3mf", p, "backup-import-test-01")
	if e != nil || !replayed || again.ID != saved.ID || !reflect.DeepEqual(again.Plates, saved.Plates) {
		t.Fatal(again, replayed, e)
	}
	if importCounts(t, restored) != before {
		t.Fatal("restored receipt duplicated sources")
	}
	if e = restored.transaction(func(tx *sql.Tx) error {
		mesh, e := loadPlate(tx, u, again.Plates[1].ModelID, again.Plates[1].Options)
		if e == nil && mesh.Bounds.Size != (Point3{12, 40, 18}) {
			t.Fatal("imported rotation lost after backup", mesh.Bounds)
		}
		return e
	}); e != nil {
		t.Fatal(e)
	}
}
func TestImport3MFRealStudio(t *testing.T) {
	engine, profiles := os.Getenv("LAYERHUB_TEST_SLICER"), os.Getenv("LAYERHUB_TEST_SLICER_PROFILES")
	if engine == "" || profiles == "" {
		t.Skip("optional real engine")
	}
	s, u, _ := setup(t)
	s.Config.SlicerPath = engine
	s.Config.SlicerProfiles = profiles
	p, e := parseModel3MF(context.Background(), mfTestZIP(t, mfFixtureFiles()))
	if e != nil {
		t.Fatal(e)
	}
	project, _, e := s.commitModelImport(u, "real-engine.3mf", p, "engine-import-test-01")
	if e != nil {
		t.Fatal(e)
	}
	for i, board := range project.Plates {
		if board.ModelID == 0 {
			continue
		}
		task, e := s.queueSlice(u, board.ModelID, board.Options)
		if e != nil {
			t.Fatal(e)
		}
		s.runSlice(task)
		result, e := s.slicerTask(u, task)
		if e != nil || result.State != "succeeded" {
			t.Fatal(result, e)
		}
		var output []byte
		s.DB.QueryRow("SELECT output FROM slice_tasks WHERE id=?", task).Scan(&output)
		paths := testOutputPaths(t, output)
		want := 50
		if i == 1 {
			want = 90
		}
		if len(paths.Layers) != want {
			t.Fatal("wrong imported geometry layers", len(paths.Layers), want)
		}
		t.Logf("import board=%d objects=%d layers=%d grams=%.2f", i+1, len(board.Options.Copies), len(paths.Layers), result.Info.Plates[0].Filaments[0].UsedGrams)
	}
}
