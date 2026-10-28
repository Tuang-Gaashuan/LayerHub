package app

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func reply(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func fail(w http.ResponseWriter, err error) {
	var e *apiError
	if errors.As(err, &e) {
		reply(w, e.Code, map[string]any{"ok": false, "error": languageText(w.Header().Get("Content-Language"), e.Message)})
		return
	}
	log.Printf("request failed: %T", err)
	reply(w, 500, map[string]any{"ok": false, "error": languageText(w.Header().Get("Content-Language"), "服务器处理失败，请查看服务日志")})
}
func decode(r *http.Request, target any) error {
	defer r.Body.Close()
	limit := int64(64 << 10)

	raw, e := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if e != nil || int64(len(raw)) > limit {
		return invalid("请求内容过长")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return invalid("请求 JSON 格式无效")
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return invalid("请求内容过长或包含多余数据")
	}
	return nil
}
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		reply(w, 200, map[string]any{"ok": true, "backend": "go", "deviceMode": s.Config.DeviceMode})
	})
	mux.HandleFunc("POST /api/login", s.login)
	mux.HandleFunc("/internal/mail/", s.mailRelay)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		u, token, err := s.session(r)
		if err != nil {
			fail(w, err)
			return
		}
		if r.Method != "GET" && subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(s.csrf(token))) != 1 {
			fail(w, forbidden("会话校验失败，请刷新页面"))
			return
		}
		if s.pendingRestore.Load() && r.Method != "GET" && r.URL.Path != "/api/restore/cancel" && r.URL.Path != "/api/backup/export" && r.URL.Path != "/api/logout" {
			fail(w, conflict("已安排恢复，请重启服务或在备份管理中取消"))
			return
		}
		if r.URL.Path == "/api/slicing" || strings.HasPrefix(r.URL.Path, "/api/slicing/") || r.URL.Path == "/api/team-projects" || strings.HasPrefix(r.URL.Path, "/api/team-projects/") {
			fail(w, &apiError{Code: http.StatusGone, Message: "模型工作台已在 Beta 中移除，请使用 Bambu Studio 切片后上传 3MF"})
			return
		}
		var result any = map[string]any{"ok": true}
		switch r.Method + " " + r.URL.Path {
		case "GET /api/diagnostics":
			var report DiagnosticReport
			report, err = s.Diagnostics(r.Context(), u)
			if err == nil {
				if s.secureCookie(r) {
					for i := range report.Checks {
						if report.Checks[i].ID == "secure_cookie" {
							report.Checks[i].Status = "ok"
							report.Checks[i].Detail = "当前请求会采用 Secure Cookie；未读取或导出会话值。"
							report.Checks[i].Fix = ""
						}
					}
				}
				result = LocalizeDiagnosticReport(report, requestLanguage(r.Header.Get("Accept-Language")))
			}
		case "POST /api/files/inspect":
			s.inspectFile(w, r)
			return
		case "POST /api/upload/batch":
			s.upload(w, r, u)
			return
		case "GET /api/jobs/import-receipt":
			result, err = s.ImportReceipt(u, r.URL.Query().Get("requestId"))
		case "GET /api/notifications":
			before, _ := strconv.ParseInt(r.URL.Query().Get("before"), 10, 64)
			result, err = s.notificationPage(u, max(int64(0), before), r.URL.Query().Get("unread") == "1")
		case "POST /api/notifications/read":
			var in struct {
				ID      int64 `json:"id"`
				Through int64 `json:"through"`
			}
			if err = decode(r, &in); err == nil {
				err = s.readNotifications(u, in.ID, in.Through)
			}
		case "GET /api/account/notification-preferences":
			var preferences NotificationPreferences
			preferences, err = readNotificationPreferences(s.DB, u.ID)
			result = map[string]any{"ok": true, "preferences": preferences}
		case "POST /api/account/notification-preferences":
			var in NotificationPreferences
			// A page left open before the new category existed has no started
			// field. Preserve that preference rather than silently opting out.
			var current NotificationPreferences
			if current, err = readNotificationPreferences(s.DB, u.ID); err == nil {
				in.Started = current.Started
				if err = decode(r, &in); err == nil {
					err = s.saveNotificationPreferences(u, in)
				}
			}
		case "POST /api/spools/alert":
			var in struct {
				ID        string `json:"id"`
				Threshold int    `json:"threshold"`
				Revision  int64  `json:"revision"`
			}
			if err = decode(r, &in); err == nil {
				err = s.saveStockThreshold(u, in.ID, in.Threshold, in.Revision)
			}
		case "GET /api/account/email":
			result, err = s.emailStatus(u)
		case "POST /api/account/email/request":
			var in struct {
				Email string `json:"email"`
			}
			if err = decode(r, &in); err == nil {
				err = s.requestEmail(u, in.Email)
			}
		case "POST /api/account/email/verify":
			var in struct {
				Code string `json:"code"`
			}
			if err = decode(r, &in); err == nil {
				err = s.verifyEmail(u, in.Code)
			}
		case "POST /api/account/email/preferences":
			var in struct {
				Enabled bool `json:"enabled"`
			}
			if err = decode(r, &in); err == nil {
				err = s.emailPreference(u, in.Enabled, false)
			}
		case "POST /api/account/email/unlink":
			err = s.emailPreference(u, false, true)
		case "GET /api/workspace":
			snapshot, e := s.Snapshot(u)
			err = e
			if err == nil {
				snapshot["ok"] = true
				snapshot["csrf"] = s.csrf(token)
				result = snapshot
			}
		case "GET /api/me":
			result = map[string]any{"ok": true, "user": u, "csrf": s.csrf(token)}
		case "GET /api/jobs/slice":
			id, _ := strconv.ParseInt(r.URL.Query().Get("jobId"), 10, 64)
			var info SliceInfo
			info, err = s.JobSlice(u, id)
			result = map[string]any{"ok": true, "slice": info}
		case "GET /api/jobs/thumbnail":
			id, _ := strconv.ParseInt(r.URL.Query().Get("jobId"), 10, 64)
			plate, _ := strconv.Atoi(r.URL.Query().Get("plate"))
			image, e := s.SliceThumbnail(u, id, plate)
			if e != nil {
				fail(w, e)
				return
			}
			w.Header().Set("Content-Type", "image/png")
			w.Header().Set("Cache-Control", "no-store")
			w.Write(image)
			return
		case "POST /api/jobs/options":
			var in struct {
				JobID   int64        `json:"jobId"`
				Options PrintOptions `json:"options"`
			}
			err = decode(r, &in)
			if err == nil {
				err = s.SavePrintOptions(u, in.JobID, in.Options)
			}
		case "GET /api/printers/commands":
			id, _ := strconv.ParseInt(r.URL.Query().Get("printerId"), 10, 64)
			var records []CommandRecord
			records, err = s.CommandHistory(u, id)
			result = map[string]any{"ok": true, "commands": records}
		case "GET /api/printers/camera":
			s.cameraHTTP(w, r)
			return
		case "POST /api/printers/control":
			var in ControlInput
			err = decode(r, &in)
			if err == nil {
				err = s.ControlPrinter(u, in)
			}
		case "POST /api/printers/motion":
			var in MotionInput
			if err = decode(r, &in); err == nil {
				err = s.MovePrinter(u, in)
			}
		case "POST /api/printers/orphans/settle":
			var in OrphanCompletionInput
			if err = decode(r, &in); err == nil {
				err = s.SettleOrphanCompletion(u, in)
			}
		case "POST /api/printers/orphans/review":
			var in OrphanReview
			if err = decode(r, &in); err == nil {
				err = s.ReviewOrphan(u, in)
			}
		case "POST /api/printers/settings":
			var in SettingInput
			err = decode(r, &in)
			if err == nil {
				err = s.SetEquipment(u, in)
			}
		case "GET /api/account/language":
			result = map[string]any{"ok": true, "language": accountLanguage(s.DB, u.ID)}
		case "POST /api/account/language":
			var in struct {
				Language string `json:"language"`
			}
			if err = decode(r, &in); err == nil {
				err = s.setLanguage(u, in.Language)
			}
		case "GET /api/account/sessions":
			result, err = s.accountSessions(u, token)
		case "POST /api/account/sessions/revoke":
			var in SessionRevoke
			if err = decode(r, &in); err == nil {
				var count int64
				count, err = s.revokeAccountSessions(u, token, in)
				result = map[string]any{"ok": true, "revoked": count}
			}
		case "GET /api/users/role-events":
			var before int64
			if raw := r.URL.Query().Get("before"); raw != "" {
				before, err = strconv.ParseInt(raw, 10, 64)
				if err != nil || before <= 0 {
					err = invalid("记录游标无效")
					break
				}
			}
			result, err = s.roleEvents(u, before)
		case "POST /api/account/password":
			var in struct {
				CurrentPassword string `json:"currentPassword"`
				NewPassword     string `json:"newPassword"`
			}
			err = decode(r, &in)
			if err == nil {
				err = s.ChangePassword(u, in.CurrentPassword, in.NewPassword)
			}
		case "POST /api/users/update":
			var in MemberUpdate
			err = decode(r, &in)
			if err == nil {
				err = s.UpdateMember(u, in)
			}
		case "POST /api/users/transfer-owner":
			var in OwnershipTransfer
			if err = decode(r, &in); err == nil {
				err = s.TransferOwnership(u, in)
			}
		case "POST /api/backup/export":
			s.exportBackupHTTP(w, r, u)
			return
		case "GET /api/backup/status":
			if err = requireOwner(s.DB, u.ID); err != nil {
				break
			}
			var last map[string]string
			s.restoreMu.Lock()
			previewReady := s.restoreTicket != ""
			s.restoreMu.Unlock()
			if raw, e := os.ReadFile(filepath.Join(s.Config.DataDir, "restore-result.json")); e == nil {
				_ = json.Unmarshal(raw, &last)
			}
			if last != nil {
				last["previous"] = filepath.Base(last["previous"])
			}
			result = map[string]any{"ok": true, "pending": s.pendingRestore.Load(), "previewReady": previewReady, "lastRestore": last}
		case "POST /api/restore/preview":
			s.stageRestoreHTTP(w, r, u)
			return
		case "POST /api/restore/confirm":
			var in struct {
				Ticket   string `json:"ticket"`
				Password string `json:"password"`
			}
			err = decode(r, &in)
			if err == nil {
				err = s.confirmRestore(u, in.Ticket, in.Password)
			}
		case "POST /api/restore/cancel":
			err = s.cancelRestore(u)
		case "POST /api/logout":
			s.writes.Lock()
			_, err = s.DB.Exec("DELETE FROM sessions WHERE hash=?", tokenHash(token))
			s.writes.Unlock()
			http.SetCookie(w, &http.Cookie{Name: "layerhub_sid", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: s.secureCookie(r)})
		case "POST /api/users":
			var in struct {
				Username, Name, Password string
				Admin                    bool
			}
			err = decode(r, &in)
			if err == nil {
				err = s.SaveUser(u, in.Username, in.Name, in.Password, in.Admin)
			}
		case "POST /api/printers":
			var in PrinterInput
			err = decode(r, &in)
			if err == nil {
				err = s.SavePrinter(u, in)
			}
		case "POST /api/spools":
			var in Spool
			err = decode(r, &in)
			if err == nil {
				err = s.SaveSpool(u, in)
			}
		case "POST /api/jobs/reorder":
			var in QueueMove
			if err = decode(r, &in); err == nil {
				err = s.MoveQueueJob(u, in)
			}
		case "POST /api/jobs/schedule":
			var in Schedule
			err = decode(r, &in)
			if err == nil {
				err = s.ScheduleJob(u, in)
			}
		case "GET /api/jobs/settlement":
			id, _ := strconv.ParseInt(r.URL.Query().Get("jobId"), 10, 64)
			var settlement Settlement
			settlement, err = s.JobSettlement(u, id)
			result = map[string]any{"ok": true, "job": settlement.Job, "rolls": settlement.Rolls}
		case "POST /api/jobs/settle":
			var in SettlementWrite
			if err = decode(r, &in); err == nil {
				err = s.SettleJob(u, in)
			}
		case "POST /api/jobs/dispatch", "POST /api/jobs/send", "POST /api/jobs/start", "POST /api/jobs/delete", "POST /api/jobs/cancel", "POST /api/jobs/complete":
			var in struct {
				JobID  int64          `json:"jobId"`
				Used   int            `json:"used"`
				Actual map[string]int `json:"actual"`
			}
			err = decode(r, &in)
			if err == nil {
				switch r.URL.Path {
				case "/api/jobs/dispatch":
					err = s.DispatchJob(u, in.JobID)
				case "/api/jobs/send":
					err = s.SendJob(u, in.JobID)
				case "/api/jobs/start":
					err = s.StartJob(u, in.JobID)
				case "/api/jobs/cancel":
					err = s.CancelJob(u, in.JobID)
				case "/api/jobs/delete":
					err = s.DeleteJob(u, in.JobID)
				case "/api/jobs/complete":
					err = s.CompleteJobUses(u, in.JobID, in.Used, in.Actual)
				}
			}
		case "POST /api/upload":
			s.upload(w, r, u)
			return
		default:
			err = missing("接口不存在")
		}
		if err != nil {
			fail(w, err)
			return
		}
		reply(w, 200, result)
	})
	static := http.FileServer(http.Dir(s.Config.WebDir))
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if strings.Contains(r.URL.Path, "\\") || strings.Contains(r.URL.Path, "\x00") {
			http.NotFound(w, r)
			return
		}
		clean := filepath.Clean(filepath.FromSlash(r.URL.Path))
		rel := strings.TrimLeft(clean, "/\\")
		for _, part := range strings.FieldsFunc(rel, func(r rune) bool { return r == '/' || r == '\\' }) {
			if strings.HasPrefix(part, ".") {
				http.NotFound(w, r)
				return
			}
		}
		path := filepath.Join(s.Config.WebDir, rel)
		info, err := os.Stat(path)
		if err != nil || info.IsDir() && r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		static.ServeHTTP(w, r)
	}))
	// Reject cross-origin state changes, including login, before credential processing.
	loginGate := make(chan struct{}, 4)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Content-Language", requestLanguage(r.Header.Get("Accept-Language")))
			w.Header().Add("Vary", "Accept-Language")
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("X-Frame-Options", "DENY")
		if r.Method != "GET" && r.Method != "HEAD" {
			if origin := r.Header.Get("Origin"); origin != "" {
				parsed, err := url.Parse(origin)
				if err != nil || parsed.Host != r.Host {
					fail(w, forbidden("拒绝跨站请求"))
					return
				}
			}
		}
		if r.URL.Path == "/api/login" {
			select {
			case loginGate <- struct{}{}:
				defer func() { <-loginGate }()
			default:
				fail(w, &apiError{429, "登录请求较多，请稍后重试"})
				return
			}
		}
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("request panic: %T", recovered)
				fail(w, fmt.Errorf("request panic"))
			}
		}()
		mux.ServeHTTP(w, r)
	})
}
func (s *Service) upload(w http.ResponseWriter, r *http.Request, u User) {
	r.Body = http.MaxBytesReader(w, r.Body, s.Config.MaxUpload+(1<<20))
	mr, err := r.MultipartReader()
	if err != nil {
		fail(w, invalid("需要 multipart 文件上传"))
		return
	}
	defer r.Body.Close()
	tmp, err := os.CreateTemp(filepath.Join(s.Config.DataDir, "uploads"), "pending-*")
	if err != nil {
		fail(w, err)
		return
	}
	path := tmp.Name()
	keep := false
	defer func() {
		tmp.Close()
		if !keep {
			_ = os.Remove(path)
		}
	}()
	fields := map[string]string{}
	filename := ""
	var bytes int64
	hash := sha256.New()
	for {
		part, e := mr.NextPart()
		if e == io.EOF {
			break
		}
		if e != nil {
			fail(w, invalid("上传中断或请求超过限制"))
			return
		}
		if part.FormName() == "file" && part.FileName() != "" {
			if filename != "" {
				part.Close()
				fail(w, invalid("一次只能上传一个文件"))
				return
			}
			filename = filepath.Base(strings.ReplaceAll(part.FileName(), "\\", "/"))
			if len([]rune(filename)) > 160 {
				part.Close()
				fail(w, invalid("文件名不能超过 160 个字符"))
				return
			}
			ext := strings.ToLower(filepath.Ext(filename))
			if ext != ".3mf" {
				part.Close()
				fail(w, invalid("Beta 仅接受已切片的 3MF 文件，请先在 Bambu Studio 完成切片并导出切片盘文件"))
				return
			}
			bytes, err = io.Copy(io.MultiWriter(tmp, hash), io.LimitReader(part, s.Config.MaxUpload+1))
			if err != nil || bytes == 0 || bytes > s.Config.MaxUpload {
				part.Close()
				fail(w, invalid("文件为空、上传中断或超过大小限制"))
				return
			}
		} else {
			body, e := io.ReadAll(io.LimitReader(part, 65537))
			if e != nil || len(body) > 65536 {
				part.Close()
				fail(w, invalid("表单字段过长"))
				return
			}
			if len(fields) > 12 {
				part.Close()
				fail(w, invalid("表单字段过多"))
				return
			}
			fields[part.FormName()] = string(body)
		}
		part.Close()
	}
	if filename == "" {
		fail(w, invalid("没有收到文件"))
		return
	}
	if err = tmp.Close(); err != nil {
		fail(w, err)
		return
	}
	// The chooser is advisory. Enforce the same sliced-file boundary for API
	// clients before creating a job or reserving any inventory.
	var sliced SliceInfo
	if strings.EqualFold(filepath.Ext(filename), ".3mf") {
		if sliced, err = inspectSlice(path); err != nil {
			fail(w, err)
			return
		}
	}
	if r.URL.Path == "/api/upload/batch" {
		var batch ImportBatch
		if err = json.Unmarshal([]byte(fields["batch"]), &batch); err != nil {
			fail(w, invalid("请确认每个打印盘的设备与耗材安排"))
			return
		}
		receipt, e := s.CreateImportBatch(u, batch, filename, path, hex.EncodeToString(hash.Sum(nil)), fields["note"], bytes)
		if e != nil {
			fail(w, e)
			return
		}
		keep = !receipt.Replayed
		status := 201
		if receipt.Replayed {
			status = 200
		}
		reply(w, status, map[string]any{"ok": true, "receipt": receipt})
		return
	}
	var in Schedule
	if err = json.Unmarshal([]byte(fields["schedule"]), &in); err != nil {
		fail(w, invalid("请确认设备与耗材安排"))
		return
	}
	if len(sliced.Plates) > 0 {
		plate, e := selectedImportPlate(sliced, in.Plate)
		if e != nil {
			fail(w, e)
			return
		}
		in.Plate = plate.ID
	}
	id, err := s.CreateJob(u, in, filename, path, hex.EncodeToString(hash.Sum(nil)), fields["note"], bytes)
	if err != nil {
		fail(w, err)
		return
	}
	keep = true
	reply(w, 201, map[string]any{"ok": true, "jobId": id, "bytes": bytes, "sha256": hex.EncodeToString(hash.Sum(nil))})
}
func EnvConfig() Config {
	max := int64(500 << 20)
	if n, err := strconv.ParseInt(os.Getenv("LAYERHUB_MAX_UPLOAD_MB"), 10, 64); err == nil && n > 0 && n <= 2000 {
		max = n << 20
	}
	value := func(key, def string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}
		return def
	}
	return Config{Address: value("LAYERHUB_ADDRESS", "127.0.0.1:5181"), DataDir: value("LAYERHUB_DATA_DIR", "data"), WebDir: value("LAYERHUB_WEB_DIR", "web"), AdminUser: value("LAYERHUB_ADMIN_USER", "admin"), AdminPassword: os.Getenv("LAYERHUB_ADMIN_PASSWORD"), DeviceMode: value("LAYERHUB_DEVICE_MODE", "offline"), EnablePrintStart: os.Getenv("LAYERHUB_ENABLE_PRINT_START") == "1", CookieSecure: os.Getenv("LAYERHUB_COOKIE_SECURE") == "1", MaxUpload: max, MailToken: os.Getenv("LAYERHUB_MAIL_TOKEN"), PublicURL: os.Getenv("LAYERHUB_PUBLIC_URL")}
}
