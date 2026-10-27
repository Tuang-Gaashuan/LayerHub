package app

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func intakeFixture() map[string]string {
	return map[string]string{
		"Metadata/project_settings.config": `{"printer_model":"Bambu Lab X2D","layer_height":"0.20","filament_type":["PETG"],"machine_start_gcode":"PRIVATE_MACRO_CANARY"}`,
		"Metadata/plate_1.gcode":           "; TEST FIXTURE ONLY",
		"Metadata/slice_info.config":       `<config><plate><metadata key="index" value="1"/><metadata key="prediction" value="3701"/><filament id="1" type="PETG" color="#FFFFFF" used_g="12.1"/></plate></config>`,
	}
}
func betaCounts(t *testing.T, s *Service) []int {
	t.Helper()
	out := []int{}
	for _, table := range []string{"jobs", "spools", "printer_commands"} {
		var n int
		if e := s.DB.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); e != nil {
			t.Fatal(e)
		}
		out = append(out, n)
	}
	return out
}
func betaFileHTTP(t *testing.T, s *Service, token, path, name string, raw []byte, csrf bool) *httptest.ResponseRecorder {
	t.Helper()
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	f, _ := mw.CreateFormFile("file", name)
	f.Write(raw)
	mw.WriteField("schedule", `{"printer":0,"estimate":62,"grams":13,"spoolId":"missing"}`)
	mw.Close()
	// Inspection only accepts the file part, so build that form independently.
	if path == "/api/files/inspect" {
		b.Reset()
		mw = multipart.NewWriter(&b)
		f, _ = mw.CreateFormFile("file", name)
		f.Write(raw)
		mw.Close()
	}
	r := httptest.NewRequest("POST", path, &b)
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
func TestBetaAcceptsOnlyActualSliced3MFWithoutGeometryParsing(t *testing.T) {
	s, _, driver := setup(t)
	token := securityLogin(t, s, "admin", "TestPassword123", "beta-file")
	before := betaCounts(t, s)
	files := intakeFixture()
	files["3D/3dmodel.model"] = "THIS IS NOT PARSED OR EXECUTED"
	raw := sliceFixture(t, files)
	w := betaFileHTTP(t, s, token, "/api/files/inspect", "project.3mf", raw, true)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var result struct {
		Review fileReview `json:"review"`
	}
	json.Unmarshal(w.Body.Bytes(), &result)
	if result.Review.Schema != 2 || result.Review.Kind != "sliced-3mf" || !result.Review.Sliced.Available || result.Review.Sliced.Info.Plates[0].Seconds != 3701 {
		t.Fatal(w.Body.String())
	}
	if strings.Contains(w.Body.String(), "PRIVATE_MACRO_CANARY") || strings.Contains(w.Body.String(), `"geometry"`) {
		t.Fatal("geometry or macros returned")
	}
	for _, c := range []struct {
		path, name string
		raw        []byte
	}{{"/api/files/inspect", "raw.gcode", raw}, {"/api/upload", "raw.gcode", raw}, {"/api/upload", "model.stl", raw}, {"/api/files/inspect", "model.3mf", sliceFixture(t, map[string]string{"3D/3dmodel.model": "<model/>"})}, {"/api/upload", "model.3mf", sliceFixture(t, map[string]string{"3D/3dmodel.model": "<model/>"})}} {
		if got := betaFileHTTP(t, s, token, c.path, c.name, c.raw, true); got.Code != 400 {
			t.Fatal(c.name, got.Code, got.Body.String())
		}
	}
	if !reflect.DeepEqual(betaCounts(t, s), before) || driver.uploads != 0 {
		t.Fatal("inspection wrote business data or dispatched")
	}
	left, _ := filepath.Glob(filepath.Join(s.Config.DataDir, "uploads", "*"))
	if len(left) != 0 {
		t.Fatal("inspection/rejection left temporary uploads")
	}
}
func TestBetaInspectionAuthLimitsAndUnsafeArchives(t *testing.T) {
	s, _, _ := setup(t)
	token := securityLogin(t, s, "admin", "TestPassword123", "beta-auth")
	raw := sliceFixture(t, intakeFixture())
	for _, c := range []struct {
		token string
		csrf  bool
		want  int
	}{{"", false, 401}, {token, false, 403}} {
		if w := betaFileHTTP(t, s, c.token, "/api/files/inspect", "test.3mf", raw, c.csrf); w.Code != c.want {
			t.Fatal(w.Code)
		}
	}
	for _, entry := range []string{"../escape", "Metadata/PROJECT_SETTINGS.CONFIG"} {
		files := intakeFixture()
		files[entry] = "bad"
		if w := betaFileHTTP(t, s, token, "/api/files/inspect", "test.3mf", sliceFixture(t, files), true); w.Code != 400 {
			t.Fatal("unsafe archive accepted", entry, w.Code)
		}
	}
	s.Config.MaxUpload = 100
	if w := betaFileHTTP(t, s, token, "/api/files/inspect", "test.3mf", raw, true); w.Code != 400 {
		t.Fatal("size limit ignored")
	}
	name := filepath.Join(t.TempDir(), "file.3mf")
	os.WriteFile(name, raw, 0600)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := inspectPrintFile(ctx, name, "", int64(len(raw))); e == nil {
		t.Fatal("cancellation ignored")
	}
}
func TestBetaRetiresWorkbenchRoutesAndKeepsLegacyRows(t *testing.T) {
	s, u, _ := setup(t)
	token := securityLogin(t, s, "admin", "TestPassword123", "beta-retired")
	for _, path := range []string{"/api/slicing", "/api/slicing/models", "/api/slicing/import-3mf", "/api/slicing/tasks", "/api/team-projects", "/api/team-projects/publish"} {
		for _, method := range []string{"GET", "POST"} {
			r := httptest.NewRequest(method, path, strings.NewReader(`{}`))
			r.AddCookie(&http.Cookie{Name: "layerhub_sid", Value: token})
			r.Header.Set("X-CSRF-Token", s.csrf(token))
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, r)
			if w.Code != 410 {
				t.Fatal(method, path, w.Code)
			}
		}
	}
	snapshot, e := s.Snapshot(u)
	if e != nil {
		t.Fatal(e)
	}
	b, _ := json.Marshal(snapshot)
	if !strings.Contains(string(b), `"sliced3MFOnly":true`) || !strings.Contains(string(b), `"modelWorkbench":false`) {
		t.Fatal(string(b))
	}
	var count int
	s.DB.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE name='model_sources'").Scan(&count)
	if count != 0 {
		t.Fatal("fresh beta created legacy workbench storage")
	}
	s.DB.Exec("CREATE TABLE model_sources(id INTEGER PRIMARY KEY,payload TEXT)")
	s.DB.Exec("INSERT INTO model_sources VALUES(1,'private-alpha-project')")
	cfg := s.Config
	s.Close()
	next, e := Open(cfg, &fakeDevice{})
	if e != nil {
		t.Fatal(e)
	}
	defer next.Close()
	var value string
	if e = next.DB.QueryRow("SELECT payload FROM model_sources WHERE id=1").Scan(&value); e != nil || value != "private-alpha-project" {
		t.Fatal("legacy data changed", e)
	}
	archive, cleanup, e := next.ExportBackup(u, "TestPassword123")
	if e != nil {
		t.Fatal(e)
	}
	defer cleanup()
	dir := t.TempDir()
	if _, e = unpackBackup(archive, dir); e != nil {
		t.Fatal(e)
	}
	db, e := openBackupDB(filepath.Join(dir, "layerhub.sqlite"), true)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	if e = db.QueryRow("SELECT payload FROM model_sources WHERE id=1").Scan(&value); e != nil || value != "private-alpha-project" {
		t.Fatal("legacy row omitted from backup", e)
	}
}
