package app

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func roleUser(t *testing.T, s *Service, name string) User {
	t.Helper()
	var id int64
	if e := s.DB.QueryRow("SELECT id FROM users WHERE username=?", name).Scan(&id); e != nil {
		t.Fatal(e)
	}
	u, e := currentUser(s.DB, id)
	if e != nil {
		t.Fatal(e)
	}
	return u
}
func roleSession(t *testing.T, s *Service, u User) string {
	t.Helper()
	token := randomID(32)
	if _, e := s.DB.Exec("INSERT INTO sessions(hash,user_id,expires) VALUES(?,?,?)", tokenHash(token), u.ID, time.Now().Add(time.Hour).Unix()); e != nil {
		t.Fatal(e)
	}
	return token
}
func roleRequest(s *Service, token, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.AddCookie(&http.Cookie{Name: "layerhub_sid", Value: token})
	r.Header.Set("X-CSRF-Token", s.csrf(token))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}
func TestRolesOwnerAdminMemberBoundariesAndSessionRevocation(t *testing.T) {
	s, owner, _ := setup(t)
	owner = roleUser(t, s, "admin")
	if !owner.Owner || !owner.Admin {
		t.Fatal("initial owner missing")
	}
	for _, v := range []struct {
		name  string
		admin bool
	}{{"manager", true}, {"peer", true}, {"maker", false}} {
		if e := s.SaveUser(owner, v.name, v.name, "MemberPassword123", v.admin); e != nil {
			t.Fatal(e)
		}
	}
	manager, peer, member := roleUser(t, s, "manager"), roleUser(t, s, "peer"), roleUser(t, s, "maker")
	if e := s.SaveUser(manager, "illegal", "illegal", "MemberPassword123", true); e == nil {
		t.Fatal("admin created peer")
	}
	if e := s.SaveUser(manager, "regular", "regular", "MemberPassword123", false); e != nil {
		t.Fatal(e)
	}
	name, disable, role := "changed", true, "admin"
	for _, target := range []User{owner, peer} {
		for _, in := range []MemberUpdate{{UserID: target.ID, Name: &name}, {UserID: target.ID, Disabled: &disable, CurrentPassword: "MemberPassword123"}, {UserID: target.ID, NewPassword: "ResetPassword123", CurrentPassword: "MemberPassword123"}} {
			if e := s.UpdateMember(manager, in); e == nil {
				t.Fatal("manager modified protected account", target.ID)
			}
		}
	}
	if e := s.UpdateMember(manager, MemberUpdate{UserID: member.ID, Name: &name}); e != nil {
		t.Fatal(e)
	}
	if e := s.UpdateMember(manager, MemberUpdate{UserID: member.ID, Role: &role, CurrentPassword: "MemberPassword123"}); e == nil {
		t.Fatal("admin promoted member")
	}
	token := roleSession(t, s, member)
	if e := s.UpdateMember(owner, MemberUpdate{UserID: member.ID, Role: &role, CurrentPassword: "wrong"}); e == nil {
		t.Fatal("promotion without owner password")
	}
	if e := s.UpdateMember(owner, MemberUpdate{UserID: member.ID, Role: &role, CurrentPassword: "TestPassword123"}); e != nil {
		t.Fatal(e)
	}
	if roleRequest(s, token, "GET", "/api/me", "").Code != 401 {
		t.Fatal("old role session survived")
	}
	if !roleUser(t, s, "maker").Admin {
		t.Fatal("promotion not saved")
	}
	role = "member"
	if e := s.UpdateMember(owner, MemberUpdate{UserID: peer.ID, Role: &role, CurrentPassword: "TestPassword123"}); e != nil {
		t.Fatal(e)
	}
	if e := s.SaveUser(peer, "stale", "stale", "MemberPassword123", false); e == nil {
		t.Fatal("stale admin authority accepted")
	}
	for _, in := range []MemberUpdate{{UserID: owner.ID, Disabled: &disable, CurrentPassword: "TestPassword123"}, {UserID: owner.ID, Role: &role, CurrentPassword: "TestPassword123"}} {
		if e := s.UpdateMember(owner, in); e == nil {
			t.Fatal("owner disabled/demoted itself")
		}
	}
	for _, u := range []User{manager, member} {
		if _, _, e := s.ExportBackup(u, "MemberPassword123"); e == nil {
			t.Fatal("non-owner exported backup")
		}
		if _, _, e := s.stageRestore(u, strings.NewReader("not a backup")); e == nil {
			t.Fatal("non-owner staged restore")
		}
		if e := s.confirmRestore(u, "x", "MemberPassword123"); e == nil {
			t.Fatal("non-owner confirmed restore")
		}
		if e := s.cancelRestore(u); e == nil {
			t.Fatal("non-owner cancelled restore")
		}
		if roleRequest(s, roleSession(t, s, u), "GET", "/api/backup/status", "").Code != 403 {
			t.Fatal("backup status exposed")
		}
	}
}
func TestRolesOwnershipTransferIsAtomicExplicitAndSurvivesRestart(t *testing.T) {
	s, owner, _ := setup(t)
	s.SaveUser(owner, "nextowner", "接任者", "MemberPassword123", false)
	target := roleUser(t, s, "nextowner")
	oldToken, newToken := roleSession(t, s, owner), roleSession(t, s, target)
	for _, in := range []OwnershipTransfer{{UserID: target.ID, CurrentPassword: "wrong", ConfirmUsername: target.Username}, {UserID: target.ID, CurrentPassword: "TestPassword123", ConfirmUsername: "wrong"}, {UserID: owner.ID, CurrentPassword: "TestPassword123", ConfirmUsername: owner.Username}} {
		if e := s.TransferOwnership(owner, in); e == nil {
			t.Fatal("invalid transfer accepted")
		}
	}
	if e := s.TransferOwnership(owner, OwnershipTransfer{UserID: target.ID, CurrentPassword: "TestPassword123", ConfirmUsername: target.Username}); e != nil {
		t.Fatal(e)
	}
	if roleUser(t, s, "admin").Owner || !roleUser(t, s, "nextowner").Owner {
		t.Fatal("transfer failed")
	}
	for _, token := range []string{oldToken, newToken} {
		if roleRequest(s, token, "GET", "/api/me", "").Code != 401 {
			t.Fatal("transfer session survived")
		}
	}
	if e := s.TransferOwnership(owner, OwnershipTransfer{UserID: target.ID, CurrentPassword: "TestPassword123", ConfirmUsername: target.Username}); e == nil {
		t.Fatal("stale owner transferred again")
	}
	cfg := s.Config
	s.Close()
	reopened, e := Open(cfg, &fakeDevice{})
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.Close()
	if roleUser(t, reopened, "admin").Owner || !roleUser(t, reopened, "nextowner").Owner {
		t.Fatal("bootstrap name reclaimed ownership")
	}
	archive, cleanup, e := reopened.ExportBackup(roleUser(t, reopened, "nextowner"), "MemberPassword123")
	if e != nil {
		t.Fatal(e)
	}
	defer cleanup()
	f, e := os.Open(archive)
	if e != nil {
		t.Fatal(e)
	}
	preview, ticket, e := reopened.stageRestore(roleUser(t, reopened, "nextowner"), f)
	f.Close()
	if e != nil || ticket == "" {
		t.Fatal("owner backup roundtrip", e)
	}
	if preview.OwnerUsername != "nextowner" {
		t.Fatal("restore owner missing from preview", preview.OwnerUsername)
	}
	if e = reopened.TransferOwnership(roleUser(t, reopened, "nextowner"), OwnershipTransfer{UserID: owner.ID, CurrentPassword: "MemberPassword123", ConfirmUsername: "admin"}); e == nil {
		t.Fatal("transferred while restore staged")
	}
}
func TestRolesLegacyMigrationPreservesAccountsAndFailsClosed(t *testing.T) {
	s, owner, _ := setup(t)
	s.SaveUser(owner, "manager", "manager", "MemberPassword123", true)
	var before string
	s.DB.QueryRow("SELECT password_hash FROM users WHERE id=?", owner.ID).Scan(&before)
	if _, e := s.DB.Exec("DROP INDEX one_workspace_owner;ALTER TABLE users DROP COLUMN owner;DROP TABLE workspace_security;DROP TABLE role_audit;PRAGMA user_version=3"); e != nil {
		t.Fatal(e)
	}
	if e := initOwnership(s.DB, "admin"); e != nil {
		t.Fatal(e)
	}
	if !roleUser(t, s, "admin").Owner || roleUser(t, s, "manager").Owner {
		t.Fatal("wrong migration owner")
	}
	var after string
	s.DB.QueryRow("SELECT password_hash FROM users WHERE id=?", owner.ID).Scan(&after)
	if before != after {
		t.Fatal("migration changed password")
	}
	if _, e := s.DB.Exec("UPDATE users SET owner=0"); e != nil {
		t.Fatal(e)
	}
	if e := initOwnership(s.DB, "admin"); e == nil {
		t.Fatal("missing owner silently reassigned")
	}
	if _, e := s.DB.Exec("UPDATE users SET owner=1,disabled=1 WHERE id=?", owner.ID); e != nil {
		t.Fatal(e)
	}
	if validateBackupOwnership(s.DB, 4) == nil {
		t.Fatal("disabled owner backup accepted")
	}
}
func TestRolesConcurrentTransfersHaveOneWinner(t *testing.T) {
	s, owner, _ := setup(t)
	for _, n := range []string{"nexta", "nextb"} {
		s.SaveUser(owner, n, n, "MemberPassword123", false)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, n := range []string{"nexta", "nextb"} {
		u := roleUser(t, s, n)
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- s.TransferOwnership(owner, OwnershipTransfer{UserID: u.ID, CurrentPassword: "TestPassword123", ConfirmUsername: u.Username})
		}()
	}
	wg.Wait()
	close(results)
	success := 0
	for e := range results {
		if e == nil {
			success++
		}
	}
	var count int
	s.DB.QueryRow("SELECT COUNT(*) FROM users WHERE owner=1 AND admin=1 AND disabled=0").Scan(&count)
	if success != 1 || count != 1 {
		t.Fatal("non-atomic ownership", success, count)
	}
}
