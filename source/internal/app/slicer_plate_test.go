package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"testing"
	"time"
)

func mixedMesh() ModelMesh {
	m := workbenchMesh()
	for i := range m.Vertices {
		m.Vertices[i][0] *= 2
		m.Vertices[i][1] *= .6
		m.Vertices[i][2] *= 1.8
	}
	m.Bounds, _ = meshBounds(m.Vertices)
	return m
}
func mixedOptions(a, b int64) SliceSetup {
	o := defaultSliceSetup()
	o.Copies = []ModelPlacement{{ModelID: a, X: 70, Y: 90, Scale: 1}, {ModelID: b, X: 120, Y: 90, Scale: 1}}
	return o
}
func storeMixedSources(t *testing.T, s *Service, u User) (int64, int64) {
	t.Helper()
	a, b := storeWorkbenchModel(t, s, u), storeWorkbenchModel(t, s, u)
	m := mixedMesh()
	bounds, _ := json.Marshal(m.Bounds)
	if _, e := s.DB.Exec("UPDATE model_sources SET filename='mixed-box.stl',stl=?,bounds=? WHERE id=?", encodeModelSTL(m), string(bounds), b); e != nil {
		t.Fatal(e)
	}
	return a, b
}
func TestMixedPlateGeometryAndLimits(t *testing.T) {
	o := mixedOptions(1, 2)
	sources := map[int64]ModelMesh{1: workbenchMesh(), 2: mixedMesh()}
	m, e := transformModels(sources, 1, o)
	if e != nil || len(m.Vertices) != 72 || m.Bounds.Size != (Point3{80, 20, 18}) {
		t.Fatal(m.Bounds, e)
	}
	// The second object must retain its distinct 40 x 12 x 18 geometry.
	box, e := meshBounds(m.Vertices[36:])
	if e != nil || box.Size != (Point3{40, 12, 18}) {
		t.Fatal(box, e)
	}
	o.Copies[1].X = 95
	if _, e = transformModels(sources, 1, o); e == nil {
		t.Fatal("mixed overlap accepted")
	}
	o = mixedOptions(1, 2)
	delete(sources, 2)
	if _, e = transformModels(sources, 1, o); e == nil {
		t.Fatal("missing secondary accepted")
	}
	sources[2] = ModelMesh{Vertices: make([]Point3, 150000*3)}
	if _, e = transformModels(sources, 1, o); e == nil {
		t.Fatal("mixed aggregate limit bypassed")
	}
}
func TestMixedPlateOwnershipAndReferences(t *testing.T) {
	s, u, _ := setup(t)
	a, b := storeMixedSources(t, s, u)
	o := mixedOptions(a, b)
	state := SlicerState{ModelID: a, Options: o, ActiveCopy: 1}
	p, e := s.saveSlicerProject(u, ProjectWrite{Name: "mixed", SlicerState: state})
	if e != nil {
		t.Fatal(e)
	}
	if e = s.deleteSliceModel(u, b); e == nil {
		t.Fatal("secondary project source deleted")
	}
	if e = s.deleteSlicerProject(u, p.ID, p.Revision); e != nil {
		t.Fatal(e)
	}
	if _, e = s.saveSlicerDraft(u, DraftWrite{SlicerState: state}, false); e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(o)
	r, e := s.DB.Exec("INSERT INTO slice_tasks(user_id,model_id,state,options,created,updated) VALUES(?,?,'queued',?,1,1)", u.ID, a, string(raw))
	if e != nil {
		t.Fatal(e)
	}
	task, _ := r.LastInsertId()
	if e = s.deleteSliceModel(u, b); e == nil {
		t.Fatal("active secondary source deleted")
	}
	if _, e = s.DB.Exec("UPDATE slice_tasks SET state='succeeded' WHERE id=?", task); e != nil {
		t.Fatal(e)
	}
	if e = s.deleteSliceModel(u, b); e != nil {
		t.Fatal(e)
	}
	content, e := s.slicerStorage(u)
	if e != nil {
		t.Fatal(e)
	}
	data := content.(map[string]any)
	if data["draft"].(*SlicerProject) != nil || data["draftRevision"].(int64) != 2 {
		t.Fatal("stale mixed draft", data)
	}
	var n int
	s.DB.QueryRow("SELECT COUNT(*) FROM slice_tasks WHERE id=?", task).Scan(&n)
	if n != 0 {
		t.Fatal("mixed history retained missing source")
	}
	s.DB.QueryRow("SELECT COUNT(*) FROM model_sources WHERE id=?", a).Scan(&n)
	if n != 1 {
		t.Fatal("unrelated source deleted")
	}
	// A privately owned primary never grants access to somebody else's secondary.
	foreign := storeWorkbenchModel(t, s, u)
	member, e := s.DB.Exec("INSERT INTO users(username,name,password_hash,admin) VALUES('mixed-other','Other','test-only',0)")
	if e != nil {
		t.Fatal(e)
	}
	otherID, _ := member.LastInsertId()
	if _, e = s.DB.Exec("UPDATE model_sources SET user_id=? WHERE id=?", otherID, foreign); e != nil {
		t.Fatal(e)
	}
	for _, bad := range []int64{foreign, -1, 999999} {
		badState := SlicerState{ModelID: a, Options: mixedOptions(a, bad)}
		if _, e = s.saveSlicerProject(u, ProjectWrite{Name: "bad", SlicerState: badState}); e == nil {
			t.Fatal("foreign/invalid secondary accepted", bad)
		}
		if e = s.transaction(func(tx *sql.Tx) error { _, e := loadPlate(tx, u, a, badState.Options); return e }); e == nil {
			t.Fatal("load accepted foreign source", bad)
		}
	}
	if _, e = plateSourceIDs(a, mixedOptions(foreign, foreign)); e == nil {
		t.Fatal("anchor absent from plate")
	}
}
func TestMixedPlateBackupRestoresAllSources(t *testing.T) {
	s, u, _ := setup(t)
	a, b := storeMixedSources(t, s, u)
	state := SlicerState{ModelID: a, Options: mixedOptions(a, b), ActiveCopy: 1}
	state.Options.Advanced = testAdvanced()
	p, e := s.saveSlicerProject(u, ProjectWrite{Name: "mixed backup", SlicerState: state})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.saveSlicerDraft(u, DraftWrite{SlicerState: state}, false); e != nil {
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
	got, e := restored.slicerProject(u, p.ID)
	if e != nil || got.Options.Copies[1].ModelID != b || got.ActiveCopy != 1 || got.Options.Advanced == nil || *got.Options.Advanced != *state.Options.Advanced {
		t.Fatal(got, e)
	}
	if e = restored.transaction(func(tx *sql.Tx) error {
		mesh, e := loadPlate(tx, u, got.ModelID, got.Options)
		if e == nil && mesh.Bounds.Size[2] != 18 {
			t.Fatal("secondary geometry lost")
		}
		return e
	}); e != nil {
		t.Fatal(e)
	}
}
func TestMixedPlateBrokenQueuedSourceFailsOnce(t *testing.T) {
	s, u, _ := setup(t)
	a := storeWorkbenchModel(t, s, u)
	o := mixedOptions(a, 999999)
	raw, _ := json.Marshal(o)
	r, e := s.DB.Exec("INSERT INTO slice_tasks(user_id,model_id,state,options,created,updated) VALUES(?,?,'queued',?,1,1)", u.ID, a, string(raw))
	if e != nil {
		t.Fatal(e)
	}
	id, _ := r.LastInsertId()
	s.runSlice(id)
	s.runSlice(id)
	var state string
	s.DB.QueryRow("SELECT state FROM slice_tasks WHERE id=?", id).Scan(&state)
	if state != "failed" {
		t.Fatal(state)
	}
}
func TestMixedPlateRealStudio(t *testing.T) {
	engine, profiles := os.Getenv("LAYERHUB_TEST_SLICER"), os.Getenv("LAYERHUB_TEST_SLICER_PROFILES")
	if engine == "" || profiles == "" {
		t.Skip("optional real engine")
	}
	s, _, _ := setup(t)
	s.Config.SlicerPath = engine
	s.Config.SlicerProfiles = profiles
	o := mixedOptions(1, 2)
	mesh, e := transformModels(map[int64]ModelMesh{1: workbenchMesh(), 2: mixedMesh()}, 1, o)
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	output, info, e := s.executePlate(ctx, mesh, o)
	if e != nil {
		t.Fatal(e)
	}
	if len(output) == 0 || len(info.Plates) != 1 || info.Plates[0].Seconds <= 0 || info.Plates[0].Filaments[0].UsedGrams <= 0 {
		t.Fatal("missing plate result", info)
	}
	t.Logf("mixed distinct meshes validated: %.2fg, %ds", info.Plates[0].Filaments[0].UsedGrams, info.Plates[0].Seconds)
}
