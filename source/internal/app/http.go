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
		reply(w, e.Code, map[string]any{"ok": false, "error": e.Message})
		return
	}
	log.Printf("request failed: %T", err)
	reply(w, 500, map[string]any{"ok": false, "error": "服务器处理失败，请查看服务日志"})
}
func decode(r *http.Request, target any) error {
	defer r.Body.Close()
	limit := int64(64 << 10)
	if r.URL.Path == "/api/slicing/project" || r.URL.Path == "/api/slicing/draft" {
		limit = 128 << 10
	}
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
		var result any = map[string]any{"ok": true}
		switch r.Method + " " + r.URL.Path {
		case "GET /api/slicing":
			result, err = s.sliceWorkspace(u)
		case "GET /api/slicing/compare":
			var ids []int64
			if ids, err = comparisonIDs(r.URL.Query().Get("ids")); err == nil {
				var comparison SliceComparison
				comparison, err = s.compareSlices(r.Context(), u, ids)
				result = map[string]any{"ok": true, "entries": comparison.Entries, "sameGeometry": comparison.SameGeometry}
			}
		case "GET /api/slicing/task":
			id, _ := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
			var t SliceTask
			t, err = s.slicerTask(u, id)
			result = map[string]any{"ok": true, "task": t}
		case "GET /api/slicing/projects":
			result, err = s.slicerStorage(u)
		case "GET /api/team-projects":
			var before int64
			if raw := r.URL.Query().Get("before"); raw != "" {
				before, err = strconv.ParseInt(raw, 10, 64)
				if err != nil || before <= 0 {
					err = invalid("工程游标无效")
					break
				}
			}
			result, err = s.teamProjects(u, r.URL.Query().Get("scope"), strings.TrimSpace(r.URL.Query().Get("q")), before)
		case "GET /api/team-projects/detail":
			id, e := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
			var version int64
			if raw := r.URL.Query().Get("version"); raw != "" {
				version, err = strconv.ParseInt(raw, 10, 64)
			}
			if e != nil || id <= 0 || err != nil || version < 0 {
				err = invalid("共享工程或版本编号无效")
				break
			}
			result, err = s.teamProjectDetail(u, id, version)
		case "POST /api/team-projects/publish":
			var in TeamPublish
			if err = decode(r, &in); err == nil {
				var id int64
				var replayed bool
				id, replayed, err = s.publishTeamProject(u, in)
				var shared int64
				if err == nil {
					err = s.DB.QueryRow("SELECT project_id FROM team_project_versions WHERE id=?", id).Scan(&shared)
				}
				result = map[string]any{"ok": true, "versionId": id, "sharedId": shared, "replayed": replayed}
			}
		case "POST /api/team-projects/copy":
			var in TeamCopy
			if err = decode(r, &in); err == nil {
				var p SlicerProject
				var replayed bool
				p, replayed, err = s.copyTeamProject(u, in)
				result = map[string]any{"ok": true, "project": p, "replayed": replayed}
			}
		case "POST /api/team-projects/visibility":
			var in TeamVisibility
			if err = decode(r, &in); err == nil {
				err = s.setTeamVisibility(u, in)
			}
		case "GET /api/slicing/presets":
			var presets []ProcessPreset
			presets, err = s.processPresets(u)
			result = map[string]any{"ok": true, "presets": presets, "machine": "X2D · 0.4 mm"}
		case "POST /api/slicing/presets":
			var in PresetWrite
			if err = decode(r, &in); err == nil {
				var preset ProcessPreset
				preset, err = s.saveProcessPreset(u, in)
				result = map[string]any{"ok": true, "preset": preset}
			}
		case "POST /api/slicing/presets/archive":
			var in PresetArchive
			if err = decode(r, &in); err == nil {
				err = s.archiveProcessPreset(u, in)
			}
		case "GET /api/slicing/project":
			id, _ := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
			var p SlicerProject
			p, err = s.slicerProject(u, id)
			var origin any
			if err == nil {
				origin, err = s.teamCopyOrigin(u, id)
			}
			result = map[string]any{"ok": true, "project": p, "teamOrigin": origin}
		case "POST /api/slicing/project":
			var in ProjectWrite
			if err = decode(r, &in); err == nil {
				var p SlicerProject
				p, err = s.saveSlicerProject(u, in)
				result = map[string]any{"ok": true, "project": p}
			}
		case "POST /api/slicing/delete-project":
			var in struct {
				ID       int64 `json:"id"`
				Revision int64 `json:"revision"`
			}
			if err = decode(r, &in); err == nil {
				err = s.deleteSlicerProject(u, in.ID, in.Revision)
			}
		case "POST /api/slicing/draft", "POST /api/slicing/clear-draft":
			var in DraftWrite
			if err = decode(r, &in); err == nil {
				var rev int64
				rev, err = s.saveSlicerDraft(u, in, r.URL.Path == "/api/slicing/clear-draft")
				result = map[string]any{"ok": true, "revision": rev}
			}
		case "GET /api/slicing/layers":
			result, err = s.toolpathRequest(r.Context(), u, r.URL.Query().Get("id"), r.URL.Query().Get("layer"))
			w.Header().Set("Cache-Control", "no-store")
		case "POST /api/slicing/models":
			s.uploadModel(w, r, u)
			return
		case "POST /api/slicing/import-3mf":
			s.importModel3MF(w, r, u)
			return
		case "GET /api/slicing/model", "GET /api/slicing/output", "GET /api/slicing/preview":
			id, _ := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
			s.sliceFile(w, r, u, strings.TrimPrefix(r.URL.Path, "/api/slicing/"), id)
			return
		case "POST /api/slicing/tasks":
			var in struct {
				ClientVersion int        `json:"clientVersion"`
				ModelID       int64      `json:"modelId"`
				Options       SliceSetup `json:"options"`
			}
			if err = decode(r, &in); err == nil {
				var id int64
				if in.ClientVersion != 2 {
					err = conflict("切片工作台已升级，请刷新页面后重新检查对象工艺")
				} else {
					id, err = s.queueSlice(u, in.ModelID, in.Options)
				}
				result = map[string]any{"ok": true, "taskId": id}
			}
		case "POST /api/slicing/cancel", "POST /api/slicing/delete-model":
			var in struct {
				ID int64 `json:"id"`
			}
			if err = decode(r, &in); err == nil {
				if r.URL.Path == "/api/slicing/cancel" {
					err = s.cancelSlice(u, in.ID)
				} else {
					err = s.deleteSliceModel(u, in.ID)
				}
			}
		case "POST /api/slicing/promote":
			var in struct {
				ID        int64  `json:"id"`
				PrinterID int64  `json:"printerId"`
				SpoolID   string `json:"spoolId"`
			}
			if err = decode(r, &in); err == nil {
				var id int64
				id, err = s.promoteSlice(u, in.ID, in.PrinterID, in.SpoolID)
				result = map[string]any{"ok": true, "jobId": id}
			}
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
		case "POST /api/printers/settings":
			var in SettingInput
			err = decode(r, &in)
			if err == nil {
				err = s.SetEquipment(u, in)
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
		case "POST /api/jobs/dispatch", "POST /api/jobs/send", "POST /api/jobs/start", "POST /api/jobs/delete", "POST /api/jobs/complete":
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
			if ext != ".gcode" && ext != ".3mf" {
				part.Close()
				fail(w, invalid("只接受 .gcode 或 .3mf 文件"))
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
	var in Schedule
	if err = json.Unmarshal([]byte(fields["schedule"]), &in); err != nil {
		fail(w, invalid("请确认设备与耗材安排"))
		return
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
	return Config{SlicerPath: os.Getenv("LAYERHUB_SLICER_PATH"), SlicerProfiles: os.Getenv("LAYERHUB_SLICER_PROFILES"), Address: value("LAYERHUB_ADDRESS", "127.0.0.1:5181"), DataDir: value("LAYERHUB_DATA_DIR", "data"), WebDir: value("LAYERHUB_WEB_DIR", "web"), AdminUser: value("LAYERHUB_ADMIN_USER", "admin"), AdminPassword: os.Getenv("LAYERHUB_ADMIN_PASSWORD"), DeviceMode: value("LAYERHUB_DEVICE_MODE", "offline"), EnablePrintStart: os.Getenv("LAYERHUB_ENABLE_PRINT_START") == "1", CookieSecure: os.Getenv("LAYERHUB_COOKIE_SECURE") == "1", MaxUpload: max, MailToken: os.Getenv("LAYERHUB_MAIL_TOKEN"), PublicURL: os.Getenv("LAYERHUB_PUBLIC_URL")}
}
