package app

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestMaterialProfilesDefaultsAndProjectValidation(t *testing.T) {
	s, u, _ := setup(t)
	id := storeWorkbenchModel(t, s, u)
	for _, key := range []string{"", "petg-basic", "pla-basic", "petg-hf"} {
		o := defaultSliceSetup()
		o.Material = key
		_, err := s.saveSlicerProject(u, ProjectWrite{Name: "material " + key, SlicerState: SlicerState{ModelID: id, Options: o}})
		if err != nil {
			t.Fatal(key, err)
		}
	}
	o := defaultSliceSetup()
	o.Material = "../../other"
	if _, e := s.saveSlicerProject(u, ProjectWrite{Name: "invalid", SlicerState: SlicerState{ModelID: id, Options: o}}); e == nil {
		t.Fatal("unknown material accepted")
	}
	for _, m := range slicerMaterials[1:] {
		raw, e := s.filamentProfile(m.Key)
		if e != nil {
			t.Fatal(e)
		}
		var v map[string]any
		if json.Unmarshal(raw, &v) != nil || v["filament_id"] != m.ProfileID || v["inherits"] != nil {
			t.Fatal("unresolved profile", m.Key)
		}
	}
}

func TestMaterialProfilesRealStudio(t *testing.T) {
	engine, profiles := os.Getenv("LAYERHUB_TEST_SLICER"), os.Getenv("LAYERHUB_TEST_SLICER_PROFILES")
	if engine == "" || profiles == "" {
		t.Skip("optional real-engine test")
	}
	s, u, d := setup(t)
	s.Config.SlicerPath = engine
	s.Config.SlicerProfiles = profiles
	p, sp := fixture(t, s, u)
	model := storeWorkbenchModel(t, s, u)
	for _, m := range slicerMaterials {
		t.Run(m.Key, func(t *testing.T) {
			o := defaultSliceSetup()
			o.Material = m.Key
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			raw, info, e := s.executeSlice(ctx, encodeModelSTL(workbenchMesh()), o)
			if e != nil {
				t.Fatal(e)
			}
			if len(raw) == 0 || info.Plates[0].Filaments[0].ProfileID != m.ProfileID {
				t.Fatal(info)
			}
			if !m.PrintReady {
				encoded, _ := json.Marshal(info)
				opts, _ := json.Marshal(o)
				r, e := s.DB.Exec("INSERT INTO slice_tasks(user_id,model_id,state,options,created,updated,output,info,sha256) VALUES(?,?,'succeeded',?,1,1,?,?,?)", u.ID, model, string(opts), raw, string(encoded), "offline-test")
				if e != nil {
					t.Fatal(e)
				}
				id, _ := r.LastInsertId()
				if _, e = s.promoteSlice(u, id, p.ID, sp.ID); e == nil {
					t.Fatal("unvalidated profile promoted")
				}
			}
		})
	}
	if d.uploads != 0 {
		t.Fatal("offline material test uploaded to a printer")
	}
}
