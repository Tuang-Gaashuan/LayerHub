package app

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMemberRenamePreservesIdentityOwnershipAndSession(t *testing.T) {
	s, admin, _ := setup(t)
	if err := s.SaveUser(admin, "maker", "错别字", "MemberPassword123", false); err != nil {
		t.Fatal(err)
	}
	var member User
	if err := s.DB.QueryRow("SELECT id,username,name,admin FROM users WHERE username='maker'").Scan(&member.ID, &member.Username, &member.Name, &member.Admin); err != nil {
		t.Fatal(err)
	}
	p, sp := fixture(t, s, admin)
	jobID := newJob(t, s, member, p, sp, 20)
	private := sp
	private.ID = ""
	private.Label = "个人耗材"
	private.Owner = member.Username
	private.Shared = false
	if err := s.SaveSpool(member, private); err != nil {
		t.Fatal(err)
	}
	login := httptest.NewRecorder()
	s.Handler().ServeHTTP(login, httptest.NewRequest("POST", "/api/login", strings.NewReader(`{"Username":"maker","Password":"MemberPassword123"}`)))
	if login.Code != 200 {
		t.Fatal("member login failed")
	}
	cookie := login.Result().Cookies()[0]
	name := "  正确姓名  "
	if err := s.UpdateMember(admin, MemberUpdate{UserID: member.ID, Name: &name}); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("GET", "/api/workspace", nil)
	request.AddCookie(cookie)
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != 200 {
		t.Fatal("rename invalidated existing member session")
	}
	var snapshot struct {
		User   User    `json:"user"`
		Jobs   []Job   `json:"jobs"`
		Spools []Spool `json:"spools"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.User.ID != member.ID || snapshot.User.Name != "正确姓名" || snapshot.User.Username != "maker" {
		t.Fatal("identity changed or stale name")
	}
	if len(snapshot.Jobs) != 1 || snapshot.Jobs[0].ID != jobID || snapshot.Jobs[0].User != "正确姓名" {
		t.Fatal("task no longer associated with renamed member")
	}
	var owned int
	s.DB.QueryRow("SELECT COUNT(*) FROM spools WHERE owner_id=?", member.ID).Scan(&owned)
	if owned != 1 || len(snapshot.Spools) != 2 {
		t.Fatal("private inventory ownership lost")
	}
	var hash string
	s.DB.QueryRow("SELECT password_hash FROM users WHERE id=?", member.ID).Scan(&hash)
	if !verifyPassword("MemberPassword123", hash) {
		t.Fatal("rename changed password")
	}
	for _, bad := range []string{"  ", strings.Repeat("名", 33), "名字\n换行", admin.Name} {
		if err := s.UpdateMember(admin, MemberUpdate{UserID: member.ID, Name: &bad}); err == nil {
			t.Fatalf("invalid or duplicate name accepted: %q", bad)
		}
	}
	if err := s.UpdateMember(member, MemberUpdate{UserID: admin.ID, Name: &name}); err == nil {
		t.Fatal("non-admin renamed member")
	}
	if err := s.UpdateMember(admin, MemberUpdate{UserID: 9999, Name: &name}); err == nil {
		t.Fatal("missing member accepted")
	}
	disable := true
	combined := "不应保存"
	if err := s.UpdateMember(admin, MemberUpdate{UserID: member.ID, Name: &combined, Disabled: &disable, CurrentPassword: "wrong"}); err == nil {
		t.Fatal("mixed rename bypassed access-setting password")
	}
	var stored string
	s.DB.QueryRow("SELECT name FROM users WHERE id=?", member.ID).Scan(&stored)
	if stored != "正确姓名" {
		t.Fatal("failed request partially renamed member")
	}
	own := "管理者新姓名"
	if err := s.UpdateMember(admin, MemberUpdate{UserID: admin.ID, Name: &own}); err != nil {
		t.Fatal("admin cannot correct own display name", err)
	}
}
