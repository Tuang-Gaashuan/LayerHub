package app

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestSlicerProjectLimitAndBackupRoundtrip(t *testing.T) {
	s, u, _ := setup(t)
	model := storeWorkbenchModel(t, s, u)
	state := SlicerState{ModelID: model, Options: defaultSliceSetup()}
	var first SlicerProject
	for i := 0; i < 20; i++ {
		p, e := s.saveSlicerProject(u, ProjectWrite{Name: fmt.Sprintf("工程 %d", i), SlicerState: state})
		if e != nil {
			t.Fatal(e)
		}
		if i == 0 {
			first = p
		}
	}
	if _, e := s.saveSlicerProject(u, ProjectWrite{Name: "超限", SlicerState: state}); e == nil {
		t.Fatal("project quota bypassed")
	}
	if _, e := s.saveSlicerDraft(u, DraftWrite{SlicerState: state}, false); e != nil {
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
	p, e := restored.slicerProject(u, first.ID)
	if e != nil || p.Name != first.Name || p.ModelID != model {
		t.Fatal("saved project missing after backup restore", p, e)
	}
	content, e := restored.slicerStorage(u)
	if e != nil {
		t.Fatal(e)
	}
	if content.(map[string]any)["draft"].(*SlicerProject).ModelID != model {
		t.Fatal("draft missing from backup")
	}
}

func TestSlicerProjectsDraftsIsolationConflictAndPersistence(t *testing.T) {
	s, u, driver := setup(t)
	model := storeWorkbenchModel(t, s, u)
	state := SlicerState{ModelID: model, Options: defaultSliceSetup(), ActiveCopy: 1}
	state.Options.Copies = []ModelPlacement{{X: 60, Y: 60, Scale: 1}, {X: 60, Y: 60, Scale: 1}}
	// Saving unfinished, overlapping placement is allowed; slicing must still reject it.
	p, e := s.saveSlicerProject(u, ProjectWrite{Name: "重复打印工程", SlicerState: state})
	if e != nil {
		t.Fatal(e)
	}
	if p.Revision != 1 || p.ActiveCopy != 1 || len(p.Options.Copies) != 2 {
		t.Fatal(p)
	}
	if _, e = s.queueSlice(u, model, state.Options); e == nil {
		t.Fatal("saved project bypassed slicing checks")
	}
	rev, e := s.saveSlicerDraft(u, DraftWrite{SlicerState: state}, false)
	if e != nil || rev != 1 {
		t.Fatal(rev, e)
	}
	if _, e = s.saveSlicerDraft(u, DraftWrite{SlicerState: state}, false); e == nil {
		t.Fatal("stale draft overwrite")
	}
	if _, e = s.saveSlicerProject(u, ProjectWrite{Name: p.Name, SlicerState: state}); e == nil {
		t.Fatal("duplicate name")
	}
	other := u
	other.ID += 999
	if _, e = s.slicerProject(other, p.ID); e == nil {
		t.Fatal("private project leaked")
	}
	if _, e = s.saveSlicerProject(other, ProjectWrite{Name: "foreign model", SlicerState: state}); e == nil {
		t.Fatal("foreign model accepted")
	}
	if e = s.deleteSlicerProject(other, p.ID, p.Revision); e == nil {
		t.Fatal("foreign delete accepted")
	}
	p2, e := s.saveSlicerProject(u, ProjectWrite{ID: p.ID, Revision: p.Revision, Name: "新名称", SlicerState: state})
	if e != nil || p2.Revision != 2 {
		t.Fatal(p2, e)
	}
	if _, e = s.saveSlicerProject(u, ProjectWrite{ID: p.ID, Revision: 1, Name: "旧页面", SlicerState: state}); e == nil {
		t.Fatal("stale project overwrite")
	}
	if e = s.deleteSliceModel(u, model); e == nil || !strings.Contains(e.Error(), "工程") {
		t.Fatal("project source was not retained", e)
	}
	cfg := s.Config
	s.Close()
	reopened, e := Open(cfg, driver)
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.Close()
	restored, e := reopened.slicerProject(u, p.ID)
	if e != nil || restored.Name != "新名称" || restored.ActiveCopy != 1 || len(restored.Options.Copies) != 2 {
		t.Fatal("project lost across restart", restored, e)
	}
	content, e := reopened.slicerStorage(u)
	if e != nil {
		t.Fatal(e)
	}
	snapshot := content.(map[string]any)
	if snapshot["draft"].(*SlicerProject).ModelID != model || snapshot["draftRevision"].(int64) != 1 {
		t.Fatal(snapshot)
	}
	rev, e = reopened.saveSlicerDraft(u, DraftWrite{Revision: 1}, true)
	if e != nil || rev != 2 {
		t.Fatal(e)
	}
	if _, e = reopened.saveSlicerDraft(u, DraftWrite{Revision: 1, SlicerState: state}, false); e == nil {
		t.Fatal("cleared draft ABA overwrite")
	}
	if e = reopened.deleteSlicerProject(u, p.ID, 1); e == nil {
		t.Fatal("stale delete accepted")
	}
	if e = reopened.deleteSlicerProject(u, p.ID, 2); e != nil {
		t.Fatal(e)
	}
	rev, e = reopened.saveSlicerDraft(u, DraftWrite{Revision: 2, SlicerState: state}, false)
	if e != nil {
		t.Fatal(e)
	}
	if e = reopened.deleteSliceModel(u, model); e != nil {
		t.Fatal(e)
	}
	content, e = reopened.slicerStorage(u)
	if e != nil {
		t.Fatal(e)
	}
	snapshot = content.(map[string]any)
	if snapshot["draft"].(*SlicerProject) != nil || snapshot["draftRevision"].(int64) != rev+1 {
		t.Fatal("deleted source left stale draft", snapshot)
	}
	var jobs int
	if e = reopened.DB.QueryRow("SELECT COUNT(*) FROM jobs").Scan(&jobs); e != nil || jobs != 0 {
		t.Fatal("saving created print jobs", jobs, e)
	}
}
func TestSlicerStorageRejectsInvalidStateWithoutOverwriting(t *testing.T) {
	s, u, _ := setup(t)
	model := storeWorkbenchModel(t, s, u)
	state := SlicerState{ModelID: model, Options: defaultSliceSetup()}
	if _, e := s.saveSlicerDraft(u, DraftWrite{SlicerState: state}, false); e != nil {
		t.Fatal(e)
	}
	bad := state
	bad.Options.Scale = 0
	if _, e := s.saveSlicerDraft(u, DraftWrite{Revision: 1, SlicerState: bad}, false); e == nil {
		t.Fatal("invalid scale saved")
	}
	var revision int
	if e := s.DB.QueryRow("SELECT revision FROM slicer_drafts WHERE user_id=?", u.ID).Scan(&revision); e != nil || revision != 1 {
		t.Fatal("invalid save changed revision", revision, e)
	}
	for _, name := range []string{" ", strings.Repeat("字", 81), "bad\nname"} {
		if _, e := s.saveSlicerProject(u, ProjectWrite{Name: name, SlicerState: state}); e == nil {
			t.Fatal("bad name", name)
		}
	}
}
