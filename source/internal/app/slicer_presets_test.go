package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func presetFixture() PresetWrite {
	return PresetWrite{ID: randomID(16), Name: "PETG 支架", Settings: ProcessSettings{Material: "petg-basic", Layer: .2, Infill: 20, Support: true, Advanced: testAdvanced()}}
}

func TestProcessPresetsOwnershipRevisionsAndRecoverableArchive(t *testing.T) {
	s, u, _ := setup(t)
	in := presetFixture()
	p, e := s.saveProcessPreset(u, in)
	if e != nil {
		t.Fatal(e)
	}
	if p.Revision != 1 {
		t.Fatal(p)
	}
	// A replay of a new save after a lost response returns the same record.
	replay, e := s.saveProcessPreset(u, in)
	if e != nil || !reflect.DeepEqual(replay, p) {
		t.Fatal(replay, e)
	}
	if e = s.SaveUser(u, "preset_other", "另一成员", "OtherPassword123", false); e != nil {
		t.Fatal(e)
	}
	var other User
	s.DB.QueryRow("SELECT id,username,name,admin FROM users WHERE username='preset_other'").Scan(&other.ID, &other.Username, &other.Name, &other.Admin)
	list, e := s.processPresets(other)
	if e != nil || len(list) != 0 {
		t.Fatal("private preset leaked", list, e)
	}
	foreign := in
	foreign.Revision = 1
	if _, e = s.saveProcessPreset(other, foreign); e == nil {
		t.Fatal("foreign update accepted")
	}
	if e = s.archiveProcessPreset(other, PresetArchive{ID: p.ID, Revision: 1, Archived: true}); e == nil {
		t.Fatal("foreign archive accepted")
	}
	// Concurrent edits starting at the same revision cannot overwrite each other.
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, name := range []string{"支架新参数 A", "支架新参数 B"} {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			v := in
			v.Revision = 1
			v.Name = name
			_, e := s.saveProcessPreset(u, v)
			results <- e
		}(name)
	}
	wg.Wait()
	close(results)
	n := 0
	for err := range results {
		if err == nil {
			n++
		}
	}
	if n != 1 {
		t.Fatal("concurrent overwrite", n)
	}
	list, _ = s.processPresets(u)
	p = list[0]
	if e = s.archiveProcessPreset(u, PresetArchive{ID: p.ID, Revision: 1, Archived: true}); e == nil {
		t.Fatal("stale archive accepted")
	}
	if e = s.archiveProcessPreset(u, PresetArchive{ID: p.ID, Revision: 2, Archived: true}); e != nil {
		t.Fatal(e)
	}
	list, _ = s.processPresets(u)
	if !list[0].Archived || list[0].Revision != 3 {
		t.Fatal(list)
	}
	if e = s.archiveProcessPreset(u, PresetArchive{ID: p.ID, Revision: 3, Archived: false}); e != nil {
		t.Fatal(e)
	}
	list, _ = s.processPresets(u)
	if list[0].Archived || list[0].Revision != 4 {
		t.Fatal(list)
	}
	var jobs, commands int
	s.DB.QueryRow("SELECT COUNT(*) FROM jobs").Scan(&jobs)
	s.DB.QueryRow("SELECT COUNT(*) FROM printer_commands").Scan(&commands)
	if jobs != 0 || commands != 0 {
		t.Fatal("preset wrote physical work")
	}
}

func TestProcessPresetsValidationAndNameConflicts(t *testing.T) {
	s, u, _ := setup(t)
	in := presetFixture()
	if _, e := s.saveProcessPreset(u, in); e != nil {
		t.Fatal(e)
	}
	for _, edit := range []func(*PresetWrite){
		func(v *PresetWrite) { v.Name = "  " }, func(v *PresetWrite) { v.Name = "bad\nname" }, func(v *PresetWrite) { v.Name = strings.Repeat("字", 41) }, func(v *PresetWrite) { v.ID = "../bad" }, func(v *PresetWrite) { v.Settings.Material = "custom-gcode" }, func(v *PresetWrite) { v.Settings.Layer = .3 }, func(v *PresetWrite) { v.Settings.Infill = 99 }, func(v *PresetWrite) { v.Settings.Advanced.Walls = 0 }, func(v *PresetWrite) { v.Revision = 8 },
	} {
		v := presetFixture()
		edit(&v)
		if _, e := s.saveProcessPreset(u, v); e == nil {
			t.Fatal("invalid recipe accepted", v)
		}
	}
	duplicate := presetFixture()
	duplicate.Name = in.Name
	if _, e := s.saveProcessPreset(u, duplicate); e == nil {
		t.Fatal("duplicate active name")
	}
	p, _ := s.processPresets(u)
	if e := s.archiveProcessPreset(u, PresetArchive{ID: p[0].ID, Revision: 1, Archived: true}); e != nil {
		t.Fatal(e)
	}
	if _, e := s.saveProcessPreset(u, duplicate); e != nil {
		t.Fatal(e)
	}
	if e := s.archiveProcessPreset(u, PresetArchive{ID: p[0].ID, Revision: 2, Archived: false}); e == nil {
		t.Fatal("restore clobbered same-name recipe")
	}
	list, _ := s.processPresets(u)
	if len(list) != 2 {
		t.Fatal(list)
	}
}

func TestProcessPresetHTTPRejectsGeometryAndUntrustedSettings(t *testing.T) {
	s, u, _ := setup(t)
	token := randomID(32)
	s.DB.Exec("INSERT INTO sessions(hash,user_id,expires) VALUES(?,?,?)", tokenHash(token), u.ID, time.Now().Add(time.Hour).Unix())
	in := presetFixture()
	raw, _ := json.Marshal(in)
	var payload map[string]any
	json.Unmarshal(raw, &payload)
	for _, field := range []string{"x", "copies", "filament_start_gcode", "code"} {
		payload["settings"].(map[string]any)[field] = "not allowed"
		body, _ := json.Marshal(payload)
		r := httptest.NewRequest("POST", "/api/slicing/presets", strings.NewReader(string(body)))
		r.AddCookie(&http.Cookie{Name: "layerhub_sid", Value: token})
		r.Header.Set("X-CSRF-Token", s.csrf(token))
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != 400 {
			t.Fatal(field, w.Code, w.Body.String())
		}
		delete(payload["settings"].(map[string]any), field)
	}
	r := httptest.NewRequest("POST", "/api/slicing/presets", strings.NewReader(string(raw)))
	r.AddCookie(&http.Cookie{Name: "layerhub_sid", Value: token})
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("missing csrf", w.Code)
	}
	list, _ := s.processPresets(u)
	if len(list) != 0 {
		t.Fatal(list)
	}
}

func TestProcessPresetsBackupAndRestart(t *testing.T) {
	s, u, _ := setup(t)
	in := presetFixture()
	p, e := s.saveProcessPreset(u, in)
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
	in.Revision = 1
	in.Name = "备份后修改"
	if _, e = s.saveProcessPreset(u, in); e != nil {
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
	got, e := restored.processPresets(u)
	if e != nil || len(got) != 1 || !reflect.DeepEqual(got[0], p) {
		t.Fatal(got, e)
	}
}

func TestProcessPresetRealStudio(t *testing.T) {
	engine, profiles := os.Getenv("LAYERHUB_TEST_SLICER"), os.Getenv("LAYERHUB_TEST_SLICER_PROFILES")
	if engine == "" || profiles == "" {
		t.Skip("optional real engine test")
	}
	s, u, d := setup(t)
	s.Config.SlicerPath = engine
	s.Config.SlicerProfiles = profiles
	in := presetFixture()
	p, e := s.saveProcessPreset(u, in)
	if e != nil {
		t.Fatal(e)
	}
	o := defaultSliceSetup()
	o.Material = p.Settings.Material
	o.Layer = p.Settings.Layer
	o.Infill = p.Settings.Infill
	o.Support = p.Settings.Support
	o.Brim = p.Settings.Brim
	o.Advanced = p.Settings.Advanced
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	raw, info, e := s.executeSlice(ctx, encodeModelSTL(workbenchMesh()), o)
	if e != nil || len(raw) == 0 || info.Plates[0].Filaments[0].ProfileID != "GFG00" {
		t.Fatal(info, e)
	}
	if d.uploads != 0 {
		t.Fatal("recipe slice uploaded to a printer")
	}
}

func TestProcessPresetQuotaIncludesArchivedButAllowsExistingUpdates(t *testing.T) {
	s, u, _ := setup(t)
	in := presetFixture()
	p, e := s.saveProcessPreset(u, in)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(p.Settings)
	for n := 1; n < 100; n++ {
		if _, e = s.DB.Exec("INSERT INTO slicer_process_presets(id,user_id,name,settings,revision,updated,archived) VALUES(?,?,?,?,1,1,1)", randomID(16), u.ID, fmt.Sprint("old-", n), string(raw)); e != nil {
			t.Fatal(e)
		}
	}
	if _, e = s.saveProcessPreset(u, presetFixture()); e == nil {
		t.Fatal("quota bypass")
	}
	in.Revision = 1
	in.Name = "复用现有预设"
	if _, e = s.saveProcessPreset(u, in); e != nil {
		t.Fatal(e)
	}
}
