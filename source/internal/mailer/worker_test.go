package mailer

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func workerTest(t *testing.T) *Worker {
	t.Helper()
	dir := t.TempDir()
	config := filepath.Join(dir, "smtp.json")
	b, _ := json.Marshal(SMTPConfig{Host: "smtp.163.com", Port: 465, Security: "implicit_tls", Username: "sender@163.com", Password: "test-only-code", FromName: "LayerHub 通知"})
	if err := os.WriteFile(config, b, 0600); err != nil {
		t.Fatal(err)
	}
	w, err := New(strings.Repeat("a", 64), "https://example.test/", config, filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	return w
}
func response(value any) *http.Response {
	b, _ := json.Marshal(value)
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(b)))}
}
func TestWorkerLostAckPersistsDedupeAcrossRestart(t *testing.T) {
	w := workerTest(t)
	sends, acks := 0, 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "Bearer "+w.Token || r.URL.Host != "127.0.0.1:15181" {
			t.Fatal("unsafe relay request")
		}
		switch r.URL.Path {
		case "/internal/mail/poll":
			return response(map[string]any{"ok": true, "message": Message{ID: "test-message", Lease: "lease", To: "member@example.com", Subject: "测试", Text: "text"}, "admins": []string{}}), nil
		case "/internal/mail/check":
			return response(map[string]bool{"allowed": true}), nil
		case "/internal/mail/ack":
			acks++
			if acks == 1 {
				return nil, errors.New("connection lost")
			}
			return response(map[string]bool{"ok": true}), nil
		}
		t.Fatal(r.URL.Path)
		return nil, nil
	})}
	w.Client = client
	w.Send = func(SMTPConfig, Message) error { sends++; return nil }
	if err := w.Step(context.Background()); err == nil {
		t.Fatal("lost ack not reported")
	}
	next, err := New(w.Token, w.PublicURL, w.ConfigPath, w.StatePath)
	if err != nil {
		t.Fatal(err)
	}
	next.Client = client
	next.Send = w.Send
	if err = next.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if sends != 1 || acks != 2 {
		t.Fatal("duplicate SMTP delivery", sends, acks)
	}
}
func TestWorkerRevokedLeaseDoesNotSendAndOfflineAlertIsBounded(t *testing.T) {
	w := workerTest(t)
	sends := 0
	w.Send = func(SMTPConfig, Message) error { sends++; return nil }
	w.Client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "poll") {
			return response(map[string]any{"ok": true, "message": Message{ID: "test-message"}, "admins": []string{}}), nil
		}
		return response(map[string]bool{"allowed": false}), nil
	})}
	if err := w.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if sends != 0 {
		t.Fatal("revoked lease sent")
	}
	w.State.Admins = []string{"admin@example.com"}
	w.State.Seen = time.Now().Unix() - 220
	w.State.DownSince = time.Now().Unix() - 200
	w.Client = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("offline") })}
	_ = w.Step(context.Background())
	_ = w.Step(context.Background())
	if sends != 1 {
		t.Fatal("offline alert duplicate or missing", sends)
	}
	w.State.DownSince--
	w.State.DownSent = false
	w.State.Seen = time.Now().Unix() - 601
	_ = w.Step(context.Background())
	if sends != 1 {
		t.Fatal("stale opt-in cache used")
	}
}
func TestRenderUTF8AndHeaderInjection(t *testing.T) {
	c := SMTPConfig{Username: "sender@163.com", FromName: "LayerHub 通知"}
	m := Message{ID: "stable-id", To: "member@example.com", Subject: "打印已完成", Text: "中文通知\n正文"}
	b, err := Render(c, m)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "Content-Transfer-Encoding: base64\r\n") || !strings.Contains(string(b), "Message-ID: <stable-id.layerhub@163.com>") || !strings.Contains(string(b), "Auto-Submitted: auto-generated\r\n") || strings.Contains(string(b), "layerhub.local") {
		t.Fatal("bad MIME")
	}
	for _, bad := range []Message{{ID: "x", To: "member@example.com\r\nBcc: other@example.com"}, {ID: "x", To: m.To, Subject: "test\r\nBcc: other@example.com"}, {ID: "id\r\n", To: m.To}} {
		if _, err := Render(c, bad); err == nil {
			t.Fatal("header injection accepted")
		}
	}
}
