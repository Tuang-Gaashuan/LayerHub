package app

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"
)

// An additive extension has its own schema version. Old LayerHub binaries can
// still open workspace version 3; they simply do not run notification workers.
const mailSchema = `
CREATE TABLE IF NOT EXISTS mail_meta(version INTEGER NOT NULL);
INSERT INTO mail_meta SELECT 1 WHERE NOT EXISTS(SELECT 1 FROM mail_meta);
CREATE TABLE IF NOT EXISTS mail_accounts(user_id INTEGER PRIMARY KEY REFERENCES users(id),email TEXT NOT NULL,verified INTEGER NOT NULL,enabled INTEGER NOT NULL DEFAULT 1);
CREATE TABLE IF NOT EXISTS mail_challenges(user_id INTEGER PRIMARY KEY REFERENCES users(id),id TEXT NOT NULL,email TEXT NOT NULL,hash TEXT NOT NULL,expires INTEGER NOT NULL,attempts INTEGER NOT NULL DEFAULT 0,requested INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS mail_outbox(id TEXT PRIMARY KEY,user_id INTEGER NOT NULL REFERENCES users(id),email TEXT NOT NULL,kind TEXT NOT NULL,event_key TEXT NOT NULL UNIQUE,body BLOB NOT NULL,created INTEGER NOT NULL,expires INTEGER NOT NULL,state TEXT NOT NULL DEFAULT 'queued',attempts INTEGER NOT NULL DEFAULT 0,next_at INTEGER NOT NULL DEFAULT 0,lease TEXT NOT NULL DEFAULT '',lease_until INTEGER NOT NULL DEFAULT 0);
CREATE INDEX IF NOT EXISTS mail_outbox_pending ON mail_outbox(state,next_at);
CREATE TABLE IF NOT EXISTS mail_observations(key TEXT PRIMARY KEY,value TEXT NOT NULL,candidate TEXT NOT NULL,since INTEGER NOT NULL,seq INTEGER NOT NULL DEFAULT 0);
CREATE TABLE IF NOT EXISTS mail_worker(id INTEGER PRIMARY KEY CHECK(id=1),seen INTEGER NOT NULL,ready INTEGER NOT NULL);
`

type mailMessage struct {
	ID      string `json:"id"`
	Lease   string `json:"lease"`
	To      string `json:"to"`
	Subject string `json:"subject"`
	Text    string `json:"text"`
}

func (s *Service) initMail() error {
	if _, err := s.DB.Exec(mailSchema); err != nil {
		return err
	}
	if _, err := s.DB.Exec(settlementSchema); err != nil {
		return err
	}
	var version int
	if err := s.DB.QueryRow("SELECT version FROM mail_meta").Scan(&version); err != nil {
		return err
	}
	if version != 1 {
		return fmt.Errorf("unsupported mail schema")
	}
	// A stale heartbeat cannot make a newly started backend appear ready.
	if _, err := s.DB.Exec("DELETE FROM mail_worker"); err != nil {
		return err
	}
	if _, err := s.DB.Exec(usabilitySchema); err != nil {
		return err
	}
	var hasStarted int
	if err := s.DB.QueryRow("SELECT COUNT(*) FROM pragma_table_info('notification_preferences') WHERE name='started'").Scan(&hasStarted); err != nil {
		return err
	}
	if hasStarted == 0 {
		if _, err := s.DB.Exec("ALTER TABLE notification_preferences ADD COLUMN started INTEGER NOT NULL DEFAULT 1"); err != nil {
			return err
		}
	}
	if s.Config.DisableBackgroundEvents {
		return nil
	}
	s.workers.Add(1)
	go func() {
		defer s.workers.Done()
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		failed := false
		for {
			err := s.collectMail(time.Now())
			if err != nil && !failed {
				log.Printf("notification observer failed: %T", err)
			}
			failed = err != nil
			select {
			case <-s.ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return nil
}

func (s *Service) mailConfigured() bool {
	u, err := url.Parse(s.Config.PublicURL)
	return err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil && len(s.Config.MailToken) >= 32
}
func (s *Service) mailReady() bool {
	var seen int64
	var ready bool
	return s.mailConfigured() && s.DB.QueryRow("SELECT seen,ready FROM mail_worker WHERE id=1").Scan(&seen, &ready) == nil && ready && time.Now().Unix()-seen < 60
}
func normalizeEmail(value string) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) > 254 || strings.ContainsAny(value, "\r\n") {
		return "", invalid("请输入有效的邮箱地址")
	}
	for _, c := range value {
		if c < 33 || c > 126 {
			return "", invalid("邮箱地址暂时只支持英文字母、数字与常用符号")
		}
	}
	a, err := mail.ParseAddress(value)
	if err != nil || a.Address != value || a.Name != "" || !strings.Contains(value, "@") {
		return "", invalid("请输入完整的邮箱地址")
	}
	return strings.ToLower(value), nil
}
func (s *Service) emailStatus(u User) (any, error) {
	result := map[string]any{"ok": true, "email": "", "verified": false, "enabled": false, "available": s.mailReady(), "pendingEmail": "", "pendingState": "", "resendAfter": int64(0)}
	var address string
	var enabled bool
	err := s.DB.QueryRow("SELECT email,enabled FROM mail_accounts WHERE user_id=?", u.ID).Scan(&address, &enabled)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err == nil {
		result["email"] = address
		result["verified"] = true
		result["enabled"] = enabled
	}
	var pending, id string
	var requested, expires int64
	err = s.DB.QueryRow("SELECT email,id,requested,expires FROM mail_challenges WHERE user_id=?", u.ID).Scan(&pending, &id, &requested, &expires)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err == nil && expires > time.Now().Unix() {
		var state string
		_ = s.DB.QueryRow("SELECT state FROM mail_outbox WHERE event_key=?", "verify:"+id).Scan(&state)
		result["pendingEmail"] = pending
		result["pendingState"] = state
		result["resendAfter"] = max(int64(0), requested+60-time.Now().Unix())
	}
	if u.Admin {
		var failed int
		if err = s.DB.QueryRow("SELECT COUNT(*) FROM mail_outbox WHERE state='failed' AND created>?", time.Now().Add(-24*time.Hour).Unix()).Scan(&failed); err != nil {
			return nil, err
		}
		result["failedLastDay"] = failed
	}
	return result, nil
}
func (s *Service) otpHash(id string, user int64, email, code string) string {
	h := hmac.New(sha256.New, s.key)
	fmt.Fprintf(h, "mail-code:%s:%d:%s:%s", id, user, email, code)
	return hex.EncodeToString(h.Sum(nil))
}
func (s *Service) queueMail(tx *sql.Tx, uid int64, email, kind, event, subject, text string, now, expires int64) error {
	body, err := s.seal(string(mustJSON(map[string]string{"subject": subject, "text": text})))
	if err != nil {
		return err
	}
	_, err = tx.Exec("INSERT OR IGNORE INTO mail_outbox(id,user_id,email,kind,event_key,body,created,expires) VALUES(?,?,?,?,?,?,?,?)", randomID(16), uid, email, kind, event, body, now, expires)
	return err
}
func mustJSON(value any) []byte { b, _ := json.Marshal(value); return b }
func (s *Service) requestEmail(u User, value string) error {
	if !s.mailReady() {
		return conflict("邮件服务暂不可用，请稍后重试；不绑定邮箱也可以正常使用")
	}
	email, err := normalizeEmail(value)
	if err != nil {
		return err
	}
	return s.transaction(func(tx *sql.Tx) error {
		now := time.Now().Unix()
		var count int
		if err := tx.QueryRow("SELECT COUNT(*) FROM mail_accounts WHERE user_id=? AND email=?", u.ID, email).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			return conflict("这个邮箱已经验证，无需再次绑定")
		}
		var requested int64
		err := tx.QueryRow("SELECT requested FROM mail_challenges WHERE user_id=?", u.ID).Scan(&requested)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if requested+60 > now {
			return conflict("请至少间隔 60 秒再获取验证码")
		}
		if err = tx.QueryRow("SELECT COUNT(*) FROM mail_outbox WHERE kind='verify' AND created>? AND (user_id=? OR email=?)", now-3600, u.ID, email).Scan(&count); err != nil {
			return err
		}
		if count >= 5 {
			return conflict("验证码请求较多，请一小时后再试")
		}
		if err = tx.QueryRow("SELECT COUNT(*) FROM mail_outbox WHERE kind='verify' AND created>?", now-86400).Scan(&count); err != nil {
			return err
		}
		if count >= 100 {
			return conflict("今日验证码额度已用完，请联系管理员")
		}
		number, err := rand.Int(rand.Reader, big.NewInt(1000000))
		if err != nil {
			return err
		}
		code := fmt.Sprintf("%06d", number.Int64())
		id := randomID(16)
		if _, err = tx.Exec("UPDATE mail_outbox SET state='cancelled',body=X'' WHERE user_id=? AND kind='verify' AND state IN ('queued','leased')", u.ID); err != nil {
			return err
		}
		if _, err = tx.Exec("INSERT INTO mail_challenges(user_id,id,email,hash,expires,requested) VALUES(?,?,?,?,?,?) ON CONFLICT(user_id) DO UPDATE SET id=excluded.id,email=excluded.email,hash=excluded.hash,expires=excluded.expires,attempts=0,requested=excluded.requested", u.ID, id, email, s.otpHash(id, u.ID, email, code), now+600, now); err != nil {
			return err
		}
		language := accountLanguage(tx, u.ID)
		return s.queueMail(tx, u.ID, email, "verify", "verify:"+id, languageText(language, "LayerHub 邮箱绑定验证码"), languageText(language, "你的 LayerHub 邮箱绑定验证码是：")+code+languageText(language, "\n\n10 分钟内有效，最多可尝试 5 次。请勿把验证码交给他人。\n如果不是你本人操作，请忽略此邮件。\n\n")+s.Config.PublicURL, now, now+600)
	})
}
func (s *Service) verifyEmail(u User, code string) error {
	var denied error
	err := s.transaction(func(tx *sql.Tx) error {
		var id, email, hash string
		var expires int64
		var attempts int
		if err := tx.QueryRow("SELECT id,email,hash,expires,attempts FROM mail_challenges WHERE user_id=?", u.ID).Scan(&id, &email, &hash, &expires, &attempts); err != nil {
			return invalid("请先获取验证码")
		}
		if expires <= time.Now().Unix() || attempts >= 5 {
			return invalid("验证码已过期或尝试次数已用完，请重新获取")
		}
		if len(code) != 6 || subtle.ConstantTimeCompare([]byte(hash), []byte(s.otpHash(id, u.ID, email, code))) != 1 {
			_, err := tx.Exec("UPDATE mail_challenges SET attempts=attempts+1 WHERE user_id=?", u.ID)
			denied = invalid("验证码错误，请检查邮件后重试")
			return err
		}
		if _, err := tx.Exec("UPDATE mail_outbox SET state='cancelled',body=X'' WHERE user_id=? AND state IN ('queued','leased')", u.ID); err != nil {
			return err
		}
		if _, err := tx.Exec("INSERT INTO mail_accounts(user_id,email,verified,enabled) VALUES(?,?,?,1) ON CONFLICT(user_id) DO UPDATE SET email=excluded.email,verified=excluded.verified,enabled=1", u.ID, email, time.Now().Unix()); err != nil {
			return err
		}
		_, err := tx.Exec("DELETE FROM mail_challenges WHERE user_id=?", u.ID)
		return err
	})
	if err != nil {
		return err
	}
	return denied
}
func (s *Service) emailPreference(u User, enabled, unlink bool) error {
	return s.transaction(func(tx *sql.Tx) error {
		var err error
		if unlink {
			if _, err = tx.Exec("DELETE FROM mail_challenges WHERE user_id=?", u.ID); err != nil {
				return err
			}
			_, err = tx.Exec("DELETE FROM mail_accounts WHERE user_id=?", u.ID)
		} else {
			_, err = tx.Exec("UPDATE mail_accounts SET enabled=? WHERE user_id=?", enabled, u.ID)
		}
		if err != nil {
			return err
		}
		if !enabled || unlink {
			_, err = tx.Exec("UPDATE mail_outbox SET state='cancelled',body=X'' WHERE user_id=? AND state IN ('queued','leased')", u.ID)
		}
		return err
	})
}

// This endpoint is additionally denied by the public Nginx gateway. The worker
// accesses it only through cloud loopback and the existing SSH reverse tunnel.
func (s *Service) mailRelay(w http.ResponseWriter, r *http.Request) {
	if len(s.Config.MailToken) < 32 || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+s.Config.MailToken)) != 1 {
		reply(w, 404, map[string]any{"ok": false})
		return
	}
	if r.Method != "POST" {
		reply(w, 405, map[string]any{"ok": false})
		return
	}
	if s.pendingRestore.Load() {
		fail(w, conflict("restore pending"))
		return
	}
	switch r.URL.Path {
	case "/internal/mail/check":
		var in struct {
			ID    string `json:"id"`
			Lease string `json:"lease"`
		}
		if err := decode(r, &in); err != nil {
			fail(w, err)
			return
		}
		var count int
		err := s.DB.QueryRow(`SELECT COUNT(*) FROM mail_outbox m JOIN users u ON u.id=m.user_id WHERE m.id=? AND m.lease=? AND m.state='leased' AND m.lease_until>? AND m.expires>? AND u.disabled=0 AND ((m.kind='verify' AND EXISTS(SELECT 1 FROM mail_challenges c WHERE c.user_id=m.user_id AND 'verify:'||c.id=m.event_key AND c.attempts<5 AND c.expires>?)) OR (m.kind<>'verify' AND EXISTS(SELECT 1 FROM mail_accounts a WHERE a.user_id=m.user_id AND a.email=m.email AND a.enabled=1)))`, in.ID, in.Lease, time.Now().Unix(), time.Now().Unix(), time.Now().Unix()).Scan(&count)
		if err != nil {
			fail(w, err)
			return
		}
		reply(w, 200, map[string]any{"ok": true, "allowed": count == 1})
	case "/internal/mail/poll":
		var in struct {
			Ready bool `json:"ready"`
		}
		if err := decode(r, &in); err != nil {
			fail(w, err)
			return
		}
		message, admins, err := s.claimMail(in.Ready)
		if err != nil {
			fail(w, err)
			return
		}
		reply(w, 200, map[string]any{"ok": true, "message": message, "admins": admins})
	case "/internal/mail/ack":
		var in struct {
			ID    string `json:"id"`
			Lease string `json:"lease"`
			Sent  bool   `json:"sent"`
		}
		if err := decode(r, &in); err != nil {
			fail(w, err)
			return
		}
		err := s.transaction(func(tx *sql.Tx) error {
			now := time.Now().Unix()
			state := "queued"
			if in.Sent {
				state = "sent"
			}
			_, err := tx.Exec("UPDATE mail_outbox SET state=CASE WHEN ?='queued' AND attempts>=5 THEN 'failed' ELSE ? END,next_at=?+min(3600,30*(1 << min(attempts,6))),body=CASE WHEN ?='sent' OR attempts>=5 THEN X'' ELSE body END,lease='',lease_until=0 WHERE id=? AND lease=? AND state='leased' AND lease_until>?", state, state, now, state, in.ID, in.Lease, now)
			return err
		})
		if err != nil {
			fail(w, err)
			return
		}
		reply(w, 200, map[string]any{"ok": true})
	default:
		reply(w, 404, map[string]any{"ok": false})
	}
}

func (s *Service) claimMail(ready bool) (*mailMessage, []string, error) {
	admins := []string{}
	var message *mailMessage
	err := s.transaction(func(tx *sql.Tx) error {
		now := time.Now().Unix()
		if _, err := tx.Exec("INSERT INTO mail_worker(id,seen,ready) VALUES(1,?,?) ON CONFLICT(id) DO UPDATE SET seen=excluded.seen,ready=excluded.ready", now, ready); err != nil {
			return err
		}
		// Expired, disabled, unbound and old-address messages are never newly leased.
		if _, err := tx.Exec(`UPDATE mail_outbox SET state='cancelled',body=X'' WHERE state IN ('queued','leased') AND (expires<=? OR NOT EXISTS(SELECT 1 FROM users WHERE id=mail_outbox.user_id AND disabled=0) OR (kind='verify' AND NOT EXISTS(SELECT 1 FROM mail_challenges WHERE user_id=mail_outbox.user_id AND 'verify:'||id=mail_outbox.event_key AND expires>? AND attempts<5)) OR (kind<>'verify' AND NOT EXISTS(SELECT 1 FROM mail_accounts WHERE user_id=mail_outbox.user_id AND email=mail_outbox.email AND enabled=1)))`, now, now); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE mail_outbox SET state=CASE WHEN attempts>=5 THEN 'failed' ELSE 'queued' END,body=CASE WHEN attempts>=5 THEN X'' ELSE body END WHERE state='leased' AND lease_until<=?", now); err != nil {
			return err
		}
		if _, err := tx.Exec("DELETE FROM mail_outbox WHERE created<? AND state NOT IN ('queued','leased')", now-7*86400); err != nil {
			return err
		}
		rows, err := tx.Query("SELECT a.email FROM mail_accounts a JOIN users u ON u.id=a.user_id LEFT JOIN notification_preferences p ON p.user_id=u.id WHERE a.enabled=1 AND u.admin=1 AND u.disabled=0 AND COALESCE(p.devices,1)=1 ORDER BY u.id")
		if err != nil {
			return err
		}
		for rows.Next() {
			var email string
			if err = rows.Scan(&email); err != nil {
				rows.Close()
				return err
			}
			admins = append(admins, email)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if !ready {
			return nil
		}
		var m mailMessage
		var encrypted []byte
		err = tx.QueryRow("SELECT id,email,body FROM mail_outbox WHERE state='queued' AND next_at<=? ORDER BY CASE kind WHEN 'verify' THEN 0 ELSE 1 END,created,id LIMIT 1", now).Scan(&m.ID, &m.To, &encrypted)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		body, err := s.unseal(encrypted)
		if err != nil {
			return err
		}
		var content struct{ Subject, Text string }
		if err = json.Unmarshal([]byte(body), &content); err != nil {
			return err
		}
		m.Subject = content.Subject
		m.Text = content.Text
		m.Lease = randomID(24)
		if _, err = tx.Exec("UPDATE mail_outbox SET state='leased',attempts=attempts+1,lease=?,lease_until=? WHERE id=?", m.Lease, now+120, m.ID); err != nil {
			return err
		}
		message = &m
		return nil
	})
	return message, admins, err
}

// observeMail persists both the baseline and debounce window across restarts.
// Existing events seed a baseline, so enabling mail never floods old alerts.
func observeMail(tx *sql.Tx, key, value string, now, delay int64) (int64, bool, error) {
	var old, candidate string
	var since, seq int64
	err := tx.QueryRow("SELECT value,candidate,since,seq FROM mail_observations WHERE key=?", key).Scan(&old, &candidate, &since, &seq)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = tx.Exec("INSERT INTO mail_observations(key,value,candidate,since) VALUES(?,?,?,?)", key, value, value, now)
		return 0, false, err
	}
	if err != nil {
		return 0, false, err
	}
	if candidate != value {
		since = now
		if _, err = tx.Exec("UPDATE mail_observations SET candidate=?,since=? WHERE key=?", value, since, key); err != nil {
			return 0, false, err
		}
	}
	if old == value || now-since < delay {
		return seq, false, nil
	}
	seq++
	_, err = tx.Exec("UPDATE mail_observations SET value=?,seq=? WHERE key=?", value, seq, key)
	return seq, err == nil, err
}
func (s *Service) notifyUser(tx *sql.Tx, uid int64, event, subject, text string, now int64) error {
	var active int
	if err := tx.QueryRow("SELECT COUNT(*) FROM users WHERE id=? AND disabled=0", uid).Scan(&active); err != nil {
		return err
	}
	if active == 0 {
		return nil
	}
	category := notificationCategory(event, subject)
	entityType, entityID := "", ""
	parts := strings.Split(event, ":")
	if len(parts) >= 3 {
		switch parts[0] {
		case "job":
			entityType = "job"
		case "printer":
			entityType = "printer"
		case "stock":
			entityType = "spool"
		}
		if entityType != "" {
			entityID = parts[1]
		}
	}
	r, err := tx.Exec("INSERT OR IGNORE INTO user_notifications(user_id,event_key,category,title,body,entity_type,entity_id,created) VALUES(?,?,?,?,?,?,?,?)", uid, event, category, subject, text, entityType, entityID, now)
	if err != nil {
		return err
	}
	inserted, err := r.RowsAffected()
	if err != nil {
		return err
	}
	if inserted == 0 {
		return nil
	}
	prefs, err := readNotificationPreferences(tx, uid)
	if err != nil {
		return err
	}
	if !prefs.allows(category) {
		return nil
	}
	var email string
	err = tx.QueryRow("SELECT a.email FROM mail_accounts a JOIN users u ON u.id=a.user_id WHERE a.user_id=? AND a.enabled=1 AND u.disabled=0", uid).Scan(&email)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return s.queueMail(tx, uid, email, "notification", fmt.Sprintf("%s:user:%d", event, uid), subject, text+"\n\n"+languageText(accountLanguage(tx, uid), "打开 LayerHub 查看并确认：")+"\n"+s.Config.PublicURL, now, now+86400)
}
func (s *Service) collectMail(at time.Time) error {
	return s.transaction(func(tx *sql.Tx) error {
		now := at.Unix()
		jobs, err := readJobs(tx)
		if err != nil {
			return err
		}
		for _, j := range jobs {
			t := s.Device.Status(j.Printer)
			j, err = s.observeDeviceStopped(tx, j, t, now)
			if err != nil {
				return err
			}
			state := j.State
			if j.Error != "" && (state == "queued" || state == "assigned") {
				state = "needs_review"
			}
			active := state == "printing" || state == "paused"
			matched := active && mailTaskMatches(j, t)
			if matched && state == "printing" && t.RawState == "RUNNING" && t.State == "printing" {
				if err = s.notifyPrintMilestone(tx, j, "started", "打印已开始", "打印机已上报正在执行此任务。", now, false); err != nil {
					return err
				}
			}
			if active && printFinishedMatches(j, t) {
				state = "device_finished"
				if err = s.observePrintFinished(tx, j, t, now); err != nil {
					return err
				}
			}
			seq, changed, err := observeMail(tx, fmt.Sprintf("job:%d", j.ID), state, now, 0)
			if err != nil {
				return err
			}
			if !changed {
				continue
			}
			label := ""
			switch state {
			case "done":
				// Completion mail belongs to the hardware milestone, never to
				// the later stock/collection confirmation.
				continue
			case "paused":
				label = "任务已暂停，请检查"
			case "stopped":
				label = "任务已停止，请核对"
			case "failed", "checking", "needs_review":
				label = "任务需要检查"
			}
			if label != "" {
				language := accountLanguage(tx, j.UserID)
				localizedLabel := languageText(language, label)
				if err = s.notifyUser(tx, j.UserID, fmt.Sprintf("job:%d:%d", j.ID, seq), "LayerHub · "+localizedLabel, localizedFormat(language, "%s\n任务：%s\n任务编号：%d\n请在工作空间核对设备状态和耗材用量。", localizedLabel, j.Filename, j.ID), now); err != nil {
					return err
				}
			}
		}
		printers, err := readPrinters(tx)
		if err != nil {
			return err
		}
		for _, p := range printers {
			t := s.Device.Status(p.ID)
			state := "ok"
			delay := int64(30)
			if !p.Enabled {
				state = "disabled"
			} else if !t.Connected || !timeFresh(t.Updated) {
				state = "offline"
				delay = 180
			} else if t.PrintError != 0 {
				state = fmt.Sprintf("fault:%d", t.PrintError)
			}
			seq, changed, err := observeMail(tx, fmt.Sprintf("printer:%d", p.ID), state, now, delay)
			if err != nil {
				return err
			}
			if !changed || state == "ok" || state == "disabled" {
				continue
			}
			rows, err := tx.Query("SELECT id FROM users WHERE admin=1 AND disabled=0")
			if err != nil {
				return err
			}
			ids := []int64{}
			for rows.Next() {
				var id int64
				if err = rows.Scan(&id); err != nil {
					rows.Close()
					return err
				}
				ids = append(ids, id)
			}
			err = rows.Err()
			rows.Close()
			if err != nil {
				return err
			}
			label := "设备报告异常"
			if state == "offline" {
				label = "打印机持续离线"
			}
			for _, uid := range ids {
				language := accountLanguage(tx, uid)
				localizedLabel := languageText(language, label)
				if err = s.notifyUser(tx, uid, fmt.Sprintf("printer:%d:%d", p.ID, seq), "LayerHub · "+localizedLabel, localizedFormat(language, "%s\n设备：%s\n请检查设备与 NAS 连接。邮件仅报告状态，不会重发打印或控制命令。", localizedLabel, p.Name), now); err != nil {
					return err
				}
			}
		}
		if err = s.observeStock(tx, now, jobs); err != nil {
			return err
		}
		_, err = tx.Exec("DELETE FROM user_notifications WHERE created<?", now-90*86400)
		return err
	})
}

// Backup exports do not retain codes, delivery queues, relay health or event
// baselines. Verified bindings survive, but notifications restore as disabled.
func scrubMailBackup(db *sql.DB) error {
	var exists int
	if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE name='mail_accounts' AND type='table'").Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return nil
	}
	_, err := db.Exec("DELETE FROM mail_challenges; DELETE FROM mail_outbox; DELETE FROM mail_worker; DELETE FROM mail_observations; UPDATE mail_accounts SET enabled=0;")
	return err
}
