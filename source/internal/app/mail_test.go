package app

import (
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"
)

func readyMail(t *testing.T, s *Service) {
	t.Helper()
	s.Config.MailToken = strings.Repeat("a", 64)
	s.Config.PublicURL = "https://example.test/"
	s.Config.CookieSecure = true
	if _, _, err := s.claimMail(true); err != nil {
		t.Fatal(err)
	}
}
func verification(t *testing.T, s *Service, u User, email string) (*mailMessage, string) {
	t.Helper()
	if err := s.requestEmail(u, email); err != nil {
		t.Fatal(err)
	}
	m, _, err := s.claimMail(true)
	if err != nil || m == nil {
		t.Fatalf("claim: %v, %v", m, err)
	}
	code := regexp.MustCompile(`[0-9]{6}`).FindString(m.Text)
	if code == "" {
		t.Fatal("no OTP")
	}
	return m, code
}
func bound(t *testing.T, s *Service, u User, email string) {
	t.Helper()
	if _, err := s.DB.Exec("INSERT INTO mail_accounts(user_id,email,verified,enabled) VALUES(?,?,?,1)", u.ID, email, time.Now().Unix()); err != nil {
		t.Fatal(err)
	}
}
func TestEmailBindingPrivacyCooldownAndUnlink(t *testing.T) {
	s, u, _ := setup(t)
	if err := s.requestEmail(u, "one@example.com"); err == nil {
		t.Fatal("unconfigured service sent")
	}
	readyMail(t, s)
	m, code := verification(t, s, u, " One@Example.com ")
	if m.To != "one@example.com" {
		t.Fatal(m.To)
	}
	var raw []byte
	var hash string
	s.DB.QueryRow("SELECT body FROM mail_outbox WHERE id=?", m.ID).Scan(&raw)
	s.DB.QueryRow("SELECT hash FROM mail_challenges WHERE user_id=?", u.ID).Scan(&hash)
	if strings.Contains(string(raw), code) || strings.Contains(hash, code) {
		t.Fatal("OTP stored in plaintext")
	}
	if err := s.requestEmail(u, "other@example.com"); err == nil {
		t.Fatal("resend cooldown bypass")
	}
	if err := s.verifyEmail(u, code); err != nil {
		t.Fatal(err)
	}
	snapshot, err := s.Snapshot(u)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(snapshot)
	if strings.Contains(string(b), "one@example.com") {
		t.Fatal("email exposed in workspace")
	}
	status, _ := s.emailStatus(u)
	if status.(map[string]any)["email"] != "one@example.com" {
		t.Fatal("binding missing")
	}
	if err = s.emailPreference(u, false, false); err != nil {
		t.Fatal(err)
	}
	err = s.transaction(func(tx *sql.Tx) error { return s.notifyUser(tx, u.ID, "test-off", "test", "text", time.Now().Unix()) })
	if err != nil {
		t.Fatal(err)
	}
	var count int
	s.DB.QueryRow("SELECT COUNT(*) FROM mail_outbox WHERE kind='notification'").Scan(&count)
	if count != 0 {
		t.Fatal("opt-out received notice")
	}
	if err = s.emailPreference(u, true, false); err != nil {
		t.Fatal(err)
	}
	if err = s.requestEmail(u, "two@example.com"); err != nil {
		t.Fatal(err)
	}
	status, _ = s.emailStatus(u)
	if status.(map[string]any)["email"] != "one@example.com" {
		t.Fatal("pending binding replaced verified address")
	}
	if err = s.emailPreference(u, false, true); err != nil {
		t.Fatal(err)
	}
	m, admins, err := s.claimMail(true)
	if err != nil || m != nil || len(admins) != 0 {
		t.Fatal("unlink retained deliveries", err)
	}
}
func TestEmailWrongCodesExpireAndRateLimit(t *testing.T) {
	s, u, _ := setup(t)
	readyMail(t, s)
	_, code := verification(t, s, u, "one@example.com")
	wrong := "000000"
	if code == wrong {
		wrong = "999999"
	}
	for i := 0; i < 5; i++ {
		if err := s.verifyEmail(u, wrong); err == nil {
			t.Fatal("wrong code accepted")
		}
	}
	if err := s.verifyEmail(u, code); err == nil {
		t.Fatal("locked code accepted")
	}
	s.DB.Exec("UPDATE mail_challenges SET requested=requested-61")
	if err := s.requestEmail(u, "one@example.com"); err != nil {
		t.Fatal(err)
	}
	m, _, err := s.claimMail(true)
	if err != nil || m == nil {
		t.Fatal(err)
	}
	code = regexp.MustCompile(`[0-9]{6}`).FindString(m.Text)
	s.DB.Exec("UPDATE mail_challenges SET expires=0")
	if err = s.verifyEmail(u, code); err == nil {
		t.Fatal("expired code accepted")
	}
	for i := 0; i < 3; i++ {
		s.DB.Exec("UPDATE mail_challenges SET requested=requested-61")
		if err = s.requestEmail(u, "one@example.com"); err != nil {
			t.Fatal(err)
		}
	}
	s.DB.Exec("UPDATE mail_challenges SET requested=requested-61")
	if err = s.requestEmail(u, "one@example.com"); err == nil {
		t.Fatal("hour limit bypass")
	}
	for _, email := range []string{"Name <a@example.com>", "a@example.com\r\nBcc: x@example.com", "not-an-email"} {
		if _, err = normalizeEmail(email); err == nil {
			t.Fatal("invalid address accepted")
		}
	}
}
func TestMailLeasesRetryCancellationAndRelayAuth(t *testing.T) {
	s, u, _ := setup(t)
	readyMail(t, s)
	bound(t, s, u, "one@example.com")
	now := time.Now().Unix()
	if err := s.transaction(func(tx *sql.Tx) error { return s.notifyUser(tx, u.ID, "event", "test", "body", now) }); err != nil {
		t.Fatal(err)
	}
	m, _, err := s.claimMail(true)
	if err != nil || m == nil {
		t.Fatal(err)
	}
	next, _, _ := s.claimMail(true)
	if next != nil {
		t.Fatal("double lease")
	}
	invoke := func(route, auth, payload string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", route, strings.NewReader(payload))
		r.Header.Set("Authorization", auth)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		return w
	}
	if w := invoke("/internal/mail/poll", "", `{"ready":true}`); w.Code != 404 {
		t.Fatal("relay exposed")
	}
	payload := string(mustJSON(map[string]any{"id": m.ID, "lease": "wrong", "sent": true}))
	if w := invoke("/internal/mail/ack", "Bearer "+s.Config.MailToken, payload); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var state string
	s.DB.QueryRow("SELECT state FROM mail_outbox WHERE id=?", m.ID).Scan(&state)
	if state != "leased" {
		t.Fatal("stale lease acknowledged")
	}
	s.DB.Exec("UPDATE mail_outbox SET lease_until=0 WHERE id=?", m.ID)
	next, _, err = s.claimMail(true)
	if err != nil || next == nil || next.Lease == m.Lease {
		t.Fatal("lease not recovered", err)
	}
	if err = s.emailPreference(u, false, true); err != nil {
		t.Fatal(err)
	}
	payload = string(mustJSON(map[string]any{"id": next.ID, "lease": next.Lease}))
	w := invoke("/internal/mail/check", "Bearer "+s.Config.MailToken, payload)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"allowed":false`) {
		t.Fatal("revoked email allowed", w.Body.String())
	}
	next, _, _ = s.claimMail(true)
	if next != nil {
		t.Fatal("cancelled message re-leased")
	}
	// The authenticated browser API still requires a session and CSRF token.
	w = invoke("/api/account/email/request", "", `{"email":"one@example.com"}`)
	if w.Code != 401 {
		t.Fatal(w.Code)
	}
	token := randomID(32)
	s.DB.Exec("INSERT INTO sessions(hash,user_id,expires) VALUES(?,?,?)", tokenHash(token), u.ID, time.Now().Add(time.Hour).Unix())
	r := httptest.NewRequest("POST", "/api/account/email/request", strings.NewReader(`{"email":"one@example.com"}`))
	r.AddCookie(&http.Cookie{Name: "layerhub_sid", Value: token})
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("CSRF bypass")
	}
}
func TestMailEventsBaselineDebounceOwnershipAndBackup(t *testing.T) {
	s, u, driver := setup(t)
	p, sp := fixture(t, s, u)
	id := newJob(t, s, u, p, sp, 5)
	bound(t, s, u, "one@example.com")
	now := time.Now()
	if err := s.collectMail(now); err != nil {
		t.Fatal(err)
	}
	s.DB.Exec("UPDATE jobs SET state='printing',remote_path='mail-test.gcode' WHERE id=?", id)
	driver.mu.Lock()
	driver.statuses[p.ID] = Telemetry{Connected: true, Updated: now, StateUpdated: now, TaskUpdated: now, State: "idle", RawState: "FINISH", File: "mail-test.gcode"}
	driver.mu.Unlock()
	if err := s.collectMail(now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := s.collectMail(now.Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	var count int
	s.DB.QueryRow("SELECT COUNT(*) FROM mail_outbox WHERE kind='notification'").Scan(&count)
	if count != 1 {
		t.Fatal("lost or duplicated finish event", count)
	}
	s.DB.Exec("UPDATE jobs SET state='done' WHERE id=?", id)
	driver.mu.Lock()
	driver.statuses[p.ID] = Telemetry{Connected: false}
	driver.mu.Unlock()
	if err := s.collectMail(now.Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	s.DB.QueryRow("SELECT COUNT(*) FROM mail_outbox").Scan(&count)
	if count != 1 {
		t.Fatal("brief offline notified")
	}
	if err := s.collectMail(now.Add(184 * time.Second)); err != nil {
		t.Fatal(err)
	}
	s.DB.QueryRow("SELECT COUNT(*) FROM mail_outbox").Scan(&count)
	if count != 2 {
		t.Fatal("continuous offline not notified", count)
	}
	if err := scrubMailBackup(s.DB); err != nil {
		t.Fatal(err)
	}
	s.DB.QueryRow("SELECT COUNT(*) FROM mail_outbox").Scan(&count)
	if count != 0 {
		t.Fatal("backup retains messages")
	}
	var enabled bool
	s.DB.QueryRow("SELECT enabled FROM mail_accounts WHERE user_id=?", u.ID).Scan(&enabled)
	if enabled {
		t.Fatal("restored notification opt-in retained")
	}
}

func TestSecureCookieProxyBoundaryAndNASLocalLogin(t *testing.T) {
	s, _, _ := setup(t)
	for _, item := range []struct {
		peer, proto      string
		tls, force, want bool
	}{
		{"127.0.0.1:1234", "", false, false, false},
		{"127.0.0.1:1234", "https", false, false, true},
		{"[::1]:1234", "https", false, false, true},
		{"192.0.2.1:1234", "https", false, false, false},
		{"192.0.2.1:1234", "", true, false, true},
		{"127.0.0.1:1234", "", false, true, true},
	} {
		r := httptest.NewRequest("POST", "/api/login", nil)
		r.RemoteAddr = item.peer
		r.Header.Set("X-Forwarded-Proto", item.proto)
		if item.tls {
			r.TLS = &tls.ConnectionState{}
		}
		s.Config.CookieSecure = item.force
		if s.secureCookie(r) != item.want {
			t.Fatalf("cookie policy mismatch for %s", item.peer)
		}
	}
}
