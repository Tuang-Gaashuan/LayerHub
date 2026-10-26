package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode"
)

func diagnosticByID(t *testing.T, r DiagnosticReport, id string) DiagnosticCheck {
	t.Helper()
	for _, c := range r.Checks {
		if c.ID == id {
			return c
		}
	}
	t.Fatal("missing check", id)
	return DiagnosticCheck{}
}
func TestDoctorBeforeStartupDoesNotCreateWorkspaceOrExposeConfiguration(t *testing.T) {
	root := t.TempDir()
	data := filepath.Join(root, "missing-private-workspace")
	c := Config{Address: "not-an-address", DataDir: data, WebDir: filepath.Join(root, "private-web"), DeviceMode: "invalid", MailToken: "DO-NOT-EXPORT-SECRET", PublicURL: "https://private-user:private-password@private.invalid/?secret=hidden"}
	r := LocalDiagnostics(c, false)
	if _, e := os.Stat(data); !os.IsNotExist(e) {
		t.Fatal("doctor created workspace")
	}
	for _, id := range []string{"listener", "web_assets", "device_mode", "mail_config", "public_url"} {
		if diagnosticByID(t, r, id).Status != "error" {
			t.Fatal("misconfiguration not detected", id)
		}
	}
	if diagnosticByID(t, r, "database").Status != "unknown" {
		t.Fatal("missing database claimed healthy")
	}
	raw, _ := json.Marshal(r)
	for _, secret := range []string{root, c.MailToken, "private-user", "private-password", "private.invalid", "secret=hidden"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("diagnostic leaked configuration")
		}
	}
}
func TestDoctorTemporaryProbeAndDatabaseHeaderAreExplicitAndNonDestructive(t *testing.T) {
	root := t.TempDir()
	db := filepath.Join(root, "layerhub.sqlite")
	payload := []byte("SQLite format 3\x00unchanged-test-payload")
	if e := os.WriteFile(db, payload, 0600); e != nil {
		t.Fatal(e)
	}
	c := Config{DataDir: root, WebDir: root, Address: "127.0.0.1:5181", DeviceMode: "offline"}
	if diagnosticByID(t, LocalDiagnostics(c, false), "data_directory").Status != "unknown" {
		t.Fatal("read-only inspection claimed a write probe")
	}
	r := LocalDiagnostics(c, true)
	if diagnosticByID(t, r, "data_directory").Status != "ok" || diagnosticByID(t, r, "database").Status != "ok" {
		t.Fatal("probe/header check failed")
	}
	raw, _ := os.ReadFile(db)
	if string(raw) != string(payload) {
		t.Fatal("database mutated")
	}
	files, _ := os.ReadDir(root)
	if len(files) != 1 || files[0].Name() != "layerhub.sqlite" {
		t.Fatal("temporary probe was not cleaned")
	}
	os.WriteFile(db, []byte("not a database"), 0600)
	if diagnosticByID(t, LocalDiagnostics(c, false), "database").Status != "error" {
		t.Fatal("corrupt header accepted")
	}
}
func TestDiagnosticsReadsExistingTelemetryWithoutDispatchAndDoesNotTrustSimulation(t *testing.T) {
	s, u, f := setup(t)
	p, _ := fixture(t, s, u)
	s.Config.PublicURL = "https://private.invalid/"
	s.Config.MailToken = strings.Repeat("private-token", 4)
	s.DB.Exec("INSERT INTO mail_worker(id,seen,ready) VALUES(1,?,1)", time.Now().Unix())
	for _, tc := range []struct {
		mode      string
		stamp     time.Time
		connected bool
		want      string
	}{{"live", time.Now(), true, "ok"}, {"live", time.Now().Add(-2 * time.Minute), true, "warning"}, {"live", time.Now().Add(time.Minute), true, "warning"}, {"live", time.Now(), false, "warning"}, {"simulated", time.Now(), true, "unknown"}, {"offline", time.Now(), true, "unknown"}} {
		s.Config.DeviceMode = tc.mode
		f.mu.Lock()
		f.statuses[p.ID] = Telemetry{Connected: tc.connected, Updated: tc.stamp, File: "PRIVATE-MODEL.gcode", Error: "PRIVATE-RAW-ERROR"}
		f.mu.Unlock()
		r, e := s.Diagnostics(context.Background(), u)
		if e != nil {
			t.Fatal(e)
		}
		if diagnosticByID(t, r, "printer").Status != tc.want {
			t.Fatal("invalid freshness or simulation result", tc.mode, tc.want)
		}
		if diagnosticByID(t, r, "mail_worker").Status != "ok" {
			t.Fatal("fresh worker not recognized")
		}
		raw, _ := json.Marshal(r)
		for _, secret := range []string{p.IP, p.Serial, p.Code, p.Name, "PRIVATE-MODEL.gcode", "PRIVATE-RAW-ERROR", s.Config.PublicURL, s.Config.MailToken, s.Config.DataDir} {
			if secret != "" && strings.Contains(string(raw), secret) {
				t.Fatal("runtime report leaked private data")
			}
		}
	}
	var commands, mails int
	s.DB.QueryRow("SELECT COUNT(*) FROM printer_commands").Scan(&commands)
	s.DB.QueryRow("SELECT COUNT(*) FROM mail_outbox").Scan(&mails)
	if commands != 0 || mails != 0 || f.uploads != 0 {
		t.Fatal("diagnostics generated a write or device action")
	}
	s.DB.Exec("UPDATE mail_worker SET seen=?", time.Now().Add(time.Minute).Unix())
	r, _ := s.Diagnostics(context.Background(), u)
	if diagnosticByID(t, r, "mail_worker").Status != "warning" {
		t.Fatal("future heartbeat accepted")
	}
	f.mu.Lock()
	f.statuses[p.ID] = Telemetry{Connected: true, Updated: time.Now(), PrintError: 1, Equipment: Equipment{ControlIssue: "authorization_required"}}
	f.mu.Unlock()
	s.Config.DeviceMode = "live"
	r, e := s.Diagnostics(context.Background(), u)
	if e != nil {
		t.Fatal(e)
	}
	if diagnosticByID(t, r, "printer").Status != "ok" || diagnosticByID(t, r, "printer_authorization").Status != "warning" || diagnosticByID(t, r, "printer_alert").Status != "warning" {
		t.Fatal("connection was confused with control authorization")
	}
	english := LocalizeDiagnosticReport(r, "en")
	for _, c := range english.Checks {
		for _, value := range []string{c.Label, c.Detail, c.Fix} {
			for _, ch := range value {
				if unicode.Is(unicode.Han, ch) {
					t.Fatal("untranslated diagnostic", c.ID)
				}
			}
		}
	}
}
func TestDiagnosticsEndpointRequiresCurrentAdminAndRejectsWrites(t *testing.T) {
	s, owner, _ := setup(t)
	member := notificationMember(t, s, owner)
	ownerToken := securityLogin(t, s, owner.Username, "TestPassword123", "Diagnostic test")
	memberToken := securityLogin(t, s, member.Username, "MemberPassword123", "Diagnostic test")
	call := func(method, token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/api/diagnostics", nil)
		r.Header.Set("Accept-Language", "en")
		if token != "" {
			r.AddCookie(&http.Cookie{Name: "layerhub_sid", Value: token})
			r.Header.Set("X-CSRF-Token", s.csrf(token))
		}
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		return w
	}
	if w := call("GET", ""); w.Code != 401 {
		t.Fatal("anonymous diagnostics", w.Code)
	}
	if w := call("GET", memberToken); w.Code != 403 {
		t.Fatal("member diagnostics", w.Code)
	}
	if w := call("POST", ownerToken); w.Code != 404 {
		t.Fatal("diagnostic write accepted", w.Code)
	}
	if w := call("GET", ownerToken); w.Code != 200 || !strings.Contains(w.Body.String(), `"schema":1`) {
		t.Fatal("admin diagnostics failed", w.Code)
	}
	// Re-check role from the database rather than trusting the caller's old flag.
	old := member
	old.Admin = true
	if _, e := s.Diagnostics(context.Background(), old); e == nil {
		t.Fatal("stale admin flag trusted")
	}
}

func TestDiagnosticsCookieEvidenceUsesExistingTrustedRequestPolicy(t *testing.T) {
	s, owner, _ := setup(t)
	s.Config.PublicURL = "https://example.invalid/"
	token := securityLogin(t, s, owner.Username, "TestPassword123", "Diagnostic cookie test")
	for _, tc := range []struct{ peer, scheme, want string }{{"127.0.0.1:1234", "https", "ok"}, {"192.0.2.10:1234", "https", "unknown"}, {"127.0.0.1:1234", "http", "unknown"}} {
		r := httptest.NewRequest("GET", "/api/diagnostics", nil)
		r.RemoteAddr = tc.peer
		r.Header.Set("X-Forwarded-Proto", tc.scheme)
		r.AddCookie(&http.Cookie{Name: "layerhub_sid", Value: token})
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		var report DiagnosticReport
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &report) != nil {
			t.Fatal("diagnostic failed", w.Code)
		}
		if diagnosticByID(t, report, "secure_cookie").Status != tc.want {
			t.Fatal("untrusted cookie evidence", tc.peer, tc.scheme)
		}
	}
}
