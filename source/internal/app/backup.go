package app

import (
	"archive/zip"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const maxBackupSize int64 = 8 << 30

type backupFile struct {
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}
type backupManifest struct {
	Format        string                `json:"format"`
	Version       int                   `json:"version"`
	Created       string                `json:"created"`
	Users         int                   `json:"users"`
	Printers      int                   `json:"printers"`
	Spools        int                   `json:"spools"`
	Jobs          int                   `json:"jobs"`
	OwnerUsername string                `json:"ownerUsername,omitempty"`
	Files         map[string]backupFile `json:"files"`
}
type restoreMarker struct {
	Stage    string `json:"stage"`
	Previous string `json:"previous"`
}

func openBackupDB(path string, readOnly bool) (*sql.DB, error) {
	uriPath := filepath.ToSlash(path)
	if !strings.HasPrefix(uriPath, "/") {
		uriPath = "/" + uriPath
	}
	uri := url.URL{Scheme: "file", Path: uriPath}
	query := "?_pragma=foreign_keys(1)&_pragma=trusted_schema(0)"
	if readOnly {
		query += "&mode=ro&_pragma=query_only(1)"
	}
	db, err := sql.Open("sqlite", uri.String()+query)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return db, nil
}
func fileDigest(path string) (backupFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return backupFile{}, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	return backupFile{n, hex.EncodeToString(h.Sum(nil))}, err
}
func addArchiveFile(z *zip.Writer, name, path string) (backupFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return backupFile{}, err
	}
	defer f.Close()
	header := &zip.FileHeader{Name: name, Method: zip.Deflate}
	header.SetMode(0600)
	w, err := z.CreateHeader(header)
	if err != nil {
		return backupFile{}, err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(w, h), f)
	return backupFile{n, hex.EncodeToString(h.Sum(nil))}, err
}
func counts(db *sql.DB, m *backupManifest) error {
	m.OwnerUsername = ""
	has, e := backupHasOwner(db)
	if e != nil {
		return e
	}
	if has {
		if e = db.QueryRow("SELECT username FROM users WHERE owner=1 AND admin=1 AND disabled=0").Scan(&m.OwnerUsername); e != nil {
			return e
		}
	}
	for _, item := range []struct {
		table string
		n     *int
	}{{"users", &m.Users}, {"printers", &m.Printers}, {"spools", &m.Spools}, {"jobs", &m.Jobs}} {
		if err := db.QueryRow("SELECT COUNT(*) FROM " + item.table).Scan(item.n); err != nil {
			return err
		}
	}
	return nil
}

// A consistent SQLite snapshot and only committed job files are packaged. Sessions and bootstrap passwords are omitted.
func (s *Service) ExportBackup(u User, password string) (string, func(), error) {

	s.writes.Lock()
	defer s.writes.Unlock()
	if err := requireOwner(s.DB, u.ID); err != nil {
		return "", nil, err
	}
	if err := checkPassword(s.DB, u.ID, password); err != nil {
		return "", nil, err
	}
	dir, err := os.MkdirTemp(filepath.Dir(s.Config.DataDir), ".layerhub-export-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { os.RemoveAll(dir) }
	okay := false
	defer func() {
		if !okay {
			cleanup()
		}
	}()
	snapshot := filepath.Join(dir, "layerhub.sqlite")
	if _, err = s.DB.Exec("VACUUM INTO ?", snapshot); err != nil {
		return "", nil, err
	}
	db, err := openBackupDB(snapshot, false)
	if err != nil {
		return "", nil, err
	}
	defer db.Close()
	if err = scrubMailBackup(db); err != nil {
		return "", nil, err
	}
	if _, err = db.Exec("PRAGMA journal_mode=DELETE; DELETE FROM sessions; VACUUM;"); err != nil {
		return "", nil, err
	}
	jobs, err := readJobs(db)
	if err != nil {
		return "", nil, err
	}
	manifest := backupManifest{Format: "layerhub-backup", Version: 1, Created: time.Now().UTC().Format(time.RFC3339), Files: map[string]backupFile{}}
	if err = counts(db, &manifest); err != nil {
		return "", nil, err
	}
	archive := filepath.Join(dir, "layerhub-backup.zip")
	out, err := os.OpenFile(archive, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", nil, err
	}
	defer out.Close()
	writer := zip.NewWriter(out)
	defer writer.Close()
	var total int64
	for _, job := range jobs {
		name := fmt.Sprintf("uploads/job-%d.bin", job.ID)
		info, e := addArchiveFile(writer, name, job.Path)
		if e != nil {
			return "", nil, conflict("任务文件缺失或无法读取，备份未完成")
		}
		if info.Size != job.Bytes || info.SHA256 != job.SHA256 {
			return "", nil, conflict("任务文件校验失败，备份未完成")
		}
		total += info.Size
		if total > maxBackupSize {
			return "", nil, invalid("备份超过 8 GiB 限制")
		}
		manifest.Files[name] = info
		if _, err = db.Exec("UPDATE jobs SET path=? WHERE id=?", name, job.ID); err != nil {
			return "", nil, err
		}
	}
	if err = db.Close(); err != nil {
		return "", nil, err
	}
	for name, path := range map[string]string{"layerhub.sqlite": snapshot, "master.key": filepath.Join(s.Config.DataDir, "master.key")} {
		info, e := addArchiveFile(writer, name, path)
		if e != nil {
			return "", nil, e
		}
		manifest.Files[name] = info
	}
	total = 0
	for _, info := range manifest.Files {
		total += info.Size
	}
	if total > maxBackupSize-(32<<20) || len(manifest.Files) > 100002 {
		return "", nil, invalid("备份超过大小或文件数量限制")
	}
	metadata, err := writer.Create("manifest.json")
	if err != nil {
		return "", nil, err
	}
	if err = json.NewEncoder(metadata).Encode(manifest); err != nil {
		return "", nil, err
	}
	if err = writer.Close(); err != nil {
		return "", nil, err
	}
	if err = out.Sync(); err != nil {
		return "", nil, err
	}
	if err = out.Close(); err != nil {
		return "", nil, err
	}
	okay = true
	return archive, cleanup, nil
}

var archiveUpload = regexp.MustCompile(`^uploads/job-[1-9][0-9]*\.bin$`)

func validBackupName(name string) bool {
	return name == "manifest.json" || name == "master.key" || name == "layerhub.sqlite" || archiveUpload.MatchString(name)
}
func unpackBackup(archive, dir string) (backupManifest, error) {
	var m backupManifest
	reader, err := zip.OpenReader(archive)
	if err != nil {
		return m, invalid("不是有效的 ZIP 备份")
	}
	defer reader.Close()
	if len(reader.File) > 100003 {
		return m, invalid("备份文件数量超过限制")
	}
	seen := map[string]bool{}
	var total uint64
	for _, entry := range reader.File {
		if !validBackupName(entry.Name) || seen[entry.Name] || !entry.Mode().IsRegular() {
			return m, invalid("备份包含不允许的路径、重复文件或链接")
		}
		seen[entry.Name] = true
		total += entry.UncompressedSize64
		if entry.UncompressedSize64 > uint64(maxBackupSize) || total > uint64(maxBackupSize) || (entry.Name == "manifest.json" && entry.UncompressedSize64 > 32<<20) {
			return m, invalid("备份解压大小超过限制")
		}
		input, e := entry.Open()
		if e != nil {
			return m, invalid("备份文件损坏")
		}
		target := filepath.Join(dir, filepath.FromSlash(entry.Name))
		if e = os.MkdirAll(filepath.Dir(target), 0700); e != nil {
			input.Close()
			return m, e
		}
		output, e := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			input.Close()
			return m, e
		}
		n, e := io.Copy(output, io.LimitReader(input, int64(entry.UncompressedSize64)+1))
		closeErr := output.Close()
		input.Close()
		if e != nil || closeErr != nil || uint64(n) != entry.UncompressedSize64 {
			return m, invalid("备份内容损坏或大小不一致")
		}
	}
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil || json.Unmarshal(raw, &m) != nil || m.Format != "layerhub-backup" || m.Version != 1 {
		return m, invalid("备份格式或版本不受支持")
	}
	if len(m.Files)+1 != len(seen) || !seen["layerhub.sqlite"] || !seen["master.key"] {
		return m, invalid("备份清单不完整")
	}
	for name, expected := range m.Files {
		if name == "manifest.json" || !validBackupName(name) || !seen[name] {
			return m, invalid("备份清单包含无效文件")
		}
		actual, e := fileDigest(filepath.Join(dir, filepath.FromSlash(name)))
		if e != nil || actual != expected {
			return m, invalid("备份文件校验失败")
		}
	}
	return validateBackup(dir, m)
}
func validateBackup(dir string, m backupManifest) (backupManifest, error) {
	key, err := os.ReadFile(filepath.Join(dir, "master.key"))
	if err != nil || len(key) != 32 {
		return m, invalid("备份缺少有效设备密钥")
	}
	db, err := openBackupDB(filepath.Join(dir, "layerhub.sqlite"), true)
	if err != nil {
		return m, invalid("备份数据库无法打开")
	}
	defer db.Close()
	var integrity string
	var version, admins int
	if db.QueryRow("PRAGMA quick_check").Scan(&integrity) != nil || integrity != "ok" {
		return m, invalid("备份数据库损坏")
	}
	if db.QueryRow("PRAGMA user_version").Scan(&version) != nil || (version != 2 && version != 3 && version != 4 && version != 5) {
		return m, invalid("备份数据库版本不受支持")
	}
	if db.QueryRow("SELECT COUNT(*) FROM users WHERE admin=1 AND disabled=0").Scan(&admins) != nil || admins < 1 {
		return m, invalid("备份没有可用的管理员账户")
	}
	if err := validateBackupOwnership(db, version); err != nil {
		return m, err
	}
	rows, err := db.Query("PRAGMA foreign_key_check")
	if err != nil {
		return m, invalid("备份数据库约束无效")
	}
	broken := rows.Next()
	rows.Close()
	if broken {
		return m, invalid("备份记录关联不完整")
	}
	credentials, err := db.Query("SELECT code FROM printers")
	if err != nil {
		return m, invalid("备份设备记录无效")
	}
	probe := Service{key: key}
	for credentials.Next() {
		var code []byte
		if credentials.Scan(&code) != nil {
			credentials.Close()
			return m, invalid("备份设备记录无效")
		}
		if _, err = probe.unseal(code); err != nil {
			credentials.Close()
			return m, invalid("设备密钥与备份不匹配")
		}
	}
	err = credentials.Err()
	credentials.Close()
	if err != nil {
		return m, invalid("备份设备记录无效")
	}
	jobs, err := readJobs(db)
	if err != nil {
		return m, invalid("备份任务记录无效")
	}
	for _, job := range jobs {
		name := fmt.Sprintf("uploads/job-%d.bin", job.ID)
		info, ok := m.Files[name]
		if !ok || job.Path != name || info.Size != job.Bytes || info.SHA256 != job.SHA256 {
			return m, invalid("备份任务文件与数据库不一致")
		}
	}
	if err = counts(db, &m); err != nil {
		return m, invalid("备份数据表不完整")
	}
	return m, nil
}
func (s *Service) stageRestore(u User, body io.Reader) (backupManifest, string, error) {

	s.restoreMu.Lock()
	defer s.restoreMu.Unlock()
	if err := requireOwner(s.DB, u.ID); err != nil {
		return backupManifest{}, "", err
	}
	if s.pendingRestore.Load() {
		return backupManifest{}, "", conflict("已安排恢复，请先重启或取消")
	}
	ticket := randomID(16)
	dir := s.Config.DataDir + ".restore-stage-" + ticket
	if err := os.Mkdir(dir, 0700); err != nil {
		return backupManifest{}, "", err
	}
	keep := false
	defer func() {
		if !keep {
			os.RemoveAll(dir)
		}
	}()
	archive := filepath.Join(dir, "incoming.zip")
	f, err := os.OpenFile(archive, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return backupManifest{}, "", err
	}
	n, err := io.Copy(f, io.LimitReader(body, maxBackupSize+1))
	f.Close()
	if err != nil || n > maxBackupSize {
		return backupManifest{}, "", invalid("备份上传失败或超过 8 GiB 限制")
	}
	m, err := unpackBackup(archive, dir)
	os.Remove(archive)
	if err != nil {
		return m, "", err
	}
	probe, e := openBackupDB(filepath.Join(dir, "layerhub.sqlite"), false)
	if e != nil {
		return m, "", invalid("备份数据库无法检查所有者")
	}
	has, e := backupHasOwner(probe)
	if e == nil && !has {
		var n int
		e = probe.QueryRow("SELECT COUNT(*) FROM users WHERE username=? AND admin=1 AND disabled=0", s.Config.AdminUser).Scan(&n)
		if e == nil && n != 1 {
			e = invalid("旧备份中缺少配置的初始管理员，无法确定空间所有者；请先升级原系统再导出")
		}
		if e == nil {
			m.OwnerUsername = s.Config.AdminUser
		}
	}
	probe.Close()
	if e != nil {
		return m, "", e
	}
	if s.restoreTicket != "" {
		os.RemoveAll(s.Config.DataDir + ".restore-stage-" + s.restoreTicket)
	}
	s.restoreTicket = ticket
	s.restoreOwner = u.ID
	keep = true
	return m, ticket, nil
}
func (s *Service) confirmRestore(u User, ticket, password string) error {

	s.restoreMu.Lock()
	defer s.restoreMu.Unlock()
	s.writes.Lock()
	defer s.writes.Unlock()
	if err := requireOwner(s.DB, u.ID); err != nil {
		return err
	}
	if s.pendingRestore.Load() {
		return conflict("已安排恢复")
	}
	if s.Config.DeviceMode == "live" {
		return conflict("请先以离线模式启动服务，再恢复备份")
	}
	if ticket == "" || ticket != s.restoreTicket || s.restoreOwner != u.ID {
		return invalid("恢复预览已失效，请重新选择备份")
	}
	if err := checkPassword(s.DB, u.ID, password); err != nil {
		return err
	}
	var active int
	if err := s.DB.QueryRow("SELECT COUNT(*) FROM jobs WHERE state IN ('uploading','starting','printing','paused','stopped','checking')").Scan(&active); err != nil {
		return err
	}
	if active > 0 {
		return conflict("有任务执行中，请先处理完成再恢复")
	}
	marker := restoreMarker{Stage: s.Config.DataDir + ".restore-stage-" + ticket, Previous: s.Config.DataDir + ".before-restore-" + time.Now().Format("20060102-150405") + "-" + randomID(4)}
	var hasCommands int
	if err := s.DB.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='printer_commands'").Scan(&hasCommands); err != nil {
		return err
	}
	if hasCommands > 0 {
		if err := s.DB.QueryRow("SELECT COUNT(*) FROM printer_commands WHERE state IN ('pending','uncertain')").Scan(&active); err != nil {
			return err
		}
		if active > 0 {
			return conflict("请先等待或核对设备指令，再恢复备份")
		}
	}
	raw, _ := json.Marshal(marker)
	temporary := s.Config.DataDir + ".restore.json.tmp"
	if err := os.WriteFile(temporary, raw, 0600); err != nil {
		return err
	}
	if err := os.Rename(temporary, s.Config.DataDir+".restore.json"); err != nil {
		return err
	}
	s.pendingRestore.Store(true)
	return nil
}
func (s *Service) cancelRestore(u User) error {

	s.restoreMu.Lock()
	defer s.restoreMu.Unlock()
	s.writes.Lock()
	defer s.writes.Unlock()
	if err := requireOwner(s.DB, u.ID); err != nil {
		return err
	}
	if err := os.Remove(s.Config.DataDir + ".restore.json"); err != nil && !os.IsNotExist(err) {
		return err
	}
	if s.restoreTicket != "" {
		os.RemoveAll(s.Config.DataDir + ".restore-stage-" + s.restoreTicket)
	}
	s.restoreTicket = ""
	s.pendingRestore.Store(false)
	return nil
}

// Runs only while holding the cross-process data lock, before opening SQLite.
func applyPendingRestore(dataDir string) error {
	markerPath := dataDir + ".restore.json"
	raw, err := os.ReadFile(markerPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var marker restoreMarker
	if json.Unmarshal(raw, &marker) != nil || !strings.HasPrefix(marker.Stage, dataDir+".restore-stage-") || filepath.Dir(marker.Stage) != filepath.Dir(dataDir) || !strings.HasPrefix(marker.Previous, dataDir+".before-restore-") || filepath.Dir(marker.Previous) != filepath.Dir(dataDir) {
		return fmt.Errorf("invalid restore journal")
	}
	exists := func(path string) bool { _, err := os.Stat(path); return err == nil }
	if exists(dataDir) && exists(marker.Previous) {
		os.RemoveAll(marker.Stage)
		return os.Remove(markerPath)
	}
	raw, err = os.ReadFile(filepath.Join(marker.Stage, "manifest.json"))
	var m backupManifest
	if err != nil || json.Unmarshal(raw, &m) != nil {
		return fmt.Errorf("restore staging directory missing or invalid")
	}
	for name, expected := range m.Files {
		if !validBackupName(name) || name == "manifest.json" {
			return fmt.Errorf("invalid restore manifest")
		}
		actual, e := fileDigest(filepath.Join(marker.Stage, filepath.FromSlash(name)))
		if e != nil || actual != expected {
			return fmt.Errorf("staged backup checksum mismatch")
		}
	}
	if _, err = validateBackup(marker.Stage, m); err != nil {
		return err
	}
	// Keep the checked source untouched, so preparation can be retried after a power interruption.
	ready := marker.Stage + ".ready"
	if err = os.RemoveAll(ready); err != nil {
		return err
	}
	if err = os.Mkdir(ready, 0700); err != nil {
		return err
	}
	for name := range m.Files {
		source, e := os.Open(filepath.Join(marker.Stage, filepath.FromSlash(name)))
		if e != nil {
			return e
		}
		target := filepath.Join(ready, filepath.FromSlash(name))
		if e = os.MkdirAll(filepath.Dir(target), 0700); e != nil {
			source.Close()
			return e
		}
		out, e := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			source.Close()
			return e
		}
		_, e = io.Copy(out, source)
		source.Close()
		syncErr := out.Sync()
		closeErr := out.Close()
		if e != nil {
			return e
		}
		if syncErr != nil {
			return syncErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	db, err := openBackupDB(filepath.Join(ready, "layerhub.sqlite"), false)
	if err != nil {
		return err
	}
	jobs, err := readJobs(db)
	if err == nil {
		for _, job := range jobs {
			_, err = db.Exec("UPDATE jobs SET path=? WHERE id=?", filepath.Join(dataDir, "uploads", fmt.Sprintf("job-%d.bin", job.ID)), job.ID)
			if err != nil {
				break
			}
		}
	}
	if err == nil {
		err = scrubMailBackup(db)
	}
	if err == nil {
		_, err = db.Exec("DELETE FROM sessions; UPDATE printers SET enabled=0; UPDATE jobs SET state='queued',remote_path='',error='备份恢复后请重新确认传输' WHERE state IN ('uploading','assigned','starting');")
		if err == nil {
			_, err = db.Exec("UPDATE jobs SET state='stopped',error='备份恢复后请核对设备并登记历史实际用量' WHERE state IN ('printing','paused','checking');")
		}
		var hasCommands int
		if err == nil {
			err = db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='printer_commands'").Scan(&hasCommands)
		}
		if err == nil && hasCommands > 0 {
			_, err = db.Exec("UPDATE printer_commands SET state='restored',message='从备份恢复的历史指令，执行状态需重新核对' WHERE state IN ('pending','uncertain')")
		}
	}
	db.Close()
	if err != nil {
		return err
	}
	// The prepared directory is complete before the old directory is renamed. Preserve the old directory for rollback.
	if exists(dataDir) {
		if exists(marker.Previous) {
			return fmt.Errorf("restore rollback directory already exists")
		}
		if err = os.Rename(dataDir, marker.Previous); err != nil {
			return err
		}
	}
	if err = os.Rename(ready, dataDir); err != nil {
		if !exists(dataDir) && exists(marker.Previous) {
			_ = os.Rename(marker.Previous, dataDir)
		}
		return err
	}
	if err = os.Remove(markerPath); err != nil {
		return err
	}
	os.RemoveAll(marker.Stage)
	return os.WriteFile(filepath.Join(dataDir, "restore-result.json"), []byte(fmt.Sprintf("{\"previous\":%q,\"restored\":%q}", marker.Previous, time.Now().Format(time.RFC3339))), 0600)
}
func (s *Service) exportBackupHTTP(w http.ResponseWriter, r *http.Request, u User) {
	var in struct {
		Password string `json:"password"`
	}
	if err := decode(r, &in); err != nil {
		fail(w, err)
		return
	}
	path, cleanup, err := s.ExportBackup(u, in.Password)
	if err != nil {
		fail(w, err)
		return
	}
	defer cleanup()
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Disposition", `attachment; filename="layerhub-backup-`+time.Now().Format("20060102-150405")+`.zip"`)
	http.ServeFile(w, r, path)
}
func (s *Service) stageRestoreHTTP(w http.ResponseWriter, r *http.Request, u User) {
	defer r.Body.Close()
	r.Body = http.MaxBytesReader(w, r.Body, maxBackupSize)
	m, ticket, err := s.stageRestore(u, r.Body)
	if err != nil {
		fail(w, err)
		return
	}
	reply(w, 200, map[string]any{"ok": true, "ticket": ticket, "preview": map[string]any{"created": m.Created, "users": m.Users, "printers": m.Printers, "spools": m.Spools, "jobs": m.Jobs}})
}
