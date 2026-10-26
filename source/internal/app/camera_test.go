package app

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type cameraFake struct {
	*fakeDevice
	frame CameraFrame
	fail  bool
	calls atomic.Int32
}

func (f *cameraFake) CameraFrame(_ context.Context, p Printer) (CameraFrame, error) {
	f.calls.Add(1)
	if f.fail {
		return CameraFrame{}, errors.New("sensitive-internal-camera-url")
	}
	return f.frame, nil
}

func TestCameraHTTPRequiresSessionAndDoesNotExposeDriverErrors(t *testing.T) {
	s, u, d := setup(t)
	p, _ := fixture(t, s, u)
	s.Config.DeviceMode = "live"
	var data bytes.Buffer
	jpeg.Encode(&data, image.NewRGBA(image.Rect(0, 0, 16, 9)), nil)
	driver := &cameraFake{fakeDevice: d, frame: CameraFrame{JPEG: data.Bytes(), Updated: time.Now()}}
	s.Device = driver
	server := httptest.NewServer(s.Handler())
	defer server.Close()
	path := "/api/printers/camera?printerId=1"
	response, err := http.Get(server.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 401 || driver.calls.Load() != 0 {
		t.Fatal("anonymous camera access")
	}
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	response, err = client.Post(server.URL+"/api/login", "application/json", strings.NewReader(`{"Username":"admin","Password":"TestPassword123"}`))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal("test login failed")
	}
	get := func(path string) (*http.Response, []byte) {
		t.Helper()
		r, e := client.Get(server.URL + path)
		if e != nil {
			t.Fatal(e)
		}
		b, _ := io.ReadAll(r.Body)
		r.Body.Close()
		return r, b
	}
	for _, entry := range []struct {
		path   string
		status int
	}{{"/api/printers/camera?printerId=bad", 400}, {"/api/printers/camera?printerId=999", 404}} {
		r, _ := get(entry.path)
		if r.StatusCode != entry.status {
			t.Fatal("invalid camera target", r.StatusCode)
		}
	}
	r, body := get(path)
	if r.StatusCode != 200 || !bytes.Equal(body, data.Bytes()) || r.Header.Get("Content-Type") != "image/jpeg" || r.Header.Get("X-Frame-Updated") == "" || !strings.Contains(r.Header.Get("Cache-Control"), "no-store") {
		t.Fatal("invalid authenticated frame")
	}
	driver.frame.Updated = time.Now().Add(-time.Minute)
	r, _ = get(path)
	if r.StatusCode != 503 {
		t.Fatal("stale frame served")
	}
	driver.frame.Updated = time.Now()
	driver.frame.JPEG = []byte("not jpeg")
	r, _ = get(path)
	if r.StatusCode != 503 {
		t.Fatal("invalid image served")
	}
	driver.fail = true
	r, body = get(path)
	if r.StatusCode != 503 || bytes.Contains(body, []byte("sensitive-internal")) {
		t.Fatal("driver error exposed")
	}
	calls := driver.calls.Load()
	s.Config.DeviceMode = "offline"
	r, _ = get(path)
	if r.StatusCode != 409 || driver.calls.Load() != calls {
		t.Fatal("offline camera dialed")
	}
	s.Config.DeviceMode = "live"
	if e := s.SavePrinter(u, PrinterInput{ID: p.ID, Enabled: ptr(false)}); e != nil {
		t.Fatal(e)
	}
	r, _ = get(path)
	if r.StatusCode != 409 || driver.calls.Load() != calls {
		t.Fatal("disabled camera dialed")
	}
	var count int
	s.DB.QueryRow("SELECT COUNT(*) FROM printer_commands").Scan(&count)
	if count != 0 {
		t.Fatal("viewing camera created controls")
	}
}

func ptr(v bool) *bool { return &v }

type advancingCamera struct {
	*fakeDevice
	jpeg    []byte
	calls   atomic.Int32
	initial CameraFrame
}

func (f *advancingCamera) CameraFrame(ctx context.Context, p Printer) (CameraFrame, error) {
	n := f.calls.Add(1)
	if n == 1 {
		return f.initial, nil
	}
	select {
	case <-ctx.Done():
		return CameraFrame{}, ctx.Err()
	case <-time.After(30 * time.Millisecond):
	}
	return CameraFrame{JPEG: f.jpeg, Updated: time.Now()}, nil
}

func TestCameraCaptureAndStreamUseNewFramesAndStopAfterSessionRevocation(t *testing.T) {
	s, u, d := setup(t)
	fixture(t, s, u)
	s.Config.DeviceMode = "live"
	var data bytes.Buffer
	jpeg.Encode(&data, image.NewRGBA(image.Rect(0, 0, 16, 9)), nil)
	driver := &advancingCamera{fakeDevice: d, jpeg: data.Bytes(), initial: CameraFrame{JPEG: data.Bytes(), Updated: time.Now().Add(-time.Second)}}
	s.Device = driver
	server := httptest.NewServer(s.Handler())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 5 * time.Second}
	r, err := client.Post(server.URL+"/api/login", "application/json", strings.NewReader(`{"Username":"admin","Password":"TestPassword123"}`))
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	started := time.Now()
	r, err = client.Get(server.URL + "/api/printers/camera?printerId=1&capture=1")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(r.Body)
	r.Body.Close()
	if r.StatusCode != 200 || !bytes.Equal(body, data.Bytes()) || !strings.HasPrefix(r.Header.Get("Content-Disposition"), "attachment;") || driver.calls.Load() < 2 {
		t.Fatal("capture reused cached image or missing download header")
	}
	if n, _ := strconv.ParseInt(r.Header.Get("X-Frame-Updated"), 10, 64); n < started.UnixMilli() {
		t.Fatal("capture predates request")
	}
	r, err = client.Get(server.URL + "/api/printers/camera?printerId=1&stream=1")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	kind, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || kind != "multipart/x-mixed-replace" || r.Header.Get("X-Accel-Buffering") != "no" {
		t.Fatal("invalid video response")
	}
	reader := multipart.NewReader(r.Body, params["boundary"])
	last := ""
	for i := 0; i < 3; i++ {
		part, e := reader.NextPart()
		if e != nil {
			t.Fatal(e)
		}
		frame, e := io.ReadAll(part)
		part.Close()
		if e != nil || !bytes.Equal(frame, data.Bytes()) || part.Header.Get("X-Frame-Updated") == last {
			t.Fatal("invalid or duplicate stream frame")
		}
		last = part.Header.Get("X-Frame-Updated")
	}
	if _, err = s.DB.Exec("DELETE FROM sessions"); err != nil {
		t.Fatal(err)
	}
	for {
		part, e := reader.NextPart()
		if e != nil {
			break
		}
		_, _ = io.Copy(io.Discard, part)
		part.Close()
	}
	if time.Since(started) > 4*time.Second {
		t.Fatal("revoked session stream remained open")
	}
	var count int
	s.DB.QueryRow("SELECT COUNT(*) FROM printer_commands").Scan(&count)
	if count != 0 {
		t.Fatal("camera created machine controls")
	}
}
