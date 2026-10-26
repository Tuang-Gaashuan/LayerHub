package app

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
)

func sharedFixture(t *testing.T, s *Service, u User) SlicerProject {
	t.Helper()
	id := storeWorkbenchModel(t, s, u)
	o := defaultSliceSetup()
	walls, infill := 4, 30
	o.Copies = []ModelPlacement{{ModelID: id, InstanceID: "first", X: 80, Y: 128, Scale: 1}, {ModelID: id, InstanceID: "second", X: 170, Y: 128, Scale: 1, Process: &ObjectProcess{Walls: &walls, Infill: &infill}}}
	p, e := s.saveSlicerProject(u, ProjectWrite{Name: "私人支架", SlicerState: SlicerState{FormatVersion: 4, ModelID: id, Options: o, Plates: []SlicerPlate{{ID: "one", Name: "支架盘", ModelID: id, Options: o}, {ID: "two", Name: "备用盘", Options: defaultSliceSetup()}}}})
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func sharedPublish(p SlicerProject) TeamPublish {
	return TeamPublish{ProjectID: p.ID, Revision: p.Revision, Title: "团队支架", Description: "原尺寸，不自动打印", Note: "首个版本", RequestKey: randomID(16), Acknowledge: "team-models-and-settings"}
}
func sharedID(t *testing.T, s *Service, version int64) int64 {
	t.Helper()
	var id int64
	if e := s.DB.QueryRow("SELECT project_id FROM team_project_versions WHERE id=?", version).Scan(&id); e != nil {
		t.Fatal(e)
	}
	return id
}

func TestTeamProjectImmutableCopyIsolationAndSourceDeletion(t *testing.T) {
	s, u, driver := setup(t)
	if e := s.SaveUser(u, "reader", "读者", "MemberPassword123", false); e != nil {
		t.Fatal(e)
	}
	reader := roleUser(t, s, "reader")
	p := sharedFixture(t, s, u)
	// A private task link is never shared, even if it was in a saved board.
	raw, _ := json.Marshal(p.Options)
	r, e := s.DB.Exec("INSERT INTO slice_tasks(user_id,model_id,state,options,created,updated) VALUES(?,?,'succeeded',?,1,1)", u.ID, p.ModelID, string(raw))
	if e != nil {
		t.Fatal(e)
	}
	task, _ := r.LastInsertId()
	p.Plates[0].TaskID = task
	p, e = s.saveSlicerProject(u, ProjectWrite{ID: p.ID, Revision: p.Revision, Name: p.Name, SlicerState: p.SlicerState})
	if e != nil {
		t.Fatal(e)
	}
	in := sharedPublish(p)
	version, replayed, e := s.publishTeamProject(u, in)
	if e != nil || replayed {
		t.Fatal(version, e)
	}
	id := sharedID(t, s, version)
	detail, e := s.teamProjectDetail(reader, id, 0)
	if e != nil || len(detail.Version.Sources) != 1 || detail.Version.State.Plates[0].TaskID != 0 || detail.Version.State.Plates[1].ModelID != 0 {
		t.Fatal(detail, e)
	}
	if detail.Version.State.ModelID != 1 {
		t.Fatal("shared IDs are not local")
	}
	if _, e = s.slicerProject(reader, p.ID); e == nil {
		t.Fatal("share granted access to private project")
	}
	if _, _, e = s.publishTeamProject(reader, in); e == nil {
		t.Fatal("reader published private project")
	}
	copyIn := TeamCopy{VersionID: version, RequestKey: randomID(16), Name: "我的支架"}
	copy, replay, e := s.copyTeamProject(reader, copyIn)
	if e != nil || replay {
		t.Fatal(e)
	}
	if copy.ModelID == p.ModelID || copy.Plates[0].ID == p.Plates[0].ID || copy.Plates[0].Options.Copies[0].InstanceID == p.Options.Copies[0].InstanceID || copy.Plates[0].TaskID != 0 || copy.Plates[0].Options.Copies[1].Process == nil || *copy.Plates[0].Options.Copies[1].Process.Walls != 4 {
		t.Fatal("copy lost independence or process", copy)
	}
	var a, b []byte
	s.DB.QueryRow("SELECT stl FROM model_sources WHERE id=?", p.ModelID).Scan(&a)
	s.DB.QueryRow("SELECT stl FROM model_sources WHERE id=? AND user_id=?", copy.ModelID, reader.ID).Scan(&b)
	if string(a) != string(b) {
		t.Fatal("geometry changed")
	}
	if _, e = s.saveSlicerDraft(reader, DraftWrite{SlicerState: copy.SlicerState}, false); e != nil {
		t.Fatal(e)
	}
	var draft string
	s.DB.QueryRow("SELECT options FROM slicer_drafts WHERE user_id=?", reader.ID).Scan(&draft)
	again, replay, e := s.copyTeamProject(reader, copyIn)
	if e != nil || !replay || again.ID != copy.ID {
		t.Fatal("copy receipt failed", e)
	}
	version2, replay, e := s.publishTeamProject(u, in)
	if e != nil || !replay || version2 != version {
		t.Fatal("publish receipt failed", e)
	}
	copyIn.Name = "内容变化"
	if _, _, e = s.copyTeamProject(reader, copyIn); e == nil {
		t.Fatal("nonce reused for different copy")
	}
	if e = s.deleteSlicerProject(u, p.ID, p.Revision); e != nil {
		t.Fatal(e)
	}
	if e = s.deleteSliceModel(u, p.ModelID); e != nil {
		t.Fatal(e)
	}
	if _, e = s.teamProjectDetail(reader, id, version); e != nil {
		t.Fatal("original removal destroyed published snapshot", e)
	}
	if _, _, e = s.copyTeamProject(reader, TeamCopy{VersionID: version, RequestKey: randomID(16), Name: "删除原件后再复制"}); e != nil {
		t.Fatal(e)
	}
	var after string
	s.DB.QueryRow("SELECT options FROM slicer_drafts WHERE user_id=?", reader.ID).Scan(&after)
	if draft != after {
		t.Fatal("copy overwrote draft")
	}
	var commands, jobs int
	s.DB.QueryRow("SELECT COUNT(*) FROM printer_commands").Scan(&commands)
	s.DB.QueryRow("SELECT COUNT(*) FROM jobs").Scan(&jobs)
	if commands != 0 || jobs != 0 || driver.uploads > 0 {
		t.Fatal("sharing dispatched device operation")
	}
}

func TestTeamProjectVersionsWithdrawalModerationAndStaleRoles(t *testing.T) {
	s, u, _ := setup(t)
	s.SaveUser(u, "author", "作者", "MemberPassword123", false)
	s.SaveUser(u, "reader", "读者", "MemberPassword123", false)
	author, reader := roleUser(t, s, "author"), roleUser(t, s, "reader")
	p := sharedFixture(t, s, author)
	in := sharedPublish(p)
	v1, _, e := s.publishTeamProject(author, in)
	if e != nil {
		t.Fatal(e)
	}
	id := sharedID(t, s, v1)
	if e = s.setTeamVisibility(reader, TeamVisibility{ID: id, ExpectedVersion: 1, Withdrawn: true}); e == nil {
		t.Fatal("reader withdrew another author")
	}
	in.SharedID = id
	in.ExpectedVersion = 1
	in.Note = "保留旧版，第二版说明"
	in.RequestKey = randomID(16)
	v2, _, e := s.publishTeamProject(author, in)
	if e != nil {
		t.Fatal(e)
	}
	detail, e := s.teamProjectDetail(reader, id, v1)
	if e != nil || detail.Version.Number != 1 || len(detail.Versions) != 2 || detail.Version.Note != "首个版本" || detail.Project.Latest != 2 {
		t.Fatal("version overwritten", e)
	}
	in.RequestKey = randomID(16)
	if _, _, e = s.publishTeamProject(author, in); e == nil {
		t.Fatal("stale version accepted")
	}
	if e = s.setTeamVisibility(u, TeamVisibility{ID: id, ExpectedVersion: 2, Withdrawn: true}); e != nil {
		t.Fatal(e)
	}
	if _, e = s.teamProjectDetail(reader, id, v2); e == nil {
		t.Fatal("withdrawn version visible to reader")
	}
	if _, _, e = s.copyTeamProject(reader, TeamCopy{VersionID: v2, RequestKey: randomID(16), Name: "不应复制"}); e == nil {
		t.Fatal("withdrawn version copied")
	}
	if e = s.setTeamVisibility(author, TeamVisibility{ID: id, ExpectedVersion: 2, Withdrawn: false}); e == nil {
		t.Fatal("author undid administrator moderation")
	}
	if e = s.setTeamVisibility(u, TeamVisibility{ID: id, ExpectedVersion: 2, Withdrawn: false}); e != nil {
		t.Fatal(e)
	}
	var count int
	s.DB.QueryRow("SELECT COUNT(*) FROM team_project_versions WHERE project_id=?", id).Scan(&count)
	if count != 2 {
		t.Fatal("visibility changed versions")
	}
	if _, e = s.teamProjects(reader, "manage", "", 0); e == nil {
		t.Fatal("member used management listing")
	}
	if e = s.setTeamVisibility(User{ID: reader.ID, Admin: true}, TeamVisibility{ID: id, ExpectedVersion: 2, Withdrawn: true}); e == nil {
		t.Fatal("stale administrator role accepted")
	}
}

func TestTeamProjectCopyQuotaRollbackAndConcurrentReceipts(t *testing.T) {
	s, u, _ := setup(t)
	s.SaveUser(u, "reader", "读者", "MemberPassword123", false)
	reader := roleUser(t, s, "reader")
	p := sharedFixture(t, s, u)
	v, _, e := s.publishTeamProject(u, sharedPublish(p))
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 30; i++ {
		storeWorkbenchModel(t, s, reader)
	}
	if _, _, e = s.copyTeamProject(reader, TeamCopy{VersionID: v, RequestKey: randomID(16), Name: "超额"}); e == nil {
		t.Fatal("model quota bypassed")
	}
	var n int
	s.DB.QueryRow("SELECT COUNT(*) FROM slicer_projects WHERE user_id=?", reader.ID).Scan(&n)
	if n != 0 {
		t.Fatal("quota failure left project")
	}
	s.DB.Exec("DELETE FROM model_sources WHERE user_id=?", reader.ID)
	in := TeamCopy{VersionID: v, RequestKey: randomID(16), Name: "并发副本"}
	var wg sync.WaitGroup
	ids := make(chan int64, 2)
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); p, _, e := s.copyTeamProject(reader, in); ids <- p.ID; errs <- e }()
	}
	wg.Wait()
	close(ids)
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	first := int64(0)
	for id := range ids {
		if first == 0 {
			first = id
		}
		if id != first {
			t.Fatal("duplicate copy")
		}
	}
	s.DB.QueryRow("SELECT COUNT(*) FROM model_sources WHERE user_id=?", reader.ID).Scan(&n)
	if n != 1 {
		t.Fatal("receipt duplicated source", n)
	}
	if e = s.deleteSlicerProject(reader, first, 1); e != nil {
		t.Fatal(e)
	}
	if _, _, e = s.copyTeamProject(reader, in); e == nil {
		t.Fatal("deleted receipt silently re-created project")
	}
}

func TestTeamProjectBackupRestoreAndHTTPBoundaries(t *testing.T) {
	s, u, _ := setup(t)
	p := sharedFixture(t, s, u)
	v, _, e := s.publishTeamProject(u, sharedPublish(p))
	if e != nil {
		t.Fatal(e)
	}
	copy, _, e := s.copyTeamProject(u, TeamCopy{VersionID: v, RequestKey: randomID(16), Name: "本人也可复用"})
	if e != nil {
		t.Fatal(e)
	}
	token := roleSession(t, s, u)
	for _, body := range []string{`{"projectId":1}`, `{"projectId":1,"owner":true}`} {
		if roleRequest(s, token, "POST", "/api/team-projects/publish", body).Code != 400 {
			t.Fatal("invalid publish accepted")
		}
	}
	if roleRequest(s, token, "GET", "/api/team-projects?before=bad", "").Code != 400 {
		t.Fatal("bad cursor accepted")
	}
	if roleRequest(s, token, "GET", "/api/team-projects/detail?id=1&version=-1", "").Code != 400 {
		t.Fatal("bad version accepted")
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
	if _, e = restored.teamProjectDetail(u, sharedID(t, restored, v), v); e != nil {
		t.Fatal(e)
	}
	origin, e := restored.teamCopyOrigin(u, copy.ID)
	if e != nil || origin == nil {
		t.Fatal("copy origin lost", e)
	}
	var fk string
	err := restored.DB.QueryRow("PRAGMA foreign_key_check").Scan(&fk)
	if err != sql.ErrNoRows {
		t.Fatal(fmt.Sprint("broken backup references ", err))
	}
}

func TestTeamProjectPinnedEngineCopyRoundtrip(t *testing.T) {
	if os.Getenv("LAYERHUB_TEAM_ENGINE") != "1" {
		t.Skip("explicit isolated pinned-engine acceptance")
	}
	s, u, _ := setup(t)
	s.SaveUser(u, "reader", "读者", "MemberPassword123", false)
	reader := roleUser(t, s, "reader")
	s.Config.SlicerPath = os.Getenv("LAYERHUB_OBJECT_ENGINE")
	if s.Config.SlicerPath == "" {
		s.Config.SlicerPath, _ = filepath.Abs("../../qa/slicing-poc/runtime/bambu-studio.exe")
	}
	s.Config.SlicerProfiles = os.Getenv("LAYERHUB_OBJECT_PROFILES")
	if s.Config.SlicerProfiles == "" {
		s.Config.SlicerProfiles, _ = filepath.Abs("../../qa/slicing-poc/flattened")
	}
	a, b := storeMixedSources(t, s, u)
	o := mixedOptions(a, b)
	o.Copies[0].InstanceID = "cube"
	o.Copies[1].InstanceID = "box"
	o.Copies[1].Process = &ObjectProcess{Walls: intValue(4), Infill: intValue(30), TopThickness: floatValue(1.2), Pattern: textValue("gyroid")}
	p, e := s.saveSlicerProject(u, ProjectWrite{Name: "多模型共享", SlicerState: SlicerState{FormatVersion: 4, ModelID: a, Options: o, Plates: []SlicerPlate{{ID: "first", Name: "混排盘", ModelID: a, Options: o}, {ID: "empty", Name: "空盘", Options: defaultSliceSetup()}}}})
	if e != nil {
		t.Fatal(e)
	}
	v, _, e := s.publishTeamProject(u, sharedPublish(p))
	if e != nil {
		t.Fatal(e)
	}
	copied, _, e := s.copyTeamProject(reader, TeamCopy{VersionID: v, RequestKey: randomID(16), Name: "副本重新切片"})
	if e != nil {
		t.Fatal(e)
	}
	outputs := [][]byte{}
	for _, c := range []struct {
		user    User
		project SlicerProject
	}{{u, p}, {reader, copied}} {
		board := c.project.Plates[0]
		id, e := s.queueSlice(c.user, board.ModelID, board.Options)
		if e != nil {
			t.Fatal(e)
		}
		s.runSlice(id)
		task, e := s.slicerTask(c.user, id)
		if e != nil || task.State != "succeeded" {
			t.Fatal(task.State, task.Error, e)
		}
		var raw []byte
		if e = s.DB.QueryRow("SELECT output FROM slice_tasks WHERE id=?", id).Scan(&raw); e != nil {
			t.Fatal(e)
		}
		outputs = append(outputs, raw)
		t.Logf("account %d: task %d, %d bytes, %d seconds", c.user.ID, id, len(raw), task.Info.Plates[0].Seconds)
	}
	left, right := testOutputPaths(t, outputs[0]), testOutputPaths(t, outputs[1])
	if !reflect.DeepEqual(left, right) {
		t.Fatal("copy changed actual layer paths despite identical geometry and process")
	}
	t.Logf("same real paths after publish/copy: %d layers, %d segments; two sources, separate overrides, empty board retained", len(left.Layers), left.Total)
}
