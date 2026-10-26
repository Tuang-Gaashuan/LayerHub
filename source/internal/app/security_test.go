package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func securityLogin(t *testing.T, s *Service, username, password, client string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"Username": username, "Password": password})
	r := httptest.NewRequest("POST", "/api/login", strings.NewReader(string(body)))
	r.Header.Set("User-Agent", client)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("login: %d %s", w.Code, w.Body.String())
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == "layerhub_sid" {
			return c.Value
		}
	}
	t.Fatal("missing session cookie")
	return ""
}

func TestSecuritySessionsPrivateRevocableAndLegacyCompatible(t *testing.T) {
	s, owner, _ := setup(t)
	if e := s.SaveUser(owner, "maker", "成员", "MemberPassword123", false); e != nil {
		t.Fatal(e)
	}
	member := roleUser(t, s, "maker")
	current := securityLogin(t, s, "maker", "MemberPassword123", "Mozilla/5.0 Windows NT Chrome/140.0 Safari/537.36")
	other := securityLogin(t, s, "maker", "MemberPassword123", "Mozilla/5.0 Linux Firefox/140.0")
	legacy := roleSession(t, s, member)
	ownerToken := roleSession(t, s, owner)
	expired := roleSession(t, s, member)
	s.DB.Exec("UPDATE sessions SET expires=? WHERE hash=?", time.Now().Add(-time.Hour).Unix(), tokenHash(expired))
	out, e := s.accountSessions(member, current)
	if e != nil || out.Total != 3 || out.Other != 2 || len(out.Sessions) != 3 || !out.Sessions[0].Current {
		t.Fatal("bad list", out, e)
	}
	var otherID, legacyID string
	for _, v := range out.Sessions {
		if v.ID == s.sessionID(tokenHash(other)) {
			otherID = v.ID
			if v.Client != "Firefox · Linux" || v.Created <= 0 {
				t.Fatal("bad coarse label", v)
			}
		}
		if v.ID == s.sessionID(tokenHash(legacy)) {
			legacyID = v.ID
			if v.Created != 0 || v.Client != "升级前登录" {
				t.Fatal("legacy metadata fabricated", v)
			}
		}
	}
	if otherID == "" || legacyID == "" {
		t.Fatal("missing sessions")
	}
	encoded, _ := json.Marshal(out)
	for _, secret := range []string{current, other, legacy, ownerToken, tokenHash(current), tokenHash(other), tokenHash(legacy), tokenHash(ownerToken)} {
		if strings.Contains(string(encoded), secret) {
			t.Fatal("session secret exposed")
		}
	}
	for _, in := range []SessionRevoke{
		{SessionID: otherID, CurrentPassword: "wrong"},
		{SessionID: s.sessionID(tokenHash(ownerToken)), CurrentPassword: "MemberPassword123"},
		{SessionID: out.Sessions[0].ID, CurrentPassword: "MemberPassword123"},
		{SessionID: tokenHash(other), CurrentPassword: "MemberPassword123"},
		{SessionID: otherID, Others: true, CurrentPassword: "MemberPassword123"},
		{SessionID: "bad", CurrentPassword: "MemberPassword123"},
		{CurrentPassword: "MemberPassword123"},
	} {
		if _, e = s.revokeAccountSessions(member, current, in); e == nil {
			t.Fatal("unsafe revocation accepted", in.SessionID)
		}
	}
	count, e := s.revokeAccountSessions(member, current, SessionRevoke{SessionID: otherID, CurrentPassword: "MemberPassword123"})
	if e != nil || count != 1 || roleRequest(s, other, "GET", "/api/me", "").Code != 401 {
		t.Fatal("single revocation failed", count, e)
	}
	var details int
	s.DB.QueryRow("SELECT COUNT(*) FROM session_details WHERE hash=?", tokenHash(other)).Scan(&details)
	if details != 0 {
		t.Fatal("revoked metadata retained")
	}
	count, e = s.revokeAccountSessions(member, current, SessionRevoke{Others: true, CurrentPassword: "MemberPassword123"})
	if e != nil || count != 1 || roleRequest(s, legacy, "GET", "/api/me", "").Code != 401 {
		t.Fatal("other revocation failed", count, e)
	}
	for _, token := range []string{current, ownerToken} {
		if roleRequest(s, token, "GET", "/api/me", "").Code != 200 {
			t.Fatal("current or another user logged out")
		}
	}
	// A request authenticated before another window revokes its session must
	// not use a stale in-memory User to read or revoke after deletion.
	if _, e = s.accountSessions(member, legacy); e == nil {
		t.Fatal("revoked session read list")
	}
	if _, e = s.revokeAccountSessions(member, legacy, SessionRevoke{Others: true, CurrentPassword: "MemberPassword123"}); e == nil {
		t.Fatal("revoked session removed current session")
	}
	if _, e = s.accountSessions(owner, current); e == nil {
		t.Fatal("forged user used another account token")
	}
}

func TestSecuritySessionHTTPRejectsForgeryAndCSRF(t *testing.T) {
	s, owner, _ := setup(t)
	token := roleSession(t, s, owner)
	for _, path := range []string{"/api/account/sessions/revoke", "/api/account/sessions"} {
		r := httptest.NewRequest("POST", path, strings.NewReader(`{"others":true,"currentPassword":"TestPassword123"}`))
		r.AddCookie(&http.Cookie{Name: "layerhub_sid", Value: token})
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal("missing CSRF accepted", w.Code)
		}
	}
	if roleRequest(s, token, "POST", "/api/account/sessions/revoke", `{"others":true,"currentPassword":"TestPassword123","userId":99}`).Code != 400 {
		t.Fatal("foreign user ID accepted")
	}
	w := roleRequest(s, token, "GET", "/api/account/sessions?userId=999", "")
	var out SessionList
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil || out.Total != 1 || !out.Sessions[0].Current {
		t.Fatal("session HTTP list", w.Code)
	}
	if roleRequest(s, token, "POST", "/api/account/sessions/revoke", `{"others":true,"currentPassword":"TestPassword123"}`).Code != 200 {
		t.Fatal("zero other sessions not idempotent")
	}
	// Raw hostile header content is never stored or returned.
	for _, agent := range []string{"<img src=x onerror=alert(1)>", strings.Repeat("x", 10000)} {
		if got := sessionClient(agent); got != "浏览器 · 未知系统" {
			t.Fatal("raw client data persisted", got)
		}
	}
}

func TestSecurityRoleHistoryOwnerOnlyPaginatedAndImmutable(t *testing.T) {
	s, owner, _ := setup(t)
	if e := s.SaveUser(owner, "manager", "设备管理员", "MemberPassword123", true); e != nil {
		t.Fatal(e)
	}
	if e := s.SaveUser(owner, "maker", "成员", "MemberPassword123", false); e != nil {
		t.Fatal(e)
	}
	member, manager := roleUser(t, s, "maker"), roleUser(t, s, "manager")
	for _, u := range []User{member, manager, {ID: member.ID, Owner: true, Admin: true}} {
		if _, e := s.roleEvents(u, 0); e == nil {
			t.Fatal("nonowner read role events")
		}
	}
	for i := 0; i < 55; i++ {
		if _, e := s.DB.Exec("INSERT INTO role_audit(actor_id,user_id,old_role,new_role,reason,created) VALUES(?,?,'member','admin','role_change',?)", owner.ID, member.ID, time.Now().UnixMilli()); e != nil {
			t.Fatal(e)
		}
	}
	token := roleSession(t, s, owner)
	first, e := s.roleEvents(owner, 0)
	if e != nil || len(first.Events) != 50 || first.NextCursor == 0 || first.Events[0].Actor.ID != owner.ID || first.Events[0].Target.ID != member.ID {
		t.Fatal("first history page", first, e)
	}
	last, e := s.roleEvents(owner, first.NextCursor)
	if e != nil || len(last.Events) != 8 || last.NextCursor != 0 || last.Events[0].ID >= first.NextCursor {
		t.Fatal("history cursor overlapped", last, e)
	}
	for _, suffix := range []string{"?before=bad", "?before=-1", "?before=0", "?before=999999999999999999999999"} {
		if roleRequest(s, token, "GET", "/api/users/role-events"+suffix, "").Code != 400 {
			t.Fatal("bad cursor accepted", suffix)
		}
	}
	if roleRequest(s, token, "POST", "/api/users/role-events", `{}`).Code != 404 {
		t.Fatal("role log mutable via HTTP")
	}
	if e := s.TransferOwnership(owner, OwnershipTransfer{UserID: manager.ID, CurrentPassword: "TestPassword123", ConfirmUsername: "manager"}); e != nil {
		t.Fatal(e)
	}
	if _, e = s.roleEvents(owner, 0); e == nil {
		t.Fatal("former owner retained audit access")
	}
	newEvents, e := s.roleEvents(roleUser(t, s, "manager"), 0)
	if e != nil || newEvents.Events[0].Reason != "ownership_transfer" || newEvents.Events[1].NewRole != "admin" {
		t.Fatal("transfer audit missing", e)
	}
}
