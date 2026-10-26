package app

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestToolpathModesRetractionArcAndEndBoundary(t *testing.T) {
	gcode := `G90
M83
G1 X0 Y0 Z0
; CHANGE_LAYER
; Z_HEIGHT: 0.2
G1 Z.2
; FEATURE: Outer wall
G1X10Y0E1
G1 E-1
G1 X11
M82
G92 E0
; FEATURE: Sparse infill
G1 Y10 E1
G1 X12 E.5
G91
G1 X1 E1.5
G90
G92 X0
G1 X1 E2
; CHANGE_LAYER
; Z_HEIGHT: 0.4
G1 Z.4
; FEATURE: Support interface
G3 X1 Y10 I-.5 J0 Z.6 E3
; MACHINE_END_GCODE_START
G1 X200 E100
`
	p, e := parseToolpath(context.Background(), strings.NewReader(gcode))
	if e != nil {
		t.Fatal(e)
	}
	if len(p.Layers) != 2 || p.Layers[0].Z != .2 || p.Layers[1].Z != .4 {
		t.Fatal(p)
	}
	l := p.Layers[0]
	if l.Counts[1] != 1 || l.Counts[2] != 3 || l.Counts[0] != 3 {
		t.Fatal("wrong travel/extrusion", l.Counts)
	}
	last := l.Segments[len(l.Segments)-1]
	if last[3] != 14 {
		t.Fatal("G92 world offset lost", last)
	}
	// A full XY helix, not a straight line, with the final Z and endpoint preserved.
	arc := p.Layers[1]
	if arc.Counts[4] < 70 {
		t.Fatal("arc not tessellated", arc.Counts)
	}
	final := arc.Segments[len(arc.Segments)-1]
	if final[3] != 14 || final[5] != .6 {
		t.Fatal(final)
	}
}
func TestToolpathRefusesUnsupportedOrInvalidGeometry(t *testing.T) {
	for _, movement := range []string{"G1 Xnan Y1 E1", "G1 X10001 E1", "G2 X1 Y1 R4 E1", "G18\nG3 X1 I1 E1", "G3 I0 J0 E1"} {
		_, e := parseToolpath(context.Background(), strings.NewReader("G90\nM83\nG1 X0 Y0 Z0\n; CHANGE_LAYER\n; Z_HEIGHT: 0.2\n"+movement))
		if e == nil {
			t.Fatal("invalid movement accepted", movement)
		}
	}
}
func zipPath(t *testing.T, gcode string) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	w, e := z.Create("Metadata/plate_1.gcode")
	if e != nil {
		t.Fatal(e)
	}
	w.Write([]byte(gcode))
	if e = z.Close(); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func TestToolpathCacheAlwaysChecksOwnerAndDeletion(t *testing.T) {
	s, u, _ := setup(t)
	m := storeWorkbenchModel(t, s, u)
	raw := zipPath(t, "G90\nM83\nG1 X0 Y0 Z0\n; CHANGE_LAYER\n; Z_HEIGHT: 0.2\n; FEATURE: Inner wall\nG1 X20 E1\n")
	result, e := s.DB.Exec("INSERT INTO slice_tasks(user_id,model_id,state,options,output,created,updated) VALUES(?,?,'succeeded','{}',?,1,1)", u.ID, m, raw)
	if e != nil {
		t.Fatal(e)
	}
	id, _ := result.LastInsertId()
	text := strconv.FormatInt(id, 10)
	meta, e := s.toolpathRequest(context.Background(), u, text, "")
	if e != nil {
		t.Fatal(e)
	}
	encoded, _ := json.Marshal(meta)
	if strings.Contains(string(encoded), "[0,0,0,") {
		t.Fatal("metadata includes segments")
	}
	if _, e = s.toolpathRequest(context.Background(), u, text, "1"); e != nil {
		t.Fatal(e)
	}
	other := u
	other.ID += 100
	if _, e = s.toolpathRequest(context.Background(), other, text, "1"); e == nil {
		t.Fatal("cached geometry leaked")
	}
	if _, e = s.toolpathRequest(context.Background(), u, text, "0"); e == nil {
		t.Fatal("bad layer accepted")
	}
	s.DB.Exec("DELETE FROM slice_tasks WHERE id=?", id)
	if _, e = s.toolpathRequest(context.Background(), u, text, "1"); e == nil {
		t.Fatal("deleted geometry leaked")
	}
}
func TestToolpathPinnedRealSlice(t *testing.T) {
	file := filepath.Join("..", "..", "qa", "slicing-poc", "x2d-white-petg-cube-verified-20261023", "plate_1.gcode")
	raw, e := os.ReadFile(file)
	if e != nil {
		t.Skip("local pinned engine artifact not packaged")
	}
	p, e := parseToolpath(context.Background(), bytes.NewReader(raw))
	if e != nil {
		t.Fatal(e)
	}
	if len(p.Layers) != 50 || p.Total < 1000 {
		t.Fatal("real layers missing", len(p.Layers), p.Total)
	}
	t.Logf("layers %d, segments %d", len(p.Layers), p.Total)
}
