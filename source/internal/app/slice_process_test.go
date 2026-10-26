package app

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testAdvanced() *SliceAdvanced {
	return &SliceAdvanced{Walls: 4, TopThickness: 1.2, BottomThickness: 1.2, Pattern: "gyroid", SupportType: "tree", SupportAngle: 40, PlateOnly: true}
}
func TestAdvancedValidationAndPersistence(t *testing.T) {
	s, u, _ := setup(t)
	model := storeWorkbenchModel(t, s, u)
	o := defaultSliceSetup()
	o.Advanced = testAdvanced()
	state := SlicerState{ModelID: model, Options: o}
	p, e := s.saveSlicerProject(u, ProjectWrite{Name: "工艺工程", SlicerState: state})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.saveSlicerDraft(u, DraftWrite{SlicerState: state}, false); e != nil {
		t.Fatal(e)
	}
	for _, edit := range []func(*SliceAdvanced){func(a *SliceAdvanced) { a.Walls = 0 }, func(a *SliceAdvanced) { a.Walls = 7 }, func(a *SliceAdvanced) { a.TopThickness = math.NaN() }, func(a *SliceAdvanced) { a.BottomThickness = 0 }, func(a *SliceAdvanced) { a.Pattern = "unknown" }, func(a *SliceAdvanced) { a.SupportType = "tree(manual)" }, func(a *SliceAdvanced) { a.SupportAngle = 90 }} {
		bad := o
		bad.Advanced = testAdvanced()
		edit(bad.Advanced)
		if _, e = transformModel(workbenchMesh(), bad); e == nil {
			t.Fatal("invalid process accepted")
		}
		if _, e = s.saveSlicerDraft(u, DraftWrite{Revision: 1, SlicerState: SlicerState{ModelID: model, Options: bad}}, false); e == nil {
			t.Fatal("invalid process overwrote draft")
		}
	}
	cfg := s.Config
	s.Close()
	reopened, e := Open(cfg, &fakeDevice{})
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.Close()
	got, e := reopened.slicerProject(u, p.ID)
	if e != nil || got.Options.Advanced == nil || *got.Options.Advanced != *o.Advanced {
		t.Fatal(got, e)
	}
	content, e := reopened.slicerStorage(u)
	if e != nil {
		t.Fatal(e)
	}
	draft := content.(map[string]any)["draft"].(*SlicerProject)
	if draft.Revision != 1 || *draft.Options.Advanced != *o.Advanced {
		t.Fatal("draft changed", draft)
	}
	if advancedProcessSettings(defaultSliceSetup()) != nil {
		t.Fatal("legacy preset rewritten")
	}
}
func TestAdvancedOutputVerificationRejectsIgnoredSettings(t *testing.T) {
	o := defaultSliceSetup()
	o.Advanced = testAdvanced()
	o.Layer = .16
	settings := advancedProcessSettings(o)
	if settings["top_shell_layers"] != "8" {
		t.Fatal("thickness rounded down")
	}
	settings["layer_height"] = "0.16"
	settings["sparse_infill_density"] = "15%"
	settings["enable_support"] = "0"
	path := filepath.Join(t.TempDir(), "process.3mf")
	save := func() {
		var out bytes.Buffer
		z := zip.NewWriter(&out)
		f, _ := z.Create("Metadata/project_settings.config")
		json.NewEncoder(f).Encode(settings)
		z.Close()
		if e := os.WriteFile(path, out.Bytes(), 0600); e != nil {
			t.Fatal(e)
		}
	}
	save()
	if e := verifyAdvancedProcess(path, o); e != nil {
		t.Fatal(e)
	}
	settings["wall_loops"] = "2"
	save()
	if e := verifyAdvancedProcess(path, o); e == nil {
		t.Fatal("engine silently ignored walls")
	}
	delete(settings, "wall_loops")
	save()
	if e := verifyAdvancedProcess(path, o); e == nil {
		t.Fatal("missing process accepted")
	}
}
func testOutputPaths(t *testing.T, raw []byte) *Toolpath {
	t.Helper()
	z, e := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if e != nil {
		t.Fatal(e)
	}
	for _, f := range z.File {
		if f.Name == "Metadata/plate_1.gcode" {
			r, e := f.Open()
			if e != nil {
				t.Fatal(e)
			}
			defer r.Close()
			paths, e := parseToolpath(context.Background(), io.LimitReader(r, 32<<20))
			if e != nil {
				t.Fatal(e)
			}
			return paths
		}
	}
	t.Fatal("no gcode")
	return nil
}
func TestAdvancedRealStudio(t *testing.T) {
	engine, profiles := os.Getenv("LAYERHUB_TEST_SLICER"), os.Getenv("LAYERHUB_TEST_SLICER_PROFILES")
	if engine == "" || profiles == "" {
		t.Skip("optional real engine")
	}
	s, _, _ := setup(t)
	s.Config.SlicerPath = engine
	s.Config.SlicerProfiles = profiles
	var lightWalls int
	var lightGrams float64
	for _, pattern := range []string{"rectilinear", "grid", "gyroid"} {
		o := defaultSliceSetup()
		o.Advanced = testAdvanced()
		o.Advanced.Pattern = pattern
		if pattern == "rectilinear" {
			o.Infill = 10
			o.Advanced.Walls = 2
			o.Advanced.TopThickness = .6
			o.Advanced.BottomThickness = .6
		} else {
			o.Infill = 30
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		raw, info, e := s.executeSlice(ctx, encodeModelSTL(workbenchMesh()), o)
		cancel()
		if e != nil {
			t.Fatal(pattern, e)
		}
		p := testOutputPaths(t, raw)
		walls := p.Layers[24].Counts[1]
		grams := info.Plates[0].Filaments[0].UsedGrams
		if pattern == "rectilinear" {
			lightWalls = walls
			lightGrams = grams
		} else if walls <= lightWalls || grams <= lightGrams {
			t.Fatal("strength options did not change paths/material", pattern, walls, lightWalls, grams, lightGrams)
		}
		t.Logf("%s: walls at Z5=%d, used %.2fg", pattern, walls, grams)
	}
	for _, kind := range []string{"normal", "tree"} {
		o := defaultSliceSetup()
		o.Support = true
		o.Rotation = Point3{35, 0, 0}
		o.Advanced = testAdvanced()
		o.Advanced.SupportType = kind
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		raw, _, e := s.executeSlice(ctx, encodeModelSTL(workbenchMesh()), o)
		cancel()
		if e != nil {
			t.Fatal(kind, e)
		}
		paths := testOutputPaths(t, raw)
		n := 0
		for _, l := range paths.Layers {
			n += l.Counts[4]
		}
		if n == 0 {
			t.Fatal("no real support generated", kind)
		}
		t.Logf("%s support: %d actual support segments", kind, n)
	}
}
