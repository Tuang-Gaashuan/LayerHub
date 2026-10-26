package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLanguageNegotiationAndFormattedDiagnostics(t *testing.T) {
	for header, want := range map[string]string{"": "zh-CN", "en-US,en;q=0.9": "en", "zh-TW,en;q=0.5": "zh-CN", "en;q=0,zh-CN;q=0.8": "zh-CN", "de,en;q=0.3,zh;q=0.9": "zh-CN", "en;q=bad,zh;q=0.2": "zh-CN"} {
		if got := requestLanguage(header); got != want {
			t.Fatalf("language %q: %s", header, got)
		}
	}
	if got := languageText("en", "用户名或密码错误"); got != "Incorrect username or password" {
		t.Fatal(got)
	}
	if got := languageText("en", "副本 2 与 4 距离不足，请自动排版或增大间距"); got != "Copies 2 and 4 are too close. Auto-arrange or increase spacing." {
		t.Fatal(got)
	}
	if got := languageText("en", "等待物理喷嘴 1 的新规格；需与切片 0.4 mm / Standard 一致"); !strings.Contains(got, "nozzle 1") || !strings.Contains(got, "0.4 mm") {
		t.Fatal(got)
	}
	if got := languageText("zh-CN", "打印已完成"); got != "打印已完成" {
		t.Fatal(got)
	}
}

func TestLanguagePreferenceIsPrivateSelfServiceAndCSRFProtected(t *testing.T) {
	s, owner, _ := setup(t)
	member := notificationMember(t, s, owner)
	token := securityLogin(t, s, owner.Username, "TestPassword123", "Locale test")
	request := func(body, csrf, accept string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/api/account/language", strings.NewReader(body))
		r.Header.Set("X-CSRF-Token", csrf)
		r.Header.Set("Accept-Language", accept)
		r.AddCookie(&http.Cookie{Name: "layerhub_sid", Value: token})
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		return w
	}
	if w := request(`{"language":"en"}`, "", "en"); w.Code != 403 || !strings.Contains(w.Body.String(), "Session validation failed") {
		t.Fatalf("CSRF: %d %s", w.Code, w.Body.String())
	}
	if w := request(`{"language":"fr"}`, s.csrf(token), "en"); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w := request(`{"language":"en","userId":2}`, s.csrf(token), "en"); w.Code != 400 {
		t.Fatal("cross-account field accepted", w.Code)
	}
	if w := request(`{"language":"en"}`, s.csrf(token), "en"); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if accountLanguage(s.DB, owner.ID) != "en" || accountLanguage(s.DB, member.ID) != "zh-CN" {
		t.Fatal("language crossed user boundaries")
	}
	snapshot, err := s.Snapshot(owner)
	if err != nil {
		t.Fatal(err)
	}
	u := snapshot["user"].(User)
	if u.Language != "en" {
		t.Fatal("own language absent")
	}
	raw, _ := json.Marshal(snapshot["members"])
	if strings.Contains(string(raw), `"language"`) {
		t.Fatal("account language leaked into team members")
	}
	var commands, mails int
	s.DB.QueryRow("SELECT COUNT(*) FROM printer_commands").Scan(&commands)
	s.DB.QueryRow("SELECT COUNT(*) FROM mail_outbox").Scan(&mails)
	if commands != 0 || mails != 0 {
		t.Fatal("language change generated commands or mail")
	}
}

func TestLanguageLoginPreferenceAndErrorResponse(t *testing.T) {
	s, owner, _ := setup(t)
	login := func(language, accept, password string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]string{"Username": owner.Username, "Password": password, "Language": language})
		r := httptest.NewRequest("POST", "/api/login", strings.NewReader(string(body)))
		r.Header.Set("Accept-Language", accept)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		return w
	}
	if w := login("", "en-US", "wrong"); w.Code != 401 || !strings.Contains(w.Body.String(), "Incorrect username or password") {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := login("", "en-US", "TestPassword123"); w.Code != 200 || accountLanguage(s.DB, owner.ID) != "en" {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := login("zh-CN", "en-US", "TestPassword123"); w.Code != 200 || accountLanguage(s.DB, owner.ID) != "zh-CN" {
		t.Fatal("explicit choice was ignored")
	}
	if w := login("", "en-US", "TestPassword123"); w.Code != 200 || accountLanguage(s.DB, owner.ID) != "zh-CN" {
		t.Fatal("browser default overwrote saved account preference")
	}
}

func TestEnglishMailPreservesUserContentAndMilestoneDeduplication(t *testing.T) {
	s, u, d := setup(t)
	p, sp := fixture(t, s, u)
	id := newJob(t, s, u, p, sp, 24)
	other := notificationMember(t, s, u)
	bound(t, s, u, "owner@example.com")
	bound(t, s, other, "other@example.com")
	if err := s.setLanguage(u, "en"); err != nil {
		t.Fatal(err)
	}
	filename := "打印已开始 __lh_expr_0__ $1.gcode"
	printerName := "模型工作台"
	if _, err := s.DB.Exec("UPDATE printers SET name=? WHERE id=?", printerName, p.ID); err != nil {
		t.Fatal(err)
	}
	remote := fmt.Sprintf("job-%d-unique.3mf", id)
	if _, err := s.DB.Exec("UPDATE jobs SET filename=?,state='printing',remote_path=? WHERE id=?", filename, remote, id); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	telemetry := Telemetry{Connected: true, Updated: now, StateUpdated: now, TaskUpdated: now, State: "printing", RawState: "RUNNING", File: fmt.Sprintf("job-%d", id), GcodeFile: "/data/Metadata/plate_1.gcode"}
	collect := func() {
		t.Helper()
		d.mu.Lock()
		d.statuses[p.ID] = telemetry
		d.mu.Unlock()
		if err := s.collectMail(now); err != nil {
			t.Fatal(err)
		}
	}
	collect()
	collect()
	m, _, err := s.claimMail(true)
	if err != nil || m == nil {
		t.Fatal("missing start mail", err)
	}
	if !strings.Contains(m.Subject, "Print started") || !strings.Contains(m.Subject, filename) || !strings.Contains(m.Text, "Printer: "+printerName) || !strings.Contains(m.Text, "Job: "+filename) {
		t.Fatalf("incorrect localized message: %s / %s", m.Subject, m.Text)
	}
	// Changing language cannot rewrite/requeue an existing message or notify another member.
	var beforeBody []byte
	s.DB.QueryRow("SELECT body FROM mail_outbox WHERE id=?", m.ID).Scan(&beforeBody)
	if err := s.setLanguage(u, "zh-CN"); err != nil {
		t.Fatal(err)
	}
	var afterBody []byte
	s.DB.QueryRow("SELECT body FROM mail_outbox WHERE id=?", m.ID).Scan(&afterBody)
	if string(beforeBody) != string(afterBody) {
		t.Fatal("queued email was changed")
	}
	if err := s.setLanguage(u, "en"); err != nil {
		t.Fatal(err)
	}
	telemetry.State = "idle"
	telemetry.RawState = "FINISH"
	collect()
	collect()
	var n int
	s.DB.QueryRow("SELECT COUNT(*) FROM mail_outbox WHERE user_id=?", u.ID).Scan(&n)
	if n != 2 {
		t.Fatal("milestones were duplicated", n)
	}
	s.DB.QueryRow("SELECT COUNT(*) FROM mail_outbox WHERE user_id=?", other.ID).Scan(&n)
	if n != 0 {
		t.Fatal("other member received mail")
	}
	var finished []byte
	if err := s.DB.QueryRow("SELECT body FROM mail_outbox WHERE event_key=?", fmt.Sprintf("job:%d:device_finished:user:%d", id, u.ID)).Scan(&finished); err != nil {
		t.Fatal(err)
	}
	plain, err := s.unseal(finished)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plain, "Print complete") || !strings.Contains(plain, "Inventory is not deducted automatically") {
		t.Fatal("finish mail was not localized")
	}
	for _, category := range []string{"started", "completed"} {
		s.DB.QueryRow("SELECT COUNT(*) FROM user_notifications WHERE user_id=? AND category=?", u.ID, category).Scan(&n)
		if n != 1 {
			t.Fatal("category changed with language", category, n)
		}
	}
}

func TestEnglishVerificationEmail(t *testing.T) {
	s, u, _ := setup(t)
	readyMail(t, s)
	if err := s.setLanguage(u, "en"); err != nil {
		t.Fatal(err)
	}
	m, code := verification(t, s, u, "owner@example.com")
	if m.Subject != "LayerHub email verification code" || !strings.Contains(m.Text, "Valid for 10 minutes") || !strings.Contains(m.Text, code) {
		t.Fatal("verification not localized")
	}
}
