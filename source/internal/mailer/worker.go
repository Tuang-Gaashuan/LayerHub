// Package mailer runs on the cloud gateway. It never connects to printers and
// has no public HTTP listener. All NAS requests use the private SSH tunnel.
package mailer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type SMTPConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Security string `json:"security"`
	Username string `json:"username"`
	Password string `json:"password"`
	FromName string `json:"from_name"`
}
type Message struct {
	ID      string `json:"id"`
	Lease   string `json:"lease"`
	To      string `json:"to"`
	Subject string `json:"subject"`
	Text    string `json:"text"`
}
type State struct {
	Admins    []string         `json:"admins"`
	Seen      int64            `json:"seen"`
	DownSince int64            `json:"downSince"`
	DownSent  bool             `json:"downSent"`
	Delivered map[string]int64 `json:"delivered"`
}
type Worker struct {
	Token      string
	PublicURL  string
	ConfigPath string
	StatePath  string
	Client     *http.Client
	State      State
	Send       func(SMTPConfig, Message) error
}

func New(token, publicURL, configPath, statePath string) (*Worker, error) {
	if len(token) < 32 || !strings.HasPrefix(publicURL, "https://") {
		return nil, errors.New("mail worker configuration invalid")
	}
	w := &Worker{Token: token, PublicURL: publicURL, ConfigPath: configPath, StatePath: statePath, Client: &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: 8 * time.Second}, Send: SendSMTP}
	if b, err := os.ReadFile(statePath); err == nil {
		if err = json.Unmarshal(b, &w.State); err != nil {
			return nil, errors.New("invalid worker state")
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if w.State.Delivered == nil {
		w.State.Delivered = map[string]int64{}
	}
	return w, nil
}
func (w *Worker) save() error {
	if err := os.MkdirAll(filepath.Dir(w.StatePath), 0700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(w.StatePath), ".mail-state-")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)
	if err = temp.Chmod(0600); err == nil {
		err = json.NewEncoder(temp).Encode(w.State)
	}
	if err == nil {
		err = temp.Sync()
	}
	closeErr := temp.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(name, w.StatePath)
}
func readConfig(name string) (SMTPConfig, error) {
	var c SMTPConfig
	b, err := os.ReadFile(name)
	if err != nil {
		return c, err
	}
	if len(b) > 8192 {
		return c, errors.New("invalid SMTP config")
	}
	if err = json.Unmarshal(b, &c); err != nil {
		return c, errors.New("invalid SMTP config")
	}
	a, err := mail.ParseAddress(c.Username)
	if err != nil || a.Address != c.Username || c.Host != "smtp.163.com" || c.Port != 465 || c.Security != "implicit_tls" || c.Password == "" || strings.ContainsAny(c.FromName, "\r\n") {
		return c, errors.New("invalid SMTP config")
	}
	return c, nil
}
func (w *Worker) api(ctx context.Context, path string, in, out any) error {
	b, err := json.Marshal(in)
	if err != nil {
		return err
	}
	r, err := http.NewRequestWithContext(ctx, "POST", "http://127.0.0.1:15181/internal/mail/"+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	r.Header.Set("Authorization", "Bearer "+w.Token)
	r.Header.Set("Content-Type", "application/json")
	response, err := w.Client.Do(r)
	if err != nil {
		return errors.New("NAS mail channel unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return fmt.Errorf("NAS mail channel status %d", response.StatusCode)
	}
	if out == nil {
		_, err = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		return err
	}
	return json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(out)
}
func (w *Worker) Step(ctx context.Context) error {
	cfg, cfgErr := readConfig(w.ConfigPath)
	var poll struct {
		OK      bool     `json:"ok"`
		Message *Message `json:"message"`
		Admins  []string `json:"admins"`
	}
	err := w.api(ctx, "poll", map[string]bool{"ready": cfgErr == nil}, &poll)
	now := time.Now().Unix()
	if err != nil {
		if w.State.DownSince == 0 {
			w.State.DownSince = now
		}
		// Use only a recent opt-in admin snapshot, once per outage. No unverified or
		// stale address cache is used to retry an offline alert indefinitely.
		if cfgErr == nil && !w.State.DownSent && now-w.State.DownSince >= 180 && now-w.State.Seen < 600 && w.State.Seen > 0 {
			all := true
			for _, to := range w.State.Admins {
				digest := sha256.Sum256([]byte(to))
				m := Message{ID: fmt.Sprintf("nas-down-%d-%x", w.State.DownSince, digest[:8]), To: to, Subject: "LayerHub · NAS 云端通道持续离线", Text: "NAS 云端通道已持续不可用超过 3 分钟。请检查 NAS 是否挂起、供电和网络。\n此邮件不代表打印机停止打印，请勿重复提交任务。\n\n" + w.PublicURL}
				if sendErr := w.deliver(cfg, m); sendErr != nil {
					all = false
				}
			}
			w.State.DownSent = all
		}
		if saveErr := w.save(); saveErr != nil {
			return saveErr
		}
		return err
	}
	if !poll.OK {
		return errors.New("invalid NAS mail response")
	}
	w.State.Admins = poll.Admins
	w.State.Seen = now
	w.State.DownSince = 0
	w.State.DownSent = false
	for id, at := range w.State.Delivered {
		if now-at > 7*86400 {
			delete(w.State.Delivered, id)
		}
	}
	if err = w.save(); err != nil {
		return err
	}
	if cfgErr != nil {
		return errors.New("SMTP not configured")
	}
	if poll.Message == nil {
		return nil
	}
	m := *poll.Message
	var check struct {
		Allowed bool `json:"allowed"`
	}
	if err = w.api(ctx, "check", map[string]string{"id": m.ID, "lease": m.Lease}, &check); err != nil {
		return err
	}
	if !check.Allowed {
		return nil
	}
	sendErr := w.deliver(cfg, m)
	// A cached successful SMTP handoff is acknowledged without sending again if
	// the previous NAS acknowledgment was lost. SMTP loss during DATA is still
	// inherently ambiguous; this is not an exactly-once transport.
	if err = w.api(ctx, "ack", map[string]any{"id": m.ID, "lease": m.Lease, "sent": sendErr == nil}, nil); err != nil {
		return err
	}
	if sendErr != nil {
		return errors.New("SMTP delivery failed; retry scheduled")
	}
	return nil
}
func (w *Worker) deliver(c SMTPConfig, m Message) error {
	if _, ok := w.State.Delivered[m.ID]; ok {
		return nil
	}
	if err := w.Send(c, m); err != nil {
		return err
	}
	w.State.Delivered[m.ID] = time.Now().Unix()
	return w.save()
}
func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	lastError := ""
	for {
		err := w.Step(ctx)
		status := ""
		if err != nil {
			status = err.Error()
		}
		if status != lastError {
			if status != "" {
				log.Printf("mail worker: %s", status)
			} else {
				log.Print("mail worker: channel ready")
			}
			lastError = status
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

var safeID = regexp.MustCompile(`^[a-zA-Z0-9-]{1,160}$`)

func Render(c SMTPConfig, m Message) ([]byte, error) {
	to, err := mail.ParseAddress(m.To)
	if err != nil || to.Address != m.To || strings.ContainsAny(m.Subject+m.To, "\r\n") || !safeID.MatchString(m.ID) {
		return nil, errors.New("invalid message")
	}
	from, err := mail.ParseAddress(c.Username)
	if err != nil || from.Address != c.Username || strings.ContainsAny(c.FromName, "\r\n") {
		return nil, errors.New("invalid sender")
	}
	from.Name = c.FromName
	// Use the authenticated sender's domain for stable message identifiers.
	// This is formatting hygiene; it does not replace SPF/DKIM/DMARC checks.
	identifierDomain := from.Address[strings.LastIndexByte(from.Address, '@')+1:]
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMessage-ID: <%s.layerhub@%s>\r\nAuto-Submitted: auto-generated\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: base64\r\n\r\n", from.String(), to.String(), mime.QEncoding.Encode("UTF-8", m.Subject), time.Now().Format(time.RFC1123Z), m.ID, identifierDomain)
	body := base64.StdEncoding.EncodeToString([]byte(m.Text))
	for len(body) > 76 {
		b.WriteString(body[:76] + "\r\n")
		body = body[76:]
	}
	b.WriteString(body + "\r\n")
	return []byte(b.String()), nil
}
func SendSMTP(c SMTPConfig, m Message) error {
	body, err := Render(c, m)
	if err != nil {
		return err
	}
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 10 * time.Second}, "tcp", net.JoinHostPort(c.Host, fmt.Sprint(c.Port)), &tls.Config{ServerName: c.Host, MinVersion: tls.VersionTLS12})
	if err != nil {
		return errors.New("SMTP TLS connection failed")
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	client, err := smtp.NewClient(conn, c.Host)
	if err != nil {
		return errors.New("SMTP greeting failed")
	}
	defer client.Close()
	if err = client.Auth(smtp.PlainAuth("", c.Username, c.Password, c.Host)); err != nil {
		return errors.New("SMTP authentication rejected")
	}
	if err = client.Mail(c.Username); err != nil {
		return errors.New("SMTP sender rejected")
	}
	if err = client.Rcpt(m.To); err != nil {
		return errors.New("SMTP recipient rejected")
	}
	target, err := client.Data()
	if err != nil {
		return errors.New("SMTP data rejected")
	}
	if _, err = target.Write(body); err != nil {
		return errors.New("SMTP transfer failed")
	}
	if err = target.Close(); err != nil {
		return errors.New("SMTP delivery unconfirmed")
	}
	// DATA accepted is the durable server handoff, not proof of inbox delivery.
	_ = client.Quit()
	return nil
}
