package app

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWorkbenchRealStudioDenseMesh(t *testing.T) {
	engine, profiles := os.Getenv("LAYERHUB_TEST_SLICER"), os.Getenv("LAYERHUB_TEST_SLICER_PROFILES")
	if engine == "" || profiles == "" || os.Getenv("LAYERHUB_TEST_DENSE") != "1" {
		t.Skip("optional dense real-engine benchmark")
	}
	s, _, _ := setup(t)
	s.Config.SlicerPath = engine
	s.Config.SlicerProfiles = profiles
	const segments, rings = 256, 256
	point := func(ring, index int) Point3 {
		z := 40 * float64(ring) / rings
		angle := 2 * math.Pi * float64(index%segments) / segments
		radius := 25 + 1.5*math.Sin(12*angle+3*math.Pi*z/40)
		return Point3{radius * math.Cos(angle), radius * math.Sin(angle), z}
	}
	m := ModelMesh{Vertices: make([]Point3, 0, (2*segments*rings+2*segments)*3)}
	for i := 0; i < rings; i++ {
		for j := 0; j < segments; j++ {
			a, b, c, d := point(i, j), point(i, j+1), point(i+1, j+1), point(i+1, j)
			m.Vertices = append(m.Vertices, a, b, c, a, c, d)
		}
	}
	for j := 0; j < segments; j++ {
		m.Vertices = append(m.Vertices, Point3{0, 0, 0}, point(0, j+1), point(0, j), Point3{0, 0, 40}, point(rings, j), point(rings, j+1))
	}
	m.Bounds, _ = meshBounds(m.Vertices)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	started := time.Now()
	artifact, info, err := s.executeSlice(ctx, encodeModelSTL(m), defaultSliceSetup())
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("dense mesh: triangles=%d output_bytes=%d duration=%s layers_estimate_seconds=%d", len(m.Vertices)/3, len(artifact), time.Since(started), info.Plates[0].Seconds)
}

func TestWorkbenchRealStudioModifiedSetup(t *testing.T) {
	engine, profiles := os.Getenv("LAYERHUB_TEST_SLICER"), os.Getenv("LAYERHUB_TEST_SLICER_PROFILES")
	if engine == "" || profiles == "" {
		t.Skip("optional real-engine parameter test")
	}
	s, _, _ := setup(t)
	s.Config.SlicerPath = engine
	s.Config.SlicerProfiles = profiles
	o := defaultSliceSetup()
	o.Layer = .16
	o.Infill = 20
	o.Brim = true
	o.Support = true
	o.X = 150
	o.Y = 120
	o.Rotation = Point3{30, 0, 45}
	o.Scale = 1.25
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	_, info, err := s.executeSlice(ctx, encodeModelSTL(workbenchMesh()), o)
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Plates) != 1 || info.Plates[0].Seconds <= 0 {
		t.Fatal("missing modified slice result")
	}
}

func workbenchMesh() ModelMesh {
	v := []Point3{{-10, -10, 0}, {10, -10, 0}, {10, 10, 0}, {-10, 10, 0}, {-10, -10, 10}, {10, -10, 10}, {10, 10, 10}, {-10, 10, 10}}
	faces := [][3]int{{0, 3, 2}, {0, 2, 1}, {4, 5, 6}, {4, 6, 7}, {0, 1, 5}, {0, 5, 4}, {1, 2, 6}, {1, 6, 5}, {2, 3, 7}, {2, 7, 6}, {3, 0, 4}, {3, 4, 7}}
	m := ModelMesh{}
	for _, f := range faces {
		for _, i := range f {
			m.Vertices = append(m.Vertices, v[i])
		}
	}
	m.Bounds, _ = meshBounds(m.Vertices)
	return m
}
func defaultSliceSetup() SliceSetup {
	return SliceSetup{X: 138.25, Y: 128, Scale: 1, Layer: .2, Infill: 15, Color: "#FFFFFF"}
}
func storeWorkbenchModel(t *testing.T, s *Service, u User) int64 {
	t.Helper()
	mesh := workbenchMesh()
	b, _ := json.Marshal(mesh.Bounds)
	r, e := s.DB.Exec("INSERT INTO model_sources(user_id,filename,stl,triangles,bounds,created) VALUES(?,?,?,?,?,?)", u.ID, "测试模型.stl", encodeModelSTL(mesh), 12, string(b), time.Now().Unix())
	if e != nil {
		t.Fatal(e)
	}
	id, _ := r.LastInsertId()
	return id
}
func TestWorkbenchGeometryPlacementAndMalformedInputs(t *testing.T) {
	mesh := workbenchMesh()
	round, err := parseModelSTL(encodeModelSTL(mesh))
	if err != nil || round.Bounds != mesh.Bounds {
		t.Fatal("STL roundtrip failed", err)
	}
	o := defaultSliceSetup()
	o.X = 150
	o.Y = 120
	o.Rotation = Point3{90, 0, 0}
	o.Scale = 2
	placed, err := transformModel(mesh, o)
	if err != nil || math.Abs(placed.Bounds.Min[0]-130) > .001 || math.Abs(placed.Bounds.Min[1]-110) > .001 || math.Abs(placed.Bounds.Max[2]-40) > .001 {
		t.Fatal("transform mismatch", placed.Bounds, err)
	}
	for _, edit := range []func(*SliceSetup){func(o *SliceSetup) { o.X = 0 }, func(o *SliceSetup) { o.Scale = 20 }, func(o *SliceSetup) { o.Layer = .01 }, func(o *SliceSetup) { o.Scale = math.NaN() }, func(o *SliceSetup) { o.Infill = 100 }, func(o *SliceSetup) { o.Rotation[0] = math.Inf(1) }} {
		o := defaultSliceSetup()
		edit(&o)
		if _, e := transformModel(mesh, o); e == nil {
			t.Fatal("invalid setup accepted", o)
		}
	}
	for _, b := range [][]byte{nil, []byte("not STL"), []byte("solid broken\nvertex nan 0 1\nendsolid"), encodeModelSTL(mesh)[:100]} {
		if _, e := parseModelSTL(b); e == nil {
			t.Fatal("malformed STL accepted")
		}
	}
}
func TestWorkbenchPrivacyQueueBoundsAndCancellation(t *testing.T) {
	s, u, _ := setup(t)
	other := notificationMember(t, s, u)
	id := storeWorkbenchModel(t, s, u)
	s.Config.SlicerPath = filepath.Join(t.TempDir(), "engine")
	os.WriteFile(s.Config.SlicerPath, []byte("test"), 0600)
	s.Config.SlicerProfiles = t.TempDir()
	for _, kind := range []string{"machine", "process", "filament"} {
		os.WriteFile(filepath.Join(s.Config.SlicerProfiles, kind+".json"), []byte("{}"), 0600)
	}
	if _, err := s.queueSlice(other, id, defaultSliceSetup()); err == nil {
		t.Fatal("foreign model sliced")
	}
	a, err := s.queueSlice(u, id, defaultSliceSetup())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.queueSlice(u, id, defaultSliceSetup()); err != nil {
		t.Fatal(err)
	}
	if _, err = s.queueSlice(u, id, defaultSliceSetup()); err == nil {
		t.Fatal("per-user queue bound ignored")
	}
	if err = s.cancelSlice(other, a); err == nil {
		t.Fatal("foreign task cancelled")
	}
	if err = s.cancelSlice(u, a); err != nil {
		t.Fatal(err)
	}
	page, err := s.sliceWorkspace(other)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.(map[string]any)["models"].([]ModelSource)) != 0 || len(page.(map[string]any)["tasks"].([]SliceTask)) != 0 {
		t.Fatal("private model list leaked")
	}
	w := httptest.NewRecorder()
	s.sliceFile(w, httptest.NewRequest("GET", "/", nil), other, "model", id)
	if w.Code != 404 {
		t.Fatal("private STL leaked", w.Code)
	}
	if err = s.deleteSliceModel(u, id); err == nil {
		t.Fatal("deleted model with queued work")
	}
}
func TestWorkbenchUploadUsesAuthenticationAndCSRF(t *testing.T) {
	s, u, _ := setup(t)
	token := randomID(32)
	s.DB.Exec("INSERT INTO sessions(hash,user_id,expires) VALUES(?,?,?)", tokenHash(token), u.ID, time.Now().Add(time.Hour).Unix())
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, _ := mw.CreateFormFile("file", "<model>.stl")
	part.Write(encodeModelSTL(workbenchMesh()))
	mw.Close()
	for _, csrf := range []bool{false, true} {
		r := httptest.NewRequest("POST", "/api/slicing/models", bytes.NewReader(body.Bytes()))
		r.Header.Set("Content-Type", mw.FormDataContentType())
		r.AddCookie(&http.Cookie{Name: "layerhub_sid", Value: token})
		if csrf {
			r.Header.Set("X-CSRF-Token", s.csrf(token))
		}
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		want := 403
		if csrf {
			want = 201
		}
		if w.Code != want {
			t.Fatal(w.Code, w.Body.String())
		}
	}
}
func TestWorkbenchRealStudioEndToEnd(t *testing.T) {
	engine, profiles := os.Getenv("LAYERHUB_TEST_SLICER"), os.Getenv("LAYERHUB_TEST_SLICER_PROFILES")
	if engine == "" || profiles == "" {
		t.Skip("optional real-engine test")
	}
	s, u, driver := setup(t)
	s.Config.SlicerPath = engine
	s.Config.SlicerProfiles = profiles
	p, sp := fixture(t, s, u)
	if _, e := s.DB.Exec("UPDATE printers SET model_id='x2d' WHERE id=?", p.ID); e != nil {
		t.Fatal(e)
	}
	sp.Material = "PETG"
	sp.Mount = &Mount{PrinterID: p.ID, Slot: "4"}
	if e := s.SaveSpool(u, sp); e != nil {
		t.Fatal(e)
	}
	model := storeWorkbenchModel(t, s, u)
	task, err := s.queueSlice(u, model, defaultSliceSetup())
	if err != nil {
		t.Fatal(err)
	}
	s.runSlice(task)
	var state, message string
	s.DB.QueryRow("SELECT state,error FROM slice_tasks WHERE id=?", task).Scan(&state, &message)
	if state != "succeeded" {
		t.Fatal(state, message)
	}
	job, err := s.promoteSlice(u, task, p.ID, sp.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.promoteSlice(u, task, p.ID, sp.ID); err == nil {
		t.Fatal("duplicate queue promotion")
	}
	jobs, err := readJobs(s.DB)
	if err != nil || len(jobs) != 1 || jobs[0].ID != job || jobs[0].UserID != u.ID || jobs[0].State != "queued" || jobs[0].Estimate < 1 || jobs[0].Grams != 3 || jobs[0].PrintOptions == nil || jobs[0].PrintOptions.Mapping[0].SlotID != 3 {
		t.Fatal("bad promoted job", jobs, err)
	}
	var remaining int
	s.DB.QueryRow("SELECT remaining FROM spools WHERE id=?", sp.ID).Scan(&remaining)
	if remaining != 1000 || driver.uploads != 0 {
		t.Fatal("promotion affected physical printer or stock")
	}
	if !strings.HasSuffix(jobs[0].Filename, ".gcode.3mf") {
		t.Fatal(jobs[0].Filename)
	}
	// Deleting the model/history must not remove the independently queued file.
	if err = s.deleteSliceModel(u, model); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(jobs[0].Path); err != nil {
		t.Fatal("queued artifact disappeared", err)
	}
}
