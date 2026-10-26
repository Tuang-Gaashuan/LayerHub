package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
	"time"
)

func storedComparisonTask(t *testing.T, s *Service, u User, model int64, o SliceSetup, seconds int, grams float64) int64 {
	t.Helper()
	opts, _ := json.Marshal(o)
	info, _ := json.Marshal(SliceInfo{Model: "Bambu Lab X2D", Plates: []SlicePlate{{ID: 1, Seconds: seconds, Filaments: []SliceFilament{{Material: "PETG", Color: o.Color, UsedGrams: grams, Grams: 25, ProfileID: "GFG00"}}}}})
	r, e := s.DB.Exec("INSERT INTO slice_tasks(user_id,model_id,state,options,info,created,updated) VALUES(?,?,'succeeded',?,?,1,1)", u.ID, model, string(opts), string(info))
	if e != nil {
		t.Fatal(e)
	}
	id, _ := r.LastInsertId()
	return id
}
func TestSliceComparisonUsesGeometryNotNamesOrBounds(t *testing.T) {
	s, u, d := setup(t)
	model := storeWorkbenchModel(t, s, u)
	base := defaultSliceSetup()
	a := storedComparisonTask(t, s, u, model, base, 1200, 24.16)
	strong := base
	strong.Infill = 30
	strong.Advanced = testAdvanced()
	b := storedComparisonTask(t, s, u, model, strong, 1500, 30.12)
	got, e := s.compareSlices(context.Background(), u, []int64{a, b})
	if e != nil || !got.SameGeometry || len(got.Entries) != 2 || got.Entries[0].Grams != 24.16 || got.Entries[1].Seconds != 1500 {
		t.Fatal(got, e)
	}
	moved := base
	moved.X += 5
	c := storedComparisonTask(t, s, u, model, moved, 1500, 25)
	got, e = s.compareSlices(context.Background(), u, []int64{a, c})
	if e != nil || got.SameGeometry {
		t.Fatal("translation ignored", got, e)
	}
	// Equal outer dimensions cannot conceal a change to an interior vertex.
	original := workbenchMesh()
	changed := original
	changed.Vertices = append([]Point3{}, original.Vertices...)
	changed.Vertices[0][0] += .3
	if comparisonGeometry(original) == comparisonGeometry(changed) {
		t.Fatal("different mesh with same bounds matched")
	}
	// Ordering independent copies does not change the submitted plate geometry.
	multi := base
	multi.Copies = []ModelPlacement{{ModelID: model, X: 100, Y: 128, Scale: 1}, {ModelID: model, X: 160, Y: 128, Scale: 1}}
	p := storedComparisonTask(t, s, u, model, multi, 1600, 48.32)
	reversed := multi
	reversed.Copies = []ModelPlacement{multi.Copies[1], multi.Copies[0]}
	q := storedComparisonTask(t, s, u, model, reversed, 1700, 49)
	got, e = s.compareSlices(context.Background(), u, []int64{p, q})
	if e != nil || !got.SameGeometry || got.Entries[0].Objects != 2 {
		t.Fatal(got, e)
	}
	var jobs, commands int
	s.DB.QueryRow("SELECT COUNT(*) FROM jobs").Scan(&jobs)
	s.DB.QueryRow("SELECT COUNT(*) FROM printer_commands").Scan(&commands)
	if jobs != 0 || commands != 0 || d.uploads != 0 {
		t.Fatal("comparison changed physical work")
	}
}

func TestSliceComparisonPrivacyBoundsAndMissingMetadata(t *testing.T) {
	s, u, _ := setup(t)
	model := storeWorkbenchModel(t, s, u)
	o := defaultSliceSetup()
	a := storedComparisonTask(t, s, u, model, o, 1200, 24.16)
	b := storedComparisonTask(t, s, u, model, o, 1300, 25)
	for _, ids := range [][]int64{{a}, {a, a}, {a, b, 999, 1000}, {a, 999}, {0, b}} {
		if _, e := s.compareSlices(context.Background(), u, ids); e == nil {
			t.Fatal("bad selection", ids)
		}
	}
	if _, e := s.compareSlices(context.Background(), User{ID: u.ID + 20, Admin: true}, []int64{a, b}); e == nil {
		t.Fatal("other account read private results")
	}
	for _, state := range []string{"queued", "failed", "cancelled", "running"} {
		s.DB.Exec("UPDATE slice_tasks SET state=? WHERE id=?", state, b)
		if _, e := s.compareSlices(context.Background(), u, []int64{a, b}); e == nil {
			t.Fatal("compared incomplete slice", state)
		}
	}
	s.DB.Exec("UPDATE slice_tasks SET state='succeeded',info='null' WHERE id=?", b)
	if _, e := s.compareSlices(context.Background(), u, []int64{a, b}); e == nil {
		t.Fatal("missing stats accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := s.compareSlices(ctx, u, []int64{a, b}); e == nil {
		t.Fatal("cancel ignored")
	}
	for _, raw := range []string{"1", "1,1", "1,2,3,4", "-1,2", "x,2", "1,2,"} {
		if _, e := comparisonIDs(raw); e == nil {
			t.Fatal(raw)
		}
	}
	ids, e := comparisonIDs("2,1,3")
	if e != nil || !reflect.DeepEqual(ids, []int64{2, 1, 3}) {
		t.Fatal(ids, e)
	}
}

func TestSliceComparisonHTTPReadOnlyAndUnknownEstimates(t *testing.T) {
	s, u, _ := setup(t)
	model := storeWorkbenchModel(t, s, u)
	o := defaultSliceSetup()
	a := storedComparisonTask(t, s, u, model, o, 0, 0)
	b := storedComparisonTask(t, s, u, model, o, 1300, 25)
	token := randomID(32)
	s.DB.Exec("INSERT INTO sessions(hash,user_id,expires) VALUES(?,?,?)", tokenHash(token), u.ID, time.Now().Add(time.Hour).Unix())
	req := httptest.NewRequest("GET", "/api/slicing/compare?ids=1,2", nil)
	req.AddCookie(&http.Cookie{Name: "layerhub_sid", Value: token})
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	var got struct {
		OK bool `json:"ok"`
		SliceComparison
	}
	json.Unmarshal(w.Body.Bytes(), &got)
	if w.Code != 200 || !got.OK || len(got.Entries) != 2 || got.Entries[0].ExactUsage || got.Entries[0].Seconds != 0 {
		t.Fatal(w.Code, w.Body.String())
	}
	if got.Entries[0].Task.ID != a || got.Entries[1].Task.ID != b {
		t.Fatal(got)
	}
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/slicing/compare?ids=1,2", nil))
	if w.Code != 401 {
		t.Fatal(w.Code)
	}
}

func TestSliceComparisonRealStudio(t *testing.T) {
	engine, profiles := os.Getenv("LAYERHUB_TEST_SLICER"), os.Getenv("LAYERHUB_TEST_SLICER_PROFILES")
	if engine == "" || profiles == "" {
		t.Skip("optional real engine")
	}
	s, u, d := setup(t)
	s.Config.SlicerPath = engine
	s.Config.SlicerProfiles = profiles
	model := storeWorkbenchModel(t, s, u)
	ids := []int64{}
	for _, walls := range []int{2, 4} {
		o := defaultSliceSetup()
		o.Advanced = testAdvanced()
		o.Advanced.Walls = walls
		o.Infill = 10
		if walls == 4 {
			o.Infill = 30
		}
		id, e := s.queueSlice(u, model, o)
		if e != nil {
			t.Fatal(e)
		}
		s.runSlice(id)
		ids = append(ids, id)
	}
	got, e := s.compareSlices(context.Background(), u, ids)
	if e != nil || !got.SameGeometry {
		t.Fatal(got, e)
	}
	if got.Entries[1].Grams <= got.Entries[0].Grams || !got.Entries[0].ExactUsage || got.Entries[0].Seconds <= 0 {
		t.Fatal("real estimates not retained", got)
	}
	if d.uploads != 0 {
		t.Fatal("comparison uploaded to printer")
	}
}
