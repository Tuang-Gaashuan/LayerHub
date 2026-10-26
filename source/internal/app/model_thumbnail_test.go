package app

import (
	"bytes"
	"encoding/json"
	"image/png"
	"net/http/httptest"
	"testing"
)

func TestWorkbenchThumbnailFallbackAndPrivacy(t *testing.T) {
	s, u, _ := setup(t)
	id := storeWorkbenchModel(t, s, u)
	o := defaultSliceSetup()
	o.Color = "#35A56E"
	o.Rotation = [3]float64{20, 10, 45}
	options, _ := json.Marshal(o)
	result, e := s.DB.Exec("INSERT INTO slice_tasks(user_id,model_id,state,options,output,created,updated) VALUES(?,?,'succeeded',?,?,1,1)", u.ID, id, string(options), []byte("no thumbnail"))
	if e != nil {
		t.Fatal(e)
	}
	task, _ := result.LastInsertId()
	w := httptest.NewRecorder()
	s.sliceFile(w, httptest.NewRequest("GET", "/", nil), u, "preview", task)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	image, e := png.Decode(bytes.NewReader(w.Body.Bytes()))
	if e != nil || image.Bounds().Dx() != 320 {
		t.Fatal("invalid preview", e)
	}
	r, g, _, _ := image.At(160, 160).RGBA()
	if g <= r {
		t.Fatal("model or selected color missing")
	}
	denied := httptest.NewRecorder()
	other := u
	other.ID += 999
	s.sliceFile(denied, httptest.NewRequest("GET", "/", nil), other, "preview", task)
	if denied.Code != 404 {
		t.Fatal("private preview leaked", denied.Code)
	}
	o.Color = ""
	if _, e = modelThumbnail(encodeModelSTL(workbenchMesh()), o); e == nil {
		t.Fatal("invalid color accepted")
	}
}
