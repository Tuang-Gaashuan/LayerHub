package app

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
	"time"
)

func multiPlateState(a, b int64) SlicerState {
	one, two := defaultSliceSetup(), defaultSliceSetup()
	one.Copies = []ModelPlacement{{ModelID: a, X: 100, Y: 120, Scale: 1}}
	two.Copies = []ModelPlacement{{ModelID: b, X: 140, Y: 120, Scale: 1}}
	two.Layer = .16
	two.Infill = 30
	two.Advanced = testAdvanced()
	return SlicerState{Plates: []SlicerPlate{{ID: "first", Name: "外壳", ModelID: a, Options: one}, {ID: "second", Name: "底座", ModelID: b, Options: two}, {ID: "empty", Name: "备用空盘", Options: defaultSliceSetup()}}, ActivePlate: 2}
}
func TestMultiPlatePersistenceReferencesAndDowngrade(t *testing.T) {
	s, u, _ := setup(t)
	a, b := storeMixedSources(t, s, u)
	state := multiPlateState(a, b)
	p, e := s.saveSlicerProject(u, ProjectWrite{Name: "多盘", SlicerState: state})
	if e != nil {
		t.Fatal(e)
	}
	if p.ModelID != a || len(p.Plates) != 3 || p.ActivePlate != 2 {
		t.Fatal(p)
	}
	got, e := s.slicerProject(u, p.ID)
	if e != nil || !reflect.DeepEqual(got.Plates, p.Plates) || got.ActivePlate != 2 {
		t.Fatal(got, e)
	}
	if e = s.deleteSliceModel(u, b); e == nil {
		t.Fatal("non-active board source deleted")
	}
	if _, e = s.saveSlicerDraft(u, DraftWrite{SlicerState: state}, false); e != nil {
		t.Fatal(e)
	}
	legacy := SlicerState{ModelID: a, Options: defaultSliceSetup()}
	if _, e = s.saveSlicerProject(u, ProjectWrite{ID: p.ID, Revision: p.Revision, Name: p.Name, SlicerState: legacy}); e == nil {
		t.Fatal("old cached client discarded boards")
	}
	if _, e = s.saveSlicerDraft(u, DraftWrite{Revision: 1, SlicerState: legacy}, false); e == nil {
		t.Fatal("old client discarded multi-board draft")
	}
	if _, e = s.saveSlicerProject(u, ProjectWrite{Name: "legacy separate", SlicerState: legacy}); e != nil {
		t.Fatal(e)
	}
	if _, e = s.saveSlicerDraft(u, DraftWrite{Revision: 1}, true); e != nil {
		t.Fatal(e)
	}
	if _, e = s.saveSlicerDraft(u, DraftWrite{Revision: 2, SlicerState: legacy}, false); e != nil {
		t.Fatal(e)
	}
}
func TestMultiPlateValidationAndResultOwnership(t *testing.T) {
	s, u, _ := setup(t)
	a, b := storeMixedSources(t, s, u)
	cases := map[string]func(*SlicerState){
		"duplicate ID":                  func(p *SlicerState) { p.Plates[1].ID = p.Plates[0].ID },
		"empty ID":                      func(p *SlicerState) { p.Plates[1].ID = "" },
		"unknown source in other board": func(p *SlicerState) { p.Plates[1].ModelID = 999; p.Plates[1].Options.Copies[0].ModelID = 999 },
		"invalid selected board":        func(p *SlicerState) { p.ActivePlate = 3 },
		"empty board hidden objects": func(p *SlicerState) {
			p.Plates[2].Options.Copies = []ModelPlacement{{ModelID: a, X: 90, Y: 90, Scale: 1}}
		},
		"empty board result":          func(p *SlicerState) { p.Plates[2].TaskID = 1 },
		"bad advanced on other board": func(p *SlicerState) { p.Plates[1].Options.Advanced.Walls = 0 },
		"nonexistent result":          func(p *SlicerState) { p.Plates[1].TaskID = 999 },
		"all boards empty":            func(p *SlicerState) { p.Plates = p.Plates[2:]; p.ActivePlate = 0 },
		"nine boards": func(p *SlicerState) {
			for len(p.Plates) < 9 {
				p.Plates = append(p.Plates, SlicerPlate{ID: fmt.Sprint(len(p.Plates)), Name: "盘", Options: defaultSliceSetup()})
			}
		},
		"project object quota": func(p *SlicerState) {
			p.Plates = nil
			for i := 0; i < 5; i++ {
				o := defaultSliceSetup()
				for j := 0; j < 64; j++ {
					o.Copies = append(o.Copies, ModelPlacement{ModelID: a, X: 90, Y: 90, Scale: 1})
				}
				p.Plates = append(p.Plates, SlicerPlate{ID: fmt.Sprint(i), Name: "盘", ModelID: a, Options: o})
			}
			p.ActivePlate = 0
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			state := multiPlateState(a, b)
			mutate(&state)
			if _, e := s.saveSlicerProject(u, ProjectWrite{Name: name, SlicerState: state}); e == nil {
				t.Fatal("invalid multi-board state saved")
			}
		})
	}
	state := multiPlateState(a, b)
	raw, _ := json.Marshal(state.Plates[1].Options)
	r, e := s.DB.Exec("INSERT INTO slice_tasks(user_id,model_id,state,options,created,updated) VALUES(?,?,'queued',?,1,1)", u.ID, b, string(raw))
	if e != nil {
		t.Fatal(e)
	}
	task, _ := r.LastInsertId()
	state.Plates[1].TaskID = task
	p, e := s.saveSlicerProject(u, ProjectWrite{Name: "matching", SlicerState: state})
	if e != nil {
		t.Fatal(e)
	}
	state.Plates[1].Options.Infill = 25
	if _, e = s.saveSlicerProject(u, ProjectWrite{Name: "mismatched", SlicerState: state}); e == nil {
		t.Fatal("stale task accepted")
	}
	other := u
	other.ID += 999
	if _, e = s.slicerTask(other, task); e == nil {
		t.Fatal("private task leaked")
	}
	got, e := s.slicerTask(u, task)
	if e != nil || got.ModelID != b {
		t.Fatal(got, e)
	}
	if _, e = s.saveSlicerProject(u, ProjectWrite{ID: p.ID, Revision: 0, Name: p.Name, SlicerState: p.SlicerState}); e == nil {
		t.Fatal("CAS bypass")
	}
}
func TestMultiPlateHTTPBudgetAndBackup(t *testing.T) {
	s, u, _ := setup(t)
	a, b := storeMixedSources(t, s, u)
	state := multiPlateState(a, b)
	// The maximum 256 objects and 8 boards must fit the existing 64 KiB JSON limit.
	budget := multiPlateState(a, b)
	budget.Plates = nil
	budget.ActivePlate = 7
	for i := 0; i < 8; i++ {
		o := defaultSliceSetup()
		o.Advanced = testAdvanced()
		for j := 0; j < 32; j++ {
			o.Copies = append(o.Copies, ModelPlacement{ModelID: a, X: 100.123456789, Y: 110.123456789, Scale: 1.123456789, Rotation: Point3{12.123456789, 23.123456789, 34.123456789}})
		}
		budget.Plates = append(budget.Plates, SlicerPlate{ID: fmt.Sprintf("plate-%d", i), Name: "测试盘", ModelID: a, Options: o})
	}
	if e := normalizeSlicerState(&budget); e != nil {
		t.Fatal(e)
	}
	body, _ := json.Marshal(ProjectWrite{Name: "请求长度", SlicerState: budget})
	var in ProjectWrite
	if e := decode(httptest.NewRequest("POST", "/", bytes.NewReader(body)), &in); e != nil {
		t.Fatalf("maximum project exceeds request budget (%d bytes): %v", len(body), e)
	}
	p, e := s.saveSlicerProject(u, ProjectWrite{Name: "恢复全部盘", SlicerState: state})
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
	if e != nil || !reflect.DeepEqual(got.Plates, p.Plates) || got.ActivePlate != 2 {
		t.Fatal(got, e)
	}
	content, e := restored.slicerStorage(u)
	if e != nil {
		t.Fatal(e)
	}
	draft := content.(map[string]any)["draft"].(*SlicerProject)
	if !reflect.DeepEqual(draft.Plates, p.Plates) {
		t.Fatal("draft boards lost")
	}
	for _, plate := range got.Plates {
		if plate.ModelID == 0 {
			continue
		}
		if e = restored.transaction(func(tx *sql.Tx) error { _, err := loadPlate(tx, u, plate.ModelID, plate.Options); return err }); e != nil {
			t.Fatal(e)
		}
	}
}
func TestMultiPlateRealStudio(t *testing.T) {
	engine, profiles := os.Getenv("LAYERHUB_TEST_SLICER"), os.Getenv("LAYERHUB_TEST_SLICER_PROFILES")
	if engine == "" || profiles == "" {
		t.Skip("optional real engine")
	}
	s, u, _ := setup(t)
	s.Config.SlicerPath = engine
	s.Config.SlicerProfiles = profiles
	a, b := storeMixedSources(t, s, u)
	state := multiPlateState(a, b)
	p, e := s.saveSlicerProject(u, ProjectWrite{Name: "两盘独立切片", SlicerState: state})
	if e != nil {
		t.Fatal(e)
	}
	for i, plate := range p.Plates {
		if plate.ModelID == 0 {
			continue
		}
		var mesh ModelMesh
		if e = s.transaction(func(tx *sql.Tx) error {
			var e error
			mesh, e = loadPlate(tx, u, plate.ModelID, plate.Options)
			return e
		}); e != nil {
			t.Fatal(e)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		output, info, e := s.executePlate(ctx, mesh, plate.Options)
		cancel()
		if e != nil {
			t.Fatal(e)
		}
		path := testOutputPaths(t, output)
		if len(path.Layers) == 0 || len(info.Plates) != 1 {
			t.Fatal("missing board output")
		}
		t.Logf("board=%d geometry=%v layer=%.2f layers=%d grams=%.2f", i+1, mesh.Bounds.Size, plate.Options.Layer, len(path.Layers), info.Plates[0].Filaments[0].UsedGrams)
	}
}
