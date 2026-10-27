package app

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func intakeFixture() map[string]string {
	files := mfFixtureFiles()
	files["Metadata/plate_1.gcode"] = "; test fixture only"
	files["Metadata/slice_info.config"] = `<config><plate><metadata key="index" value="1"/><metadata key="prediction" value="3701"/><filament id="1" type="PETG" color="#FFFFFF" used_g="12.1"/></plate></config>`
	return files
}
func intakeReview(t *testing.T, files map[string]string) fileReview {
	t.Helper()
	raw := mfTestZIP(t, files)
	name := filepath.Join(t.TempDir(), "input.bin")
	os.WriteFile(name, raw, 0600)
	r, e := inspectPrintFile(context.Background(), name, "digest", int64(len(raw)))
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func TestFileIntakeIndependentRoutesAndReferenceWhitelist(t *testing.T) {
	files := intakeFixture()
	r := intakeReview(t, files)
	if !r.Sliced.Available || !r.Geometry.Available || r.Reference.Printer != "Bambu Lab A1 mini" || r.Geometry.Review.Reference.Layer != "0.08" || r.Sliced.Info.Plates[0].Seconds != 3701 {
		t.Fatalf("incomplete inspection: %+v", r)
	}
	b, _ := json.Marshal(r)
	if strings.Contains(string(b), "ORIGINAL_PROFILE_SENTINEL") {
		t.Fatal("embedded machine macro exposed")
	}
	files["Metadata/model_settings.config"] = strings.Replace(files["Metadata/model_settings.config"], `subtype="normal_part"`, `subtype="negative_part"`, 1)
	r = intakeReview(t, files)
	if r.Geometry.Available || !r.Sliced.Available || !strings.Contains(r.Geometry.Reason, "负体积") {
		t.Fatal("geometry limitation disabled valid sliced route", r)
	}
	delete(files, "Metadata/slice_info.config")
	files["Metadata/model_settings.config"] = mfFixtureFiles()["Metadata/model_settings.config"]
	r = intakeReview(t, files)
	if !r.Geometry.Available || r.Sliced.Available {
		t.Fatal("geometry-only project misclassified", r)
	}
	r = intakeReview(t, map[string]string{"Metadata/plate_1.gcode": ";fixture", "Metadata/slice_info.config": intakeFixture()["Metadata/slice_info.config"]})
	if r.Geometry.Available || !r.Sliced.Available {
		t.Fatal("slice-only file rejected", r)
	}
}
func TestFileIntakeUnsafePathsCancellationAndBoundedReference(t *testing.T) {
	for _, name := range []string{"../escape", "Metadata/PROJECT_SETTINGS.CONFIG"} {
		files := intakeFixture()
		files[name] = "bad"
		raw := mfTestZIP(t, files)
		p := filepath.Join(t.TempDir(), "in.3mf")
		os.WriteFile(p, raw, 0600)
		if _, e := inspectPrintFile(context.Background(), p, "", int64(len(raw))); e == nil {
			t.Fatal("unsafe archive accepted", name)
		}
	}
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	h := zip.FileHeader{Name: "link"}
	h.SetMode(os.ModeSymlink | 0600)
	f, _ := w.CreateHeader(&h)
	f.Write([]byte("/etc/passwd"))
	w.Close()
	p := filepath.Join(t.TempDir(), "in.3mf")
	os.WriteFile(p, buf.Bytes(), 0600)
	if _, e := inspectPrintFile(context.Background(), p, "", int64(buf.Len())); e == nil {
		t.Fatal("symlink accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := inspectPrintFile(ctx, p, "", int64(buf.Len())); e == nil {
		t.Fatal("cancelled context ignored")
	}
	r := importReference([]byte(`{"printer_model":"<script>\u0000test</script>","filament_type":["PETG"],"layer_height":{"nested":"ignored"},"machine_start_gcode":"SECRET"}`))
	if r == nil || r.Layer != "" || strings.Contains(r.Printer, "\x00") {
		t.Fatal(r)
	}
	if importReference([]byte(`{"private":"SECRET"}`)) != nil {
		t.Fatal("unknown metadata exported")
	}
}
func intakeHTTP(t *testing.T, s *Service, token string, csrf bool, raw []byte, repeat bool) *httptest.ResponseRecorder {
	t.Helper()
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	f, _ := mw.CreateFormFile("file", "no-gcode-suffix.3mf")
	f.Write(raw)
	if repeat {
		f, _ = mw.CreateFormFile("file", "another.3mf")
		f.Write(raw)
	}
	mw.Close()
	r := httptest.NewRequest("POST", "/api/files/inspect", &b)
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
func TestFileIntakeHTTPAuthorizationNoBusinessWritesOrLeftovers(t *testing.T) {
	s, _, _ := setup(t)
	token := securityLogin(t, s, "admin", "TestPassword123", "intake")
	raw := mfTestZIP(t, intakeFixture())
	before := importCounts(t, s)
	for _, c := range []struct {
		token        string
		csrf, repeat bool
		status       int
	}{{"", false, false, 401}, {token, false, false, 403}, {token, true, true, 400}, {token, true, false, 200}} {
		w := intakeHTTP(t, s, c.token, c.csrf, raw, c.repeat)
		if w.Code != c.status {
			t.Fatal(w.Code, w.Body.String())
		}
		if importCounts(t, s) != before {
			t.Fatal("inspection changed business data")
		}
		files, _ := filepath.Glob(filepath.Join(s.Config.DataDir, "uploads", "inspect-*"))
		if len(files) != 0 {
			t.Fatal("temporary inspection persisted", files)
		}
	}
	s.Config.MaxUpload = 100
	if w := intakeHTTP(t, s, token, true, raw, false); w.Code != 400 {
		t.Fatal("upload limit ignored", w.Code)
	}
	files, _ := filepath.Glob(filepath.Join(s.Config.DataDir, "uploads", "inspect-*"))
	if len(files) != 0 {
		t.Fatal("failed upload leaked temporary file")
	}
}

func TestUploadRejectsUnslicedProjectBeforeQueueAndReservation(t *testing.T) {
	s, u, driver := setup(t)
	p, sp := fixture(t, s, u)
	token := securityLogin(t, s, "admin", "TestPassword123", "upload-intake")
	before := importCounts(t, s)
	for _, sliced := range []bool{false, true} {
		files := mfFixtureFiles()
		if sliced {
			files = intakeFixture()
		}
		raw := mfTestZIP(t, files)
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		f, _ := mw.CreateFormFile("file", "project.3mf")
		f.Write(raw)
		b, _ := json.Marshal(Schedule{Printer: p.ID, SpoolID: sp.ID, Estimate: 62, Grams: 13})
		mw.WriteField("schedule", string(b))
		mw.Close()
		r := httptest.NewRequest("POST", "/api/upload", &body)
		r.Header.Set("Content-Type", mw.FormDataContentType())
		r.Header.Set("X-CSRF-Token", s.csrf(token))
		r.AddCookie(&http.Cookie{Name: "layerhub_sid", Value: token})
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if !sliced {
			if w.Code != 400 || importCounts(t, s) != before {
				t.Fatal("unsliced project entered queue", w.Code, w.Body.String())
			}
			left, _ := filepath.Glob(filepath.Join(s.Config.DataDir, "uploads", "pending-*"))
			if len(left) != 0 {
				t.Fatal("rejected file persisted")
			}
		} else if w.Code != 201 || importCounts(t, s)[4] != before[4]+1 {
			t.Fatal("valid slice rejected", w.Code, w.Body.String())
		}
	}
	if driver.uploads != 0 {
		t.Fatal("file intake sent data to printer")
	}
	ss, _ := readSpools(s.DB)
	if ss[0].Remaining != sp.Remaining {
		t.Fatal("upload deducted physical inventory")
	}
}
