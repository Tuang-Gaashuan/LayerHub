package app

import (
	"context"
	"math"
	"os"
	"testing"
	"time"
)

func TestPlateCopiesPlacementLimitsAndClearance(t *testing.T) {
	source := workbenchMesh()
	o := defaultSliceSetup()
	o.Copies = []ModelPlacement{{X: 60, Y: 60, Scale: 1}, {X: 100, Y: 60, Scale: 1}}
	m, e := transformModel(source, o)
	if e != nil {
		t.Fatal(e)
	}
	if len(m.Vertices) != len(source.Vertices)*2 || m.Bounds.Size[0] != 60 {
		t.Fatal("copies missing", m.Bounds)
	}
	o.Copies[1].X = 65
	if _, e = transformModel(source, o); e == nil {
		t.Fatal("overlap accepted")
	}
	o.Copies[1].X = 100
	o.Copies[1].Scale = 2
	o.Copies[1].Rotation = Point3{0, 0, 90}
	if _, e = transformModel(source, o); e != nil {
		t.Fatal(e)
	}
	o.Copies[1].X = math.NaN()
	if _, e = transformModel(source, o); e == nil {
		t.Fatal("NaN accepted")
	}
	o.Copies = make([]ModelPlacement, 65)
	if _, e = transformModel(source, o); e == nil {
		t.Fatal("copy limit bypassed")
	}
	o.Copies = []ModelPlacement{{X: 31, Y: 20, Scale: 1}}
	o.Brim = true
	if _, e = transformModel(source, o); e == nil {
		t.Fatal("brim margin missing")
	}
	o = defaultSliceSetup()
	o.Copies = make([]ModelPlacement, 2)
	big := ModelMesh{Vertices: make([]Point3, 80000*3)}
	if _, e = transformModel(big, o); e == nil {
		t.Fatal("aggregate triangle limit bypassed")
	}
}
func TestWorkbenchRealStudioCopies(t *testing.T) {
	engine, profiles := os.Getenv("LAYERHUB_TEST_SLICER"), os.Getenv("LAYERHUB_TEST_SLICER_PROFILES")
	if engine == "" || profiles == "" {
		t.Skip("optional real engine")
	}
	s, _, _ := setup(t)
	s.Config.SlicerPath = engine
	s.Config.SlicerProfiles = profiles
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	raw := encodeModelSTL(workbenchMesh())
	single, one, e := s.executeSlice(ctx, raw, defaultSliceSetup())
	if e != nil {
		t.Fatal(e)
	}
	o := defaultSliceSetup()
	o.Copies = []ModelPlacement{{X: 70, Y: 90, Scale: 1}, {X: 110, Y: 90, Scale: 1}, {X: 150, Y: 90, Scale: 1, Rotation: Point3{0, 0, 45}}}
	multiple, three, e := s.executeSlice(ctx, raw, o)
	if e != nil {
		t.Fatal(e)
	}
	if len(multiple) <= len(single) || three.Plates[0].Filaments[0].UsedGrams < one.Plates[0].Filaments[0].UsedGrams*2.8 {
		t.Fatal("whole-plate consumption missing", one, three)
	}
	t.Logf("single %.2fg; three %.2fg, %ds", one.Plates[0].Filaments[0].UsedGrams, three.Plates[0].Filaments[0].UsedGrams, three.Plates[0].Seconds)
}
