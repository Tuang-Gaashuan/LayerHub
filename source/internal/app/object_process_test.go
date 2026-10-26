package app

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func intValue(n int) *int           { return &n }
func floatValue(n float64) *float64 { return &n }
func textValue(n string) *string    { return &n }
func extendedSetup(model int64) SliceSetup {
	o := objectSetup(model)
	o.Copies[1].Process.TopThickness = floatValue(2)
	o.Copies[1].Process.BottomThickness = floatValue(1.6)
	o.Copies[1].Process.Pattern = textValue("rectilinear")
	return o
}
func TestObjectExtendedFormatAndWhitelist(t *testing.T) {
	s, u, _ := setup(t)
	model := storeWorkbenchModel(t, s, u)
	o := extendedSetup(model)
	state := SlicerState{FormatVersion: 4, ModelID: model, Options: o}
	saved, e := s.saveSlicerProject(u, ProjectWrite{Name: "扩展工艺", SlicerState: state})
	if e != nil {
		t.Fatal(e)
	}
	got, e := s.slicerProject(u, saved.ID)
	if e != nil || got.FormatVersion != 4 || !reflect.DeepEqual(got.Options.Copies, o.Copies) {
		t.Fatal("extended roundtrip", e)
	}
	old := got.SlicerState
	old.Options = objectSetup(model)
	old.Plates[0].Options = old.Options
	old.FormatVersion = 0
	if _, e = s.saveSlicerProject(u, ProjectWrite{ID: saved.ID, Revision: saved.Revision, Name: saved.Name, SlicerState: old}); e == nil {
		t.Fatal("version-one client dropped extensions")
	}
	old.FormatVersion = 4
	if _, e = s.saveSlicerProject(u, ProjectWrite{ID: saved.ID, Revision: saved.Revision, Name: saved.Name, SlicerState: old}); e != nil {
		t.Fatal("new client cannot restore inheritance", e)
	}
	for _, key := range []string{"topThickness", "bottomThickness", "pattern"} {
		bad := extendedSetup(model)
		if key == "topThickness" {
			bad.Copies[1].Process.TopThickness = floatValue(3.1)
		}
		if key == "bottomThickness" {
			bad.Copies[1].Process.BottomThickness = floatValue(.01)
		}
		if key == "pattern" {
			bad.Copies[1].Process.Pattern = textValue("external")
		}
		if validateObjectPlacements(bad) == nil {
			t.Fatal("bad extension accepted", key)
		}
	}
	settings := objectSettings(o.Copies[1].Process, .16)
	if settings["top_shell_layers"] != "13" || settings["bottom_shell_layers"] != "10" || settings["sparse_infill_pattern"] != "zig-zag" {
		t.Fatal("object shell rounding or alias", settings)
	}
}
func TestObjectOldClientCannotQueueAnIncompleteSlice(t *testing.T) {
	s, u, _ := setup(t)
	s.Config.SlicerPath = filepath.Join(t.TempDir(), "engine")
	os.WriteFile(s.Config.SlicerPath, []byte("test"), 0600)
	s.Config.SlicerProfiles = t.TempDir()
	for _, name := range []string{"machine", "process", "filament"} {
		os.WriteFile(filepath.Join(s.Config.SlicerProfiles, name+".json"), []byte(`{}`), 0600)
	}
	id := storeWorkbenchModel(t, s, u)
	token := randomID(32)
	s.DB.Exec("INSERT INTO sessions(hash,user_id,expires) VALUES(?,?,?)", tokenHash(token), u.ID, time.Now().Add(time.Hour).Unix())
	for _, version := range []int{0, 2} {
		raw, _ := json.Marshal(map[string]any{"clientVersion": version, "modelId": id, "options": objectSetup(id)})
		r := httptest.NewRequest("POST", "/api/slicing/tasks", bytes.NewReader(raw))
		r.AddCookie(&http.Cookie{Name: "layerhub_sid", Value: token})
		r.Header.Set("X-CSRF-Token", s.csrf(token))
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if version == 0 && w.Code != 409 {
			t.Fatal("old client not blocked", w.Code, w.Body.String())
		}
		if version == 2 && w.Code != 200 {
			t.Fatal("new client rejected", w.Code, w.Body.String())
		}
	}
	var count int
	s.DB.QueryRow("SELECT COUNT(*) FROM slice_tasks").Scan(&count)
	if count != 1 {
		t.Fatal("old client created a slice", count)
	}
}
func TestObjectVersionThreeCanUpgradeProjectAndDraft(t *testing.T) {
	s, u, _ := setup(t)
	model := storeWorkbenchModel(t, s, u)
	old := SlicerState{FormatVersion: 4, ModelID: model, Options: objectSetup(model), Plates: []SlicerPlate{{ID: "plate-a", Name: "升级", ModelID: model, Options: objectSetup(model)}}}
	p, e := s.saveSlicerProject(u, ProjectWrite{Name: "升级", SlicerState: old})
	if e != nil {
		t.Fatal(e)
	}
	revision, e := s.saveSlicerDraft(u, DraftWrite{SlicerState: old}, false)
	if e != nil {
		t.Fatal(e)
	}
	next := old
	next.Options = extendedSetup(model)
	next.Plates[0].Options = next.Options
	if _, e = s.saveSlicerProject(u, ProjectWrite{ID: p.ID, Revision: p.Revision, Name: p.Name, SlicerState: next}); e != nil {
		t.Fatal("version3 upgrade blocked", e)
	}
	if _, e = s.saveSlicerDraft(u, DraftWrite{Revision: revision, SlicerState: next}, false); e != nil {
		t.Fatal("version3 draft upgrade blocked", e)
	}
}
func objectSetup(model int64) SliceSetup {
	o := defaultSliceSetup()
	o.Infill = 10
	o.Copies = []ModelPlacement{{ModelID: model, InstanceID: "instance-a", X: 90, Y: 128, Scale: 1}, {ModelID: model, InstanceID: "instance-b", X: 175, Y: 128, Scale: 1, Process: &ObjectProcess{Walls: intValue(4), Infill: intValue(30)}}}
	return o
}

func TestObjectProcessIdentityValidationAndLegacy(t *testing.T) {
	base := objectSetup(1)
	if e := validateObjectPlacements(base); e != nil {
		t.Fatal(e)
	}
	legacy := defaultSliceSetup()
	legacy.Copies = []ModelPlacement{{ModelID: 1, X: 90, Y: 128, Scale: 1}}
	if e := validateObjectPlacements(legacy); e != nil {
		t.Fatal("legacy rejected", e)
	}
	if hasObjectOverrides(legacy) {
		t.Fatal("legacy selected new engine")
	}
	for _, change := range []func(*SliceSetup){func(o *SliceSetup) { o.Copies[1].InstanceID = o.Copies[0].InstanceID }, func(o *SliceSetup) { o.Copies[1].InstanceID = "../x" }, func(o *SliceSetup) { o.Copies[0].InstanceID = "" }, func(o *SliceSetup) { o.Copies[1].Process.Walls = intValue(7) }, func(o *SliceSetup) { o.Copies[1].Process.Infill = intValue(0) }} {
		raw, _ := json.Marshal(base)
		var o SliceSetup
		json.Unmarshal(raw, &o)
		change(&o)
		if e := validateObjectPlacements(o); e == nil {
			t.Fatal("invalid identity/override accepted", o)
		}
	}
}
func TestObjectProjectRoundtripDowngradeAndBackup(t *testing.T) {
	s, u, _ := setup(t)
	model := storeWorkbenchModel(t, s, u)
	o := extendedSetup(model)
	state := SlicerState{FormatVersion: 4, ModelID: model, Options: o, Plates: []SlicerPlate{{ID: "plate-a", Name: "独立工艺", ModelID: model, Options: o}}}
	saved, e := s.saveSlicerProject(u, ProjectWrite{Name: "对象工艺", SlicerState: state})
	if e != nil {
		t.Fatal(e)
	}
	got, e := s.slicerProject(u, saved.ID)
	if e != nil || !reflect.DeepEqual(got.Plates, state.Plates) {
		t.Fatal("roundtrip", got, e)
	}
	if _, e = s.saveSlicerDraft(u, DraftWrite{SlicerState: state}, false); e != nil {
		t.Fatal(e)
	}
	raw := encodeSlicerOptions(state)
	if !strings.Contains(raw, `"version":4`) {
		t.Fatal("missing new format", raw)
	}
	legacy := state
	legacy.Plates = nil
	legacy.Options = defaultSliceSetup()
	if _, e = s.saveSlicerProject(u, ProjectWrite{ID: saved.ID, Revision: saved.Revision, Name: saved.Name, SlicerState: legacy}); e == nil {
		t.Fatal("cached old client dropped overrides")
	}
	if _, e = s.saveSlicerDraft(u, DraftWrite{Revision: 1, SlicerState: legacy}, false); e == nil {
		t.Fatal("cached draft dropped overrides")
	}
	bad := state
	bad.Plates = append(append([]SlicerPlate{}, state.Plates...), SlicerPlate{ID: "plate-b", Name: "重复身份", ModelID: model, Options: o})
	if _, e = s.saveSlicerProject(u, ProjectWrite{Name: "坏工程", SlicerState: bad}); e == nil {
		t.Fatal("cross-plate duplicate IDs accepted")
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
	got, e = restored.slicerProject(u, saved.ID)
	if e != nil || !reflect.DeepEqual(got.Plates, state.Plates) {
		t.Fatal("backup lost objects", e)
	}
}
func TestObjectGeometryChecksTransformsWindingAndDuplicatedFaces(t *testing.T) {
	a := workbenchMesh()
	b := a
	b.Vertices = append([]Point3{}, a.Vertices...)
	if !sameObjectGeometry(a, b) {
		t.Fatal("same rejected")
	}
	b.Vertices[0][0] += .01
	if sameObjectGeometry(a, b) {
		t.Fatal("shape modification accepted")
	}
	b.Vertices = append([]Point3{}, a.Vertices...)
	b.Vertices[0], b.Vertices[1] = b.Vertices[1], b.Vertices[0]
	if sameObjectGeometry(a, b) {
		t.Fatal("reversed winding accepted")
	}
	b.Vertices = append([]Point3{}, a.Vertices...)
	copy(b.Vertices[3:6], b.Vertices[:3])
	if sameObjectGeometry(a, b) {
		t.Fatal("duplicated face accepted")
	}
}

func TestObjectLegacyResultSignatureAndBoundedProjectBody(t *testing.T) {
	modern := objectSetup(1)
	modern.Copies[1].Process = nil
	legacy := modern
	legacy.Copies = append([]ModelPlacement{}, modern.Copies...)
	for i := range legacy.Copies {
		legacy.Copies[i].InstanceID = ""
	}
	if sliceLayoutSignature(modern) != sliceLayoutSignature(legacy) {
		t.Fatal("adding identity invalidated an unchanged legacy result")
	}
	changed := objectSetup(1)
	if sliceLayoutSignature(changed) == sliceLayoutSignature(legacy) {
		t.Fatal("object override did not invalidate result")
	}
	state := SlicerState{}
	for plate := 0; plate < 4; plate++ {
		o := defaultSliceSetup()
		for i := 0; i < 64; i++ {
			o.Copies = append(o.Copies, ModelPlacement{ModelID: 1, InstanceID: fmt.Sprintf("00000000-0000-0000-0000-%012d", plate*64+i), X: 123.123456789012, Y: 234.234567890123, Scale: 1.123456789012, Rotation: Point3{12.123456789012, 23.234567890123, 34.345678901234}, Process: &ObjectProcess{Walls: intValue(4), Infill: intValue(30)}})
		}
		for i := range o.Copies {
			o.Copies[i].Process = extendedSetup(1).Copies[1].Process
		}
		state.Plates = append(state.Plates, SlicerPlate{ID: fmt.Sprint(plate), Name: "独立工艺", ModelID: 1, Options: o})
	}
	normalizeSlicerState(&state)
	raw, _ := json.Marshal(ProjectWrite{Name: "最大工程", SlicerState: state})
	if len(raw) > 128<<10 {
		t.Fatal("legal request no longer fits")
	}
	var decoded ProjectWrite
	if e := decode(httptest.NewRequest("POST", "/api/slicing/project", bytes.NewReader(raw)), &decoded); e != nil {
		t.Fatal("legal instance metadata truncated", len(raw), e)
	}
	if e := decode(httptest.NewRequest("POST", "/api/slicing/project", strings.NewReader(`{"name":"x","options":{"copies":[{"process":{"temperature":300}}]}}`)), &decoded); e == nil {
		t.Fatal("unapproved object setting accepted")
	}
}
func TestObjectPinnedEngineEndToEnd(t *testing.T) {
	if os.Getenv("LAYERHUB_OBJECT_POC") != "1" {
		t.Skip("explicit pinned engine experiment")
	}
	s, u, _ := setup(t)
	s.Config.SlicerPath = os.Getenv("LAYERHUB_OBJECT_ENGINE")
	if s.Config.SlicerPath == "" {
		s.Config.SlicerPath, _ = filepath.Abs("../../qa/slicing-poc/runtime/bambu-studio.exe")
	}
	s.Config.SlicerProfiles = os.Getenv("LAYERHUB_OBJECT_PROFILES")
	if s.Config.SlicerProfiles == "" {
		s.Config.SlicerProfiles, _ = filepath.Abs("../../qa/slicing-poc/flattened")
	}
	model := storeWorkbenchModel(t, s, u)
	o := objectSetup(model)
	var mesh ModelMesh
	var objects []sliceObject
	e := s.transaction(func(tx *sql.Tx) error {
		var e error
		mesh, e = loadPlate(tx, u, model, o)
		if e != nil {
			return e
		}
		objects, e = loadSliceObjects(tx, u, model, o)
		return e
	})
	if e != nil {
		t.Fatal(e)
	}
	output, info, e := s.executePreparedPlate(context.Background(), mesh, o, objects)
	if e != nil {
		t.Fatal("object engine", e)
	}
	if len(output) == 0 || len(info.Plates) != 1 {
		t.Fatal("missing output")
	}
	artifact := filepath.Join(t.TempDir(), "output.3mf")
	os.WriteFile(artifact, output, 0600)
	if e = verifyObjectOutput(artifact, objects, o); e != nil {
		t.Fatal(e)
	}
	// An engine result that silently discards an override is not accepted.
	z, e := zip.NewReader(bytes.NewReader(output), int64(len(output)))
	if e != nil {
		t.Fatal(e)
	}
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	for _, entry := range z.File {
		r, _ := entry.Open()
		raw, _ := io.ReadAll(r)
		r.Close()
		if entry.Name == "Metadata/model_settings.config" {
			raw = bytes.ReplaceAll(raw, []byte(`key="wall_loops" value="4"`), []byte(`key="wall_loops" value="2"`))
		}
		w, _ := zw.Create(entry.Name)
		w.Write(raw)
	}
	zw.Close()
	os.WriteFile(artifact, b.Bytes(), 0600)
	if e = verifyObjectOutput(artifact, objects, o); e == nil {
		t.Fatal("ignored override accepted")
	}
	id, e := s.queueSlice(u, model, o)
	if e != nil {
		t.Fatal(e)
	}
	s.runSlice(id)
	task, e := s.slicerTask(u, id)
	if e != nil || task.State != "succeeded" {
		t.Fatal("queued object slice", task.State, task.Error, e)
	}
	t.Logf("two-instance object override output: %d bytes", len(output))
	signatures := map[string]bool{}
	for _, pattern := range []string{"grid", "gyroid", "rectilinear"} {
		extra := extendedSetup(model)
		extra.Copies[1].Process.Pattern = textValue(pattern)
		id, e := s.queueSlice(u, model, extra)
		if e != nil {
			t.Fatal(e)
		}
		s.runSlice(id)
		task, e := s.slicerTask(u, id)
		if e != nil || task.State != "succeeded" {
			t.Fatal("extended object engine", pattern, task.Error, e)
		}
		var raw []byte
		if e = s.DB.QueryRow("SELECT output FROM slice_tasks WHERE id=?", id).Scan(&raw); e != nil {
			t.Fatal(e)
		}
		paths := testOutputPaths(t, raw)
		for _, layer := range []int{5, 43} {
			counts := [2][7]int{}
			for _, seg := range paths.Layers[layer].Segments {
				for i, x := range []float32{90, 175} {
					if seg[0] >= x-10.01 && seg[0] <= x+10.01 && seg[3] >= x-10.01 && seg[3] <= x+10.01 && seg[1] >= 117.99 && seg[1] <= 138.01 && seg[4] >= 117.99 && seg[4] <= 138.01 {
						counts[i][int(seg[6])]++
					}
				}
			}
			if counts[0][2] == 0 || counts[0][3] != 0 || counts[1][3] == 0 || counts[1][2] != 0 {
				t.Fatal("object shell paths do not match", pattern, layer, counts)
			}
		}
		segments := []PathSegment{}
		for _, seg := range paths.Layers[24].Segments {
			if seg[6] == 2 && seg[0] > 164.99 && seg[3] > 164.99 {
				segments = append(segments, seg)
			}
		}
		key := fmt.Sprint(segments)
		if len(segments) == 0 || signatures[key] {
			t.Fatal("object pattern did not change real infill", pattern)
		}
		signatures[key] = true
	}
}

func TestObjectBracketFaceEndToEnd(t *testing.T) {
	if os.Getenv("LAYERHUB_OBJECT_POC") != "1" || os.Getenv("LAYERHUB_OBJECT_BRACKET") == "" {
		t.Skip("explicit offline bracket experiment")
	}
	s, u, _ := setup(t)
	s.Config.SlicerPath = os.Getenv("LAYERHUB_OBJECT_ENGINE")
	s.Config.SlicerProfiles = os.Getenv("LAYERHUB_OBJECT_PROFILES")
	raw, e := os.ReadFile(os.Getenv("LAYERHUB_OBJECT_BRACKET"))
	if e != nil {
		t.Fatal(e)
	}
	mesh, e := parseModelSTL(raw)
	if e != nil {
		t.Fatal(e)
	}
	poseRaw, e := os.ReadFile(os.Getenv("LAYERHUB_OBJECT_POSE"))
	if e != nil {
		t.Fatal(e)
	}
	var pose struct {
		Rotation Point3 `json:"rotation"`
	}
	if e = json.Unmarshal(poseRaw, &pose); e != nil {
		t.Fatal(e)
	}
	b, _ := json.Marshal(mesh.Bounds)
	r, e := s.DB.Exec("INSERT INTO model_sources(user_id,filename,stl,triangles,bounds,created) VALUES(?,?,?,?,?,?)", u.ID, "offline-bracket.stl", raw, len(mesh.Vertices)/3, string(b), 0)
	if e != nil {
		t.Fatal(e)
	}
	model, _ := r.LastInsertId()
	o := defaultSliceSetup()
	o.Support = true
	o.Rotation = pose.Rotation
	o.Copies = []ModelPlacement{{ModelID: model, InstanceID: "bracket-fixture", X: 138.25, Y: 128, Scale: 1, Rotation: pose.Rotation, Process: &ObjectProcess{Walls: intValue(3), TopThickness: floatValue(.8), BottomThickness: floatValue(.8), Pattern: textValue("gyroid")}}}
	id, e := s.queueSlice(u, model, o)
	if e != nil {
		t.Fatal(e)
	}
	s.runSlice(id)
	task, e := s.slicerTask(u, id)
	if e != nil || task.State != "succeeded" {
		t.Fatal("bracket face slice", task.Error, e)
	}
	var output []byte
	s.DB.QueryRow("SELECT output FROM slice_tasks WHERE id=?", id).Scan(&output)
	t.Logf("bracket: %d triangles, %d bytes, all output triangles verified", len(mesh.Vertices)/3, len(output))
}
