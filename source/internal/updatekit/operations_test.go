package updatekit

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func sampleWorkspace() Workspace {
	var w Workspace
	w.User.Owner = true
	w.Printers = []Printer{{ID: 1, Enabled: true, Connected: true, Updated: 1000, RawState: "FINISH", Percent: 100, File: "job-6"}}
	return w
}
func TestAllowRunningIsExplicitAndRequiresKnownStableTask(t *testing.T) {
	w := sampleWorkspace()
	w.Printers[0].RawState = "RUNNING"
	w.Printers[0].Percent = 40
	json.Unmarshal([]byte(`[{"id":6,"printer":1,"state":"printing"}]`), &w.Jobs)
	if guardWorkspace(w, 1001) == nil {
		t.Fatal("running allowed by default")
	}
	if e := guardWorkspaceMode(w, 1001, true); e != nil {
		t.Fatal(e)
	}
	w.Printers[0].File = "job-7"
	if guardWorkspaceMode(w, 1001, true) == nil {
		t.Fatal("unknown task allowed")
	}
	w.Printers[0].File = "job-6"
	w.Jobs[0].State = "starting"
	if guardWorkspaceMode(w, 1001, true) == nil {
		t.Fatal("inflight start allowed")
	}
	w.Jobs[0].State = "printing"
	w.Printers[0].PrintError = 1
	if guardWorkspaceMode(w, 1001, true) == nil {
		t.Fatal("alert ignored")
	}
}
func TestWorkspaceGuards(t *testing.T) {
	for _, state := range []string{"FINISH", "IDLE", "READY"} {
		w := sampleWorkspace()
		w.Printers[0].RawState = state
		if e := guardWorkspace(w, 1001); e != nil {
			t.Fatal(e)
		}
	}
	for _, test := range []struct {
		name   string
		change func(*Printer)
	}{{"offline", func(p *Printer) { p.Connected = false }}, {"stale", func(p *Printer) { p.Updated = 900 }}, {"future", func(p *Printer) { p.Updated = 1002 }}, {"running", func(p *Printer) { p.RawState = "RUNNING" }}} {
		t.Run(test.name, func(t *testing.T) {
			w := sampleWorkspace()
			test.change(&w.Printers[0])
			if guardWorkspace(w, 1001) == nil {
				t.Fatal("unsafe printer accepted")
			}
		})
	}
}

func TestJobGuardsRequireMatchingFinishedFile(t *testing.T) {
	for _, status := range []string{"uploading", "starting", "paused", "stopped", "checking", "printing"} {
		t.Run(status, func(t *testing.T) {
			w := sampleWorkspace()
			raw := `[{"id":6,"printer":1,"state":"` + status + `"}]`
			if e := json.Unmarshal([]byte(raw), &w.Jobs); e != nil {
				t.Fatal(e)
			}
			if status != "printing" {
				if guardWorkspace(w, 1001) == nil {
					t.Fatal("active task accepted")
				}
				return
			}
			if e := guardWorkspace(w, 1001); e != nil {
				t.Fatal(e)
			}
			w.Printers[0].File = "job-7"
			if guardWorkspace(w, 1001) == nil {
				t.Fatal("wrong completed file accepted")
			}
			w.Printers[0].File = "job-6"
			w.Printers[0].Percent = 99
			if guardWorkspace(w, 1001) == nil {
				t.Fatal("incomplete progress accepted")
			}
		})
	}
}
func TestAPIOriginAndAuthGuards(t *testing.T) {
	for _, base := range []string{"http://example.com:5181", "http://user@127.0.0.1:5181", "http://127.0.0.1:5181/path", "https://127.0.0.1:5181"} {
		if _, e := newAPI(base); e == nil {
			t.Fatal(base)
		}
	}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/login":
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "fixture", Secure: true, HttpOnly: true})
			io.WriteString(w, `{"csrf":"fixture-csrf"}`)
		case "/api/workspace":
			if r.Header.Get("Cookie") != "session=fixture" {
				t.Error("session missing")
			}
			json.NewEncoder(w).Encode(sampleWorkspace())
		case "/api/backup/status":
			io.WriteString(w, `{"pending":true}`)
		default:
			t.Fatalf("unexpected mutation %s", r.URL.Path)
		}
	}))
	defer server.Close()
	a, e := newAPI(server.URL)
	if e != nil {
		t.Fatal(e)
	}
	if e = a.login("owner", "not-logged"); e != nil {
		t.Fatal(e)
	}
	if _, e = a.guard(1001); e == nil {
		t.Fatal("pending restore accepted")
	}
	if requests != 3 {
		t.Fatal(requests)
	}
}
func TestCleanEnvironmentNeverEvaluatesShell(t *testing.T) {
	path := filepath.Join(t.TempDir(), "env")
	os.WriteFile(path, []byte("LAYERHUB_DATA_DIR='/var/lib/layerhub/data'\nLAYERHUB_ADDRESS=127.0.0.1:5181\nLAYERHUB_MAIL_TOKEN=PRIVATE_CANARY\nIGNORED=$(touch should-not-exist)\n"), 0o600)
	values, e := cleanEnvFile(path)
	if e != nil || values["LAYERHUB_DATA_DIR"] != "/var/lib/layerhub/data" || len(values) != 2 {
		t.Fatal(values, e)
	}
}
func TestCLIHelpAndInvalidInput(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"download", "--help"}, {"nas", "--help"}, {"cloud", "--help"}} {
		var out bytes.Buffer
		if e := Run(args, strings.NewReader(""), &out); e != nil || out.Len() == 0 {
			t.Fatal(args, e)
		}
	}
	if e := Run([]string{"download", "--channel", "other"}, strings.NewReader(""), io.Discard); e == nil {
		t.Fatal("invalid channel")
	}
	if confirm(bufio.NewReader(strings.NewReader("no\n")), io.Discard, "test") == nil {
		t.Fatal("cancellation ignored")
	}
}
func TestNginxTransformationPreservesTLSAndAPI(t *testing.T) {
	original := "server { listen 443 ssl; ssl_certificate /private/cert; location ^~ /internal/ { return 404; } location / {\n proxy_pass http://127.0.0.1:15181;\n error_page 502 504 =503 @nas_offline;\n } location @nas_offline { return 503 '{\"ok\":false}'; } }"
	web := "/usr/share/layerhub-gateway/releases/new/web"
	next, e := cloudConfig(original, web)
	if e != nil {
		t.Fatal(e)
	}
	for _, part := range []string{"ssl_certificate /private/cert", "location ^~ /internal/ { return 404; }", "location /api/ {", "location = /healthz {", "root " + web} {
		if !strings.Contains(next, part) {
			t.Fatal(part)
		}
	}
	again, e := cloudConfig(next, "/usr/share/layerhub-gateway/releases/next/web")
	if e != nil || strings.Count(again, "location /api/") != 1 {
		t.Fatal(e, again)
	}
	if _, e = cloudConfig("server { location / { return 200 'unrelated'; } }", web); e == nil {
		t.Fatal("unmanaged site overwritten")
	}
}
func TestNginxOldSourceRedirectRemoved(t *testing.T) {
	old := "server { location = /source/layerhub-source.tar.gz { return 302 https://github.com/" + Repository + "/releases/download/v0.1.0-beta/source.tar.gz; } location / {root /usr/share/layerhub-gateway/releases/old/web;} location /api/ {proxy_pass http://127.0.0.1:15181;} location = /healthz {proxy_pass http://127.0.0.1:15181;} }"
	next, e := cloudConfig(old, "/usr/share/layerhub-gateway/releases/new/web")
	if e != nil || strings.Contains(next, "return 302") {
		t.Fatal(next, e)
	}
}

func TestNginxCustomSecurityRulesAreNotSilentlyRemoved(t *testing.T) {
	original := `server { add_header Strict-Transport-Security "max-age=31536000" always; location / { proxy_pass http://127.0.0.1:15181; auth_basic "Private team"; } }`
	if _, e := cloudConfig(original, "/usr/share/layerhub-gateway/releases/new/web"); e == nil {
		t.Fatal("custom authentication removed")
	}
	original = `server { location / { root /usr/share/layerhub-gateway/releases/old/web; add_header Content-Security-Policy "default-src 'self'; frame-ancestors 'none'" always; } location /api/ {proxy_pass http://127.0.0.1:15181;} location = /healthz {proxy_pass http://127.0.0.1:15181;} }`
	next, e := cloudConfig(original, "/usr/share/layerhub-gateway/releases/new/web")
	if e != nil || !strings.Contains(next, `add_header Content-Security-Policy "default-src 'self'; frame-ancestors 'none'" always;`) {
		t.Fatal(next, e)
	}
}
func TestStateHelperUsesReadOnlyDatabaseAndColdBackup(t *testing.T) {
	python := "python3"
	if runtime.GOOS == "windows" {
		python = "python"
	}
	if _, e := exec.LookPath(python); e != nil {
		python = "python"
		if _, e = exec.LookPath(python); e != nil {
			t.Skip("Python 3 unavailable")
		}
	}
	root := t.TempDir()
	data := filepath.Join(root, "data")
	os.Mkdir(data, 0o700)
	setup := `import sqlite3,sys,pathlib
p=pathlib.Path(sys.argv[1]);d=sqlite3.connect(p/'layerhub.sqlite');d.execute('CREATE TABLE jobs(id INTEGER,printer_id INTEGER,state TEXT)');d.execute('CREATE TABLE printer_commands(state TEXT)');d.commit();d.close();(p/'master.key').write_text('private-test-fixture');(p/'master.key').chmod(0o600)
`
	if raw, e := exec.Command(python, "-c", setup, data).CombinedOutput(); e != nil {
		t.Fatal(string(raw), e)
	}
	db := filepath.Join(data, "layerhub.sqlite")
	before, _ := hashFile(db)
	raw, e := exec.Command(python, "-c", stateHelper, "inspect", data).CombinedOutput()
	if e != nil {
		t.Fatal(string(raw), e)
	}
	after, _ := hashFile(db)
	if before != after {
		t.Fatal("inspect changed database")
	}
	env := filepath.Join(root, "environment")
	os.WriteFile(env, []byte("PRIVATE_TEST_FIXTURE=1\n"), 0o600)
	backup := filepath.Join(root, "backups", "first")
	raw, e = exec.Command(python, "-c", stateHelper, "backup", data, backup, env).CombinedOutput()
	if e != nil {
		t.Fatal(string(raw), e)
	}
	var result map[string]any
	if json.Unmarshal(raw, &result) != nil || result["sqlite_integrity"] != "ok" {
		t.Fatal(string(raw))
	}
	if _, e = os.Stat(filepath.Join(backup, "workspace.tar.gz")); e != nil {
		t.Fatal(e)
	}
	os.WriteFile(data+".restore.json", []byte("{}"), 0o600)
	if e = exec.Command(python, "-c", stateHelper, "inspect", data).Run(); e == nil {
		t.Fatal("restore marker ignored")
	}
	_ = time.Now()
}
