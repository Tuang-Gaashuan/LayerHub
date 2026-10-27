package app

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeDevice struct {
	mu       sync.Mutex
	statuses map[int64]Telemetry
	uploads  int
	hold     chan struct{}
	startErr bool
	lastCode string
}

func (f *fakeDevice) Reconcile(_ context.Context, printers []Printer) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.statuses == nil {
		f.statuses = map[int64]Telemetry{}
	}
	for _, p := range printers {
		f.statuses[p.ID] = Telemetry{Connected: p.Enabled, Updated: time.Now(), State: "idle"}
		f.lastCode = p.Code
	}
}
func (f *fakeDevice) Status(id int64) Telemetry {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.statuses[id]
}
func (f *fakeDevice) Upload(ctx context.Context, p Printer, path, remote string, progress func(int64)) error {
	f.mu.Lock()
	f.uploads++
	f.mu.Unlock()
	if f.hold != nil {
		select {
		case <-f.hold:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(io.Discard, file)
	return err
}
func (f *fakeDevice) Start(_ context.Context, p Printer, remote string) error {
	if f.startErr {
		return context.DeadlineExceeded
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.statuses[p.ID] = Telemetry{Connected: true, Updated: time.Now(), State: "printing", File: remote}
	return nil
}
func (f *fakeDevice) Close() {}
func setup(t testing.TB) (*Service, User, *fakeDevice) {
	t.Helper()
	driver := &fakeDevice{}
	web := t.TempDir()
	os.WriteFile(filepath.Join(web, "index.html"), []byte("LayerHub"), 0600)
	s, err := Open(Config{DataDir: t.TempDir(), WebDir: web, AdminUser: "admin", AdminPassword: "TestPassword123", DeviceMode: "simulated", EnablePrintStart: true, DisableBackgroundEvents: true}, driver)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	var user User
	if err = s.DB.QueryRow("SELECT id,username,name,admin FROM users WHERE username='admin'").Scan(&user.ID, &user.Username, &user.Name, &user.Admin); err != nil {
		t.Fatal(err)
	}
	return s, user, driver
}
func fixture(t testing.TB, s *Service, u User) (Printer, Spool) {
	t.Helper()
	err := s.SavePrinter(u, PrinterInput{Name: "P1S", IP: "192.0.2.1", Serial: "TEST-001", ModelID: "p1s", Configuration: "ams", Code: "test-device-code"})
	if err != nil {
		t.Fatal(err)
	}
	ps, _ := s.printers()
	sp := Spool{Label: "公共白", Brand: "Test", Material: "PLA", ColorName: "白", Color: "#eeeeee", Capacity: 1000, Remaining: 1000, Owner: "team", State: "available"}
	if err = s.SaveSpool(u, sp); err != nil {
		t.Fatal(err)
	}
	ss, _ := readSpools(s.DB)
	return ps[0], ss[0]
}
func newJob(t testing.TB, s *Service, u User, p Printer, sp Spool, grams int) int64 {
	t.Helper()
	path := filepath.Join(s.Config.DataDir, "uploads", randomID(8)+".gcode")
	payload := []byte("; local test only\r\n")
	if err := os.WriteFile(path, payload, 0600); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(payload)
	id, err := s.CreateJob(u, Schedule{Printer: p.ID, Estimate: 20, Grams: grams, SpoolID: sp.ID}, "same-name.gcode", path, hex.EncodeToString(hash[:]), "", int64(len(payload)))
	if err != nil {
		t.Fatal(err)
	}
	return id
}
func waitState(t testing.TB, s *Service, id int64, state string) {
	t.Helper()
	until := time.Now().Add(3 * time.Second)
	for time.Now().Before(until) {
		var got string
		if err := s.DB.QueryRow("SELECT state FROM jobs WHERE id=?", id).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got == state {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("task %d did not reach %s", id, state)
}
func TestConcurrentReservationsCannotOverspend(t *testing.T) {
	s, u, _ := setup(t)
	p, sp := fixture(t, s, u)
	sp.Remaining = 100
	if err := s.SaveSpool(u, sp); err != nil {
		t.Fatal(err)
	}
	var accepted atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.CreateJob(u, Schedule{Printer: p.ID, Estimate: 10, Grams: 10, SpoolID: sp.ID}, "part.gcode", "unused", strings.Repeat("a", 64), "", 1); err == nil {
				accepted.Add(1)
			}
		}()
	}
	wg.Wait()
	if accepted.Load() != 10 {
		t.Fatalf("accepted %d tasks, want exactly 10", accepted.Load())
	}
	jobs, _ := readJobs(s.DB)
	if reserve(jobs, sp.ID, 0) != 100 {
		t.Fatal("reservation invariant failed")
	}
}
func TestQueueOwnershipAndOneInFlightTransfer(t *testing.T) {
	s, u, d := setup(t)
	p, sp := fixture(t, s, u)
	first := newJob(t, s, u, p, sp, 20)
	second := newJob(t, s, u, p, sp, 20)
	if err := s.SendJob(u, second); err == nil {
		t.Fatal("non-head task bypassed queue")
	}
	other := User{ID: 999, Username: "other", Name: "other"}
	if err := s.SendJob(other, first); err == nil {
		t.Fatal("other user could send task")
	}
	d.hold = make(chan struct{})
	var accepted atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if s.SendJob(u, first) == nil {
				accepted.Add(1)
			}
		}()
	}
	wg.Wait()
	close(d.hold)
	waitState(t, s, first, "assigned")
	if accepted.Load() != 1 {
		t.Fatalf("accepted %d simultaneous sends", accepted.Load())
	}
	if err := s.StartJob(other, first); err == nil {
		t.Fatal("cross-user start permitted")
	}
	if err := s.StartJob(u, first); err != nil {
		t.Fatal(err)
	}
	waitState(t, s, first, "printing")
	if err := s.SendJob(u, second); err == nil {
		t.Fatal("occupied device allowed a second transfer")
	}
	if err := s.CompleteJob(u, first, 18); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteJob(u, first, 18); err == nil {
		t.Fatal("duplicate completion deducted twice")
	}
	var remaining int
	s.DB.QueryRow("SELECT remaining FROM spools WHERE id=?", sp.ID).Scan(&remaining)
	if remaining != 982 {
		t.Fatalf("remaining=%d", remaining)
	}
}
func TestStartNeedsDeviceConfirmation(t *testing.T) {
	s, u, d := setup(t)
	p, sp := fixture(t, s, u)
	id := newJob(t, s, u, p, sp, 20)
	if err := s.SendJob(u, id); err != nil {
		t.Fatal(err)
	}
	waitState(t, s, id, "assigned")
	d.startErr = true
	if err := s.StartJob(u, id); err != nil {
		t.Fatal(err)
	}
	waitState(t, s, id, "assigned")
	var message string
	s.DB.QueryRow("SELECT error FROM jobs WHERE id=?", id).Scan(&message)
	if message == "" {
		t.Fatal("unconfirmed start did not report uncertainty")
	}
}
func TestPrivateInventoryAndSlotUniqueness(t *testing.T) {
	s, u, _ := setup(t)
	p, sp := fixture(t, s, u)
	if err := s.SaveUser(u, "member", "成员", "MemberPassword123", false); err != nil {
		t.Fatal(err)
	}
	var member User
	s.DB.QueryRow("SELECT id,username,name,admin FROM users WHERE username='member'").Scan(&member.ID, &member.Username, &member.Name, &member.Admin)
	private := Spool{Label: "私有卷", Brand: "Test", Material: "PLA", ColorName: "红", Color: "#ff0000", Capacity: 1000, Remaining: 200, Owner: member.Username, State: "available", Mount: &Mount{p.ID, "2"}}
	if err := s.SaveSpool(member, private); err != nil {
		t.Fatal(err)
	}
	all, _ := readSpools(s.DB)
	snapshot, err := s.Snapshot(u)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot["spools"].([]Spool)) != 1 {
		t.Fatal("admin saw another member's private inventory")
	}
	if len(snapshot["occupiedSlots"].([]Mount)) != 1 {
		t.Fatal("private mount occupancy missing")
	}
	private = all[1]
	if err := s.SaveSpool(u, private); err == nil {
		t.Fatal("admin edited private spool")
	}
	sp.Mount = &Mount{p.ID, "2"}
	if err := s.SaveSpool(u, sp); err == nil {
		t.Fatal("occupied slot accepted duplicate spool")
	}
	if err := s.SavePrinter(u, PrinterInput{ID: p.ID, Configuration: "bare"}); err == nil {
		t.Fatal("configuration invalidated mounted spool")
	}
	id := newJob(t, s, u, p, sp, 10)
	if err := s.DeleteJob(member, id); err == nil {
		t.Fatal("cross-user deletion allowed")
	}
}
func TestPersistenceAndEncryptedDeviceCredentials(t *testing.T) {
	root := t.TempDir()
	cfg := Config{DataDir: root, WebDir: t.TempDir(), AdminPassword: "TestPassword123", DeviceMode: "simulated", DisableBackgroundEvents: true}
	d := &fakeDevice{}
	s, err := Open(cfg, d)
	if err != nil {
		t.Fatal(err)
	}
	var u User
	s.DB.QueryRow("SELECT id,username,name,admin FROM users LIMIT 1").Scan(&u.ID, &u.Username, &u.Name, &u.Admin)
	p, sp := fixture(t, s, u)
	var encrypted []byte
	s.DB.QueryRow("SELECT code FROM printers WHERE id=?", p.ID).Scan(&encrypted)
	if bytes.Contains(encrypted, []byte("test-device-code")) {
		t.Fatal("plaintext device credential in database")
	}
	id := newJob(t, s, u, p, sp, 5)
	s.Close()
	next := &fakeDevice{}
	s, err = Open(cfg, next)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var count int
	s.DB.QueryRow("SELECT COUNT(*) FROM jobs WHERE id=?", id).Scan(&count)
	if count != 1 || next.lastCode != "test-device-code" {
		t.Fatal("restart lost task or device credential")
	}
}
func TestHTTPStreamingBinaryUploadAndCSRF(t *testing.T) {
	s, u, _ := setup(t)
	p, sp := fixture(t, s, u)
	server := httptest.NewServer(s.Handler())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginBody := strings.NewReader(`{"Username":"admin","Password":"TestPassword123"}`)
	response, err := client.Post(server.URL+"/api/login", "application/json", loginBody)
	if err != nil {
		t.Fatal(err)
	}
	var login map[string]any
	json.NewDecoder(response.Body).Decode(&login)
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal("login failed")
	}
	csrf := login["csrf"].(string)
	req, _ := http.NewRequest("POST", server.URL+"/api/jobs/delete", strings.NewReader(`{"jobId":1}`))
	req.Header.Set("Content-Type", "application/json")
	response, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 403 {
		t.Fatal("mutation without CSRF accepted")
	}
	entries := intakeFixture()
	entries["Metadata/binary-test.bin"] = string(append(bytes.Repeat([]byte{0, 13, 10, 127, 255, 65}, 40000), 13, 10))
	var archive bytes.Buffer
	z := zip.NewWriter(&archive)
	for name, content := range entries {
		f, e := z.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
		if e != nil {
			t.Fatal(e)
		}
		if _, e = f.Write([]byte(content)); e != nil {
			t.Fatal(e)
		}
	}
	if e := z.Close(); e != nil {
		t.Fatal(e)
	}
	payload := archive.Bytes()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, _ := writer.CreateFormFile("file", "binary.gcode.3mf")
	part.Write(payload)
	schedule, _ := json.Marshal(Schedule{Printer: p.ID, Estimate: 20, Grams: 15, SpoolID: sp.ID})
	writer.WriteField("schedule", string(schedule))
	writer.Close()
	req, _ = http.NewRequest("POST", server.URL+"/api/upload", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-CSRF-Token", csrf)
	response, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	json.NewDecoder(response.Body).Decode(&result)
	response.Body.Close()
	if response.StatusCode != 201 {
		t.Fatalf("upload failed: %v", result)
	}
	var path string
	s.DB.QueryRow("SELECT path FROM jobs WHERE id=?", int64(result["jobId"].(float64))).Scan(&path)
	stored, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(stored, payload) {
		t.Fatal("binary upload changed payload bytes")
	}
	snapshot, err := s.Snapshot(u)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(snapshot)
	if bytes.Contains(encoded, []byte("test-device-code")) {
		t.Fatal("API leaked access code")
	}
	req, _ = http.NewRequest("POST", server.URL+"/api/login", strings.NewReader(`{}`))
	req.Header.Set("Origin", "https://attacker.example")
	response, _ = client.Do(req)
	response.Body.Close()
	if response.StatusCode != 403 {
		t.Fatal("cross-site login was accepted")
	}
	response, err = client.Get(server.URL + "/%2e%2e/data/master.key")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode == 200 {
		t.Fatal("static route exposed private data")
	}
}
func BenchmarkWorkspaceSnapshot(b *testing.B) {
	s, u, _ := setup(b)
	p, sp := fixture(b, s, u)
	for i := 0; i < 100; i++ {
		if _, err := s.CreateJob(u, Schedule{Printer: p.ID, Estimate: 20, Grams: 1, SpoolID: sp.ID}, "part.gcode", "unused", strings.Repeat("a", 64), "", 1); err != nil {
			b.Fatal(err)
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		snapshot, err := s.Snapshot(u)
		if err != nil {
			b.Fatal(err)
		}
		if _, err = json.Marshal(snapshot); err != nil {
			b.Fatal(err)
		}
	}
}
