package app

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAccountsPasswordAndDisableInvalidateSessions(t *testing.T) {
	s, admin, _ := setup(t)
	if err := s.SaveUser(admin, "member", "测试成员", "MemberPassword123", false); err != nil {
		t.Fatal(err)
	}
	var member User
	s.DB.QueryRow("SELECT id,username,name,admin FROM users WHERE username='member'").Scan(&member.ID, &member.Username, &member.Name, &member.Admin)
	login := func(username, password string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/api/login", strings.NewReader(`{"Username":"`+username+`","Password":"`+password+`"}`))
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		return rec
	}
	response := login("member", "MemberPassword123")
	if response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	cookie := response.Result().Cookies()[0]
	if s.ChangePassword(member, "wrong", "UpdatedPassword123") == nil {
		t.Fatal("wrong current password accepted")
	}
	if err := s.ChangePassword(member, "MemberPassword123", "UpdatedPassword123"); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/api/me", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Fatal("old session survived password change")
	}
	if login("member", "MemberPassword123").Code != 401 || login("member", "UpdatedPassword123").Code != 200 {
		t.Fatal("password was not replaced")
	}
	disable := true
	if s.UpdateMember(member, MemberUpdate{UserID: admin.ID, Disabled: &disable, CurrentPassword: "UpdatedPassword123"}) == nil {
		t.Fatal("member can manage users")
	}
	if s.UpdateMember(admin, MemberUpdate{UserID: admin.ID, Disabled: &disable, CurrentPassword: "TestPassword123"}) == nil {
		t.Fatal("self-disable permitted")
	}
	if err := s.UpdateMember(admin, MemberUpdate{UserID: member.ID, Disabled: &disable, CurrentPassword: "TestPassword123"}); err != nil {
		t.Fatal(err)
	}
	if login("member", "UpdatedPassword123").Code != 401 {
		t.Fatal("disabled member signed in")
	}
	disable = false
	if err := s.UpdateMember(admin, MemberUpdate{UserID: member.ID, Disabled: &disable, NewPassword: "ResetPassword123", CurrentPassword: "TestPassword123"}); err != nil {
		t.Fatal(err)
	}
	if login("member", "ResetPassword123").Code != 200 {
		t.Fatal("reset member cannot sign in")
	}
}

func TestBackupRoundTripAndRestartRecovery(t *testing.T) {
	s, admin, _ := setup(t)
	p, sp := fixture(t, s, admin)
	jobID := newJob(t, s, admin, p, sp, 20)
	s.DB.Exec("INSERT INTO sessions(hash,user_id,expires) VALUES('must-not-restore',?,9999999999)", admin.ID)
	if _, err := s.DB.Exec("INSERT INTO session_details(hash,created,client) VALUES('must-not-restore',123,'Chrome · Windows')"); err != nil {
		t.Fatal(err)
	}
	archive, cleanup, err := s.ExportBackup(admin, "TestPassword123")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	f, err := os.Open(archive)
	if err != nil {
		t.Fatal(err)
	}
	m, ticket, err := s.stageRestore(admin, f)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	if m.Jobs != 1 || m.Printers != 1 || m.Spools != 1 || s.pendingRestore.Load() {
		t.Fatal("preview changed workspace or wrong counts")
	}
	// Later changes must be retained in the pre-restore directory, not silently lost.
	sp.Remaining = 950
	if err = s.SaveSpool(admin, sp); err != nil {
		t.Fatal(err)
	}
	if s.confirmRestore(admin, ticket, "wrong") == nil {
		t.Fatal("restore accepted incorrect password")
	}
	if err = s.confirmRestore(admin, ticket, "TestPassword123"); err != nil {
		t.Fatal(err)
	}
	if s.SaveUser(admin, "blocked", "不应创建", "Password123", false) == nil {
		t.Fatal("write allowed after restore confirmed")
	}
	if _, err = Open(s.Config, &fakeDevice{}); err == nil {
		t.Fatal("second process can open the same data directory")
	}
	cfg := s.Config
	s.Close()
	// Simulate a restart interrupted immediately after the original directory was moved.
	raw, _ := os.ReadFile(cfg.DataDir + ".restore.json")
	var marker restoreMarker
	json.Unmarshal(raw, &marker)
	if err = os.Rename(cfg.DataDir, marker.Previous); err != nil {
		t.Fatal(err)
	}
	restored, err := Open(cfg, &fakeDevice{})
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	var remaining, sessions, sessionDetails int
	var path string
	var enabled bool
	restored.DB.QueryRow("SELECT remaining FROM spools WHERE id=?", sp.ID).Scan(&remaining)
	restored.DB.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&sessions)
	if err = restored.DB.QueryRow("SELECT COUNT(*) FROM session_details").Scan(&sessionDetails); err != nil || sessionDetails != 0 {
		t.Fatal("session details survived backup restore", sessionDetails, err)
	}
	restored.DB.QueryRow("SELECT enabled FROM printers WHERE id=?", p.ID).Scan(&enabled)
	restored.DB.QueryRow("SELECT path FROM jobs WHERE id=?", jobID).Scan(&path)
	if remaining != 1000 || sessions != 0 || enabled || !filepath.IsAbs(path) {
		t.Fatalf("invalid restored workspace: remaining=%d sessions=%d enabled=%v path=%s", remaining, sessions, enabled, path)
	}
	if _, err = os.Stat(path); err != nil {
		t.Fatal("portable upload was not restored", err)
	}
	oldDB, err := openBackupDB(filepath.Join(marker.Previous, "layerhub.sqlite"), true)
	if err != nil {
		t.Fatal(err)
	}
	defer oldDB.Close()
	if err = oldDB.QueryRow("SELECT remaining FROM spools WHERE id=?", sp.ID).Scan(&remaining); err != nil || remaining != 950 {
		t.Fatal("original workspace not preserved", err)
	}
}

func TestRestoreRejectsZipTraversalAndTampering(t *testing.T) {
	s, admin, _ := setup(t)
	p, sp := fixture(t, s, admin)
	newJob(t, s, admin, p, sp, 5)
	var evil bytes.Buffer
	z := zip.NewWriter(&evil)
	entry, _ := z.Create("../escaped.txt")
	entry.Write([]byte("bad"))
	z.Close()
	if _, _, err := s.stageRestore(admin, &evil); err == nil {
		t.Fatal("zip traversal accepted")
	}
	archive, cleanup, err := s.ExportBackup(admin, "TestPassword123")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	original, err := zip.OpenReader(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer original.Close()
	var broken bytes.Buffer
	writer := zip.NewWriter(&broken)
	for _, file := range original.File {
		input, _ := file.Open()
		out, _ := writer.Create(file.Name)
		if file.Name == "master.key" {
			out.Write(bytes.Repeat([]byte{7}, 32))
		} else {
			io.Copy(out, input)
		}
		input.Close()
	}
	writer.Close()
	if _, _, err = s.stageRestore(admin, &broken); err == nil {
		t.Fatal("corrupt backup accepted")
	}
	if s.pendingRestore.Load() {
		t.Fatal("invalid archive scheduled a restore")
	}
}

func TestBackupPermissionsAndCancellation(t *testing.T) {
	s, admin, _ := setup(t)
	if _, _, err := s.ExportBackup(User{ID: 99}, "anything"); err == nil {
		t.Fatal("non-admin exported backup")
	}
	if _, _, err := s.ExportBackup(admin, "wrong"); err == nil {
		t.Fatal("backup exported without reauthentication")
	}
	path, cleanup, err := s.ExportBackup(admin, "TestPassword123")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	f, _ := os.Open(path)
	_, ticket, err := s.stageRestore(admin, f)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	if err = s.confirmRestore(admin, ticket, "TestPassword123"); err != nil {
		t.Fatal(err)
	}
	if err = s.cancelRestore(admin); err != nil {
		t.Fatal(err)
	}
	if s.pendingRestore.Load() {
		t.Fatal("cancel did not unfreeze workspace")
	}
	if err = s.SaveUser(admin, "aftercancel", "取消后", "Password123", false); err != nil {
		t.Fatal(err)
	}
}

func TestUpgradeFromOriginalUserSchemaKeepsWorkspace(t *testing.T) {
	s, admin, _ := setup(t)
	p, sp := fixture(t, s, admin)
	jobID := newJob(t, s, admin, p, sp, 12)
	if _, err := s.DB.Exec("ALTER TABLE users DROP COLUMN disabled; PRAGMA user_version=1;"); err != nil {
		t.Fatal(err)
	}
	cfg := s.Config
	s.Close()
	upgraded, err := Open(cfg, &fakeDevice{})
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	var disabled bool
	var version, count int
	var hash string
	if err = upgraded.DB.QueryRow("SELECT disabled,password_hash FROM users WHERE id=?", admin.ID).Scan(&disabled, &hash); err != nil {
		t.Fatal(err)
	}
	upgraded.DB.QueryRow("PRAGMA user_version").Scan(&version)
	upgraded.DB.QueryRow("SELECT COUNT(*) FROM jobs WHERE id=? AND spool_id=?", jobID, sp.ID).Scan(&count)
	if disabled || version != 4 || count != 1 || !verifyPassword("TestPassword123", hash) {
		t.Fatal("upgrade lost existing data or changed credentials")
	}
}
