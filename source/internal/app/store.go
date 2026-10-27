package app

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const schema = `
PRAGMA journal_mode=WAL;
CREATE TABLE IF NOT EXISTS users(id INTEGER PRIMARY KEY,username TEXT NOT NULL UNIQUE,name TEXT NOT NULL,password_hash TEXT NOT NULL,admin INTEGER NOT NULL DEFAULT 0,disabled INTEGER NOT NULL DEFAULT 0);
CREATE TABLE IF NOT EXISTS sessions(hash TEXT PRIMARY KEY,user_id INTEGER NOT NULL REFERENCES users(id),expires INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS printers(id INTEGER PRIMARY KEY,name TEXT NOT NULL,ip TEXT NOT NULL UNIQUE,serial TEXT NOT NULL UNIQUE,model_id TEXT NOT NULL,configuration TEXT NOT NULL,enabled INTEGER NOT NULL,code BLOB NOT NULL,fingerprint TEXT NOT NULL DEFAULT '');
CREATE TABLE IF NOT EXISTS spools(id TEXT PRIMARY KEY,label TEXT NOT NULL,brand TEXT NOT NULL,material TEXT NOT NULL,color_name TEXT NOT NULL,color TEXT NOT NULL,capacity INTEGER NOT NULL CHECK(capacity>0),remaining INTEGER NOT NULL CHECK(remaining>=0 AND remaining<=capacity),owner_id INTEGER REFERENCES users(id),shared INTEGER NOT NULL,state TEXT NOT NULL,location TEXT NOT NULL,mount_printer INTEGER REFERENCES printers(id),mount_slot TEXT);
CREATE UNIQUE INDEX IF NOT EXISTS spool_slot_unique ON spools(mount_printer,mount_slot) WHERE mount_printer IS NOT NULL;
CREATE TABLE IF NOT EXISTS jobs(id INTEGER PRIMARY KEY,user_id INTEGER NOT NULL REFERENCES users(id),filename TEXT NOT NULL,path TEXT NOT NULL,size INTEGER NOT NULL,sha256 TEXT NOT NULL,printer_id INTEGER REFERENCES printers(id),state TEXT NOT NULL,note TEXT NOT NULL DEFAULT '',created_at INTEGER NOT NULL,queue_order INTEGER NOT NULL,estimate INTEGER NOT NULL,grams INTEGER NOT NULL,spool_id TEXT NOT NULL REFERENCES spools(id),actual_grams INTEGER,remote_path TEXT NOT NULL DEFAULT '',error TEXT NOT NULL DEFAULT '');
CREATE INDEX IF NOT EXISTS jobs_printer_queue ON jobs(printer_id,state,queue_order,id);
CREATE INDEX IF NOT EXISTS jobs_spool_reserve ON jobs(spool_id,state);
CREATE TABLE IF NOT EXISTS printer_commands(id INTEGER PRIMARY KEY,printer_id INTEGER NOT NULL REFERENCES printers(id),job_id INTEGER NOT NULL,user_id INTEGER NOT NULL REFERENCES users(id),action TEXT NOT NULL,state TEXT NOT NULL,message TEXT NOT NULL DEFAULT '',created_at INTEGER NOT NULL,finished_at INTEGER NOT NULL DEFAULT 0);
CREATE UNIQUE INDEX IF NOT EXISTS printer_command_pending ON printer_commands(printer_id) WHERE state='pending';
CREATE TABLE IF NOT EXISTS job_consumption(job_id INTEGER NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,spool_id TEXT NOT NULL REFERENCES spools(id),grams INTEGER NOT NULL CHECK(grams>=0),PRIMARY KEY(job_id,spool_id));
`

func Open(cfg Config, device Device) (*Service, error) {
	var err error
	cfg.DataDir, err = filepath.Abs(cfg.DataDir)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(filepath.Dir(cfg.DataDir), 0700); err != nil {
		return nil, err
	}
	release, err := lockDataDir(cfg.DataDir)
	if err != nil {
		return nil, err
	}
	keepLock := false
	defer func() {
		if !keepLock {
			release()
		}
	}()
	if err = applyPendingRestore(cfg.DataDir); err != nil {
		return nil, err
	}
	if cfg.MaxUpload == 0 {
		cfg.MaxUpload = 500 << 20
	}
	if cfg.AdminUser == "" {
		cfg.AdminUser = "admin"
	}
	if cfg.DeviceMode == "" {
		cfg.DeviceMode = "offline"
	}
	if err := os.MkdirAll(filepath.Join(cfg.DataDir, "uploads"), 0700); err != nil {
		return nil, err
	}
	keyPath := filepath.Join(cfg.DataDir, "master.key")
	key, err := os.ReadFile(keyPath)
	if errors.Is(err, os.ErrNotExist) {
		key = make([]byte, 32)
		if _, err = rand.Read(key); err != nil {
			return nil, err
		}
		if err = os.WriteFile(keyPath, key, 0600); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("master.key must be 32 bytes")
	}
	dbPath, err := filepath.Abs(filepath.Join(cfg.DataDir, "layerhub.sqlite"))
	if err != nil {
		return nil, err
	}
	database, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath)+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	database.SetMaxOpenConns(8)
	if _, err = database.Exec(schema); err != nil {
		database.Close()
		return nil, err
	}
	columns, err := database.Query("PRAGMA table_info(users)")
	if err != nil {
		database.Close()
		return nil, err
	}
	hasDisabled := false
	for columns.Next() {
		var id, required, pk int
		var name, kind string
		var def any
		if err = columns.Scan(&id, &name, &kind, &required, &def, &pk); err != nil {
			columns.Close()
			database.Close()
			return nil, err
		}
		if name == "disabled" {
			hasDisabled = true
		}
	}
	err = columns.Err()
	columns.Close()
	if err != nil {
		database.Close()
		return nil, err
	}
	if !hasDisabled {
		if _, err = database.Exec("ALTER TABLE users ADD COLUMN disabled INTEGER NOT NULL DEFAULT 0"); err != nil {
			database.Close()
			return nil, err
		}
	}
	jobColumns, e := database.Query("PRAGMA table_info(jobs)")
	if e != nil {
		database.Close()
		return nil, e
	}
	hasOptions := false
	for jobColumns.Next() {
		var cid, required, pk int
		var name, kind string
		var def any
		if e = jobColumns.Scan(&cid, &name, &kind, &required, &def, &pk); e != nil {
			break
		}
		if name == "print_options" {
			hasOptions = true
		}
	}
	if e == nil {
		e = jobColumns.Err()
	}
	jobColumns.Close()
	if e == nil && !hasOptions {
		_, e = database.Exec("ALTER TABLE jobs ADD COLUMN print_options TEXT NOT NULL DEFAULT ''")
	}
	if e != nil {
		database.Close()
		return nil, e
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Service{DB: database, Config: cfg, Device: device, key: key, ctx: ctx, cancel: cancel, transfers: make(chan struct{}, 2), releaseLock: release}
	if _, err = database.Exec("UPDATE printer_commands SET state='uncertain',message='服务重启，请核对设备状态',finished_at=? WHERE state='pending'", time.Now().UnixMilli()); err != nil {
		s.Close()
		return nil, err
	}
	var count int
	if err = database.QueryRow("SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		s.Close()
		return nil, err
	}
	if count == 0 {
		password := cfg.AdminPassword
		if password == "" {
			password = randomID(18)
			if err = os.WriteFile(filepath.Join(cfg.DataDir, "bootstrap-admin.txt"), []byte("username="+cfg.AdminUser+"\npassword="+password+"\n"), 0600); err != nil {
				s.Close()
				return nil, err
			}
		}
		hash, e := hashPassword(password)
		if e != nil {
			s.Close()
			return nil, e
		}
		if _, err = database.Exec("INSERT INTO users(username,name,password_hash,admin) VALUES(?,?,?,1)", cfg.AdminUser, cfg.AdminUser, hash); err != nil {
			s.Close()
			return nil, err
		}
	}
	// A process interruption never proves a device accepted the file or a start command.
	if err = initOwnership(database, cfg.AdminUser); err != nil {
		s.Close()
		return nil, err
	}
	if err = initSessionDetails(database); err != nil {
		s.Close()
		return nil, err
	}
	if _, err = database.Exec("UPDATE jobs SET state='queued',error='服务重启，传输需重新确认' WHERE state='uploading'"); err != nil {
		s.Close()
		return nil, err
	}
	if _, err = database.Exec("UPDATE jobs SET state='assigned',error='服务重启，启动结果需核对设备' WHERE state='starting'"); err != nil {
		s.Close()
		return nil, err
	}
	printers, err := s.printers()
	if err != nil {
		s.Close()
		return nil, err
	}
	for i := range printers {
		p, e := s.devicePrinter(printers[i].ID)
		if e != nil {
			s.Close()
			return nil, e
		}
		printers[i] = p
	}
	device.Reconcile(ctx, printers)
	if err = s.initLanguage(); err != nil {
		s.Close()
		return nil, err
	}
	if err = s.initMail(); err != nil {
		s.Close()
		return nil, err
	}
	keepLock = true
	return s, nil
}
func (s *Service) Close() {
	s.closeOnce.Do(func() {
		s.cancel()
		s.Device.Close()
		s.workers.Wait()
		s.DB.Close()
		if s.releaseLock != nil {
			s.releaseLock()
		}
	})
}
func randomID(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func (s *Service) seal(text string) ([]byte, error) {
	block, _ := aes.NewCipher(s.key)
	a, _ := cipher.NewGCM(block)
	nonce := make([]byte, a.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return a.Seal(nonce, nonce, []byte(text), nil), nil
}
func (s *Service) unseal(data []byte) (string, error) {
	block, _ := aes.NewCipher(s.key)
	a, _ := cipher.NewGCM(block)
	if len(data) < a.NonceSize() {
		return "", fmt.Errorf("invalid device credential")
	}
	plain, err := a.Open(nil, data[:a.NonceSize()], data[a.NonceSize():], nil)
	return string(plain), err
}
func (s *Service) transaction(fn func(*sql.Tx) error) error {
	s.writes.Lock()
	defer s.writes.Unlock()
	if s.pendingRestore.Load() {
		return conflict("工作空间已安排恢复，请先重启服务或取消恢复")
	}
	tx, err := s.DB.BeginTx(s.ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

type queryer interface {
	Query(string, ...any) (*sql.Rows, error)
	QueryRow(string, ...any) *sql.Row
}

func readPrinters(q queryer) ([]Printer, error) {
	rows, err := q.Query("SELECT id,name,ip,serial,model_id,configuration,enabled,code,fingerprint FROM printers ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Printer{}
	for rows.Next() {
		var p Printer
		var code []byte
		if err = rows.Scan(&p.ID, &p.Name, &p.IP, &p.Serial, &p.ModelID, &p.Configuration, &p.Enabled, &code, &p.Fingerprint); err != nil {
			return nil, err
		}
		p.Model = models[p.ModelID] + " · 裸机"
		if p.Configuration == "ams" {
			p.Model = models[p.ModelID] + " · " + supply(p.ModelID)
		}
		p.State = "offline"
		p.Remaining = "—"
		p.Color = "blue"
		out = append(out, p)
	}
	return out, rows.Err()
}
func (s *Service) printers() ([]Printer, error) { return readPrinters(s.DB) }
func (s *Service) devicePrinter(id int64) (Printer, error) {
	printers, err := s.printers()
	if err != nil {
		return Printer{}, err
	}
	for _, p := range printers {
		if p.ID == id {
			var encrypted []byte
			if err = s.DB.QueryRow("SELECT code FROM printers WHERE id=?", id).Scan(&encrypted); err != nil {
				return p, err
			}
			p.Code, err = s.unseal(encrypted)
			return p, err
		}
	}
	return Printer{}, missing("打印机不存在")
}
func readSpools(q queryer) ([]Spool, error) {
	base := `SELECT s.id,s.label,s.brand,s.material,s.color_name,s.color,s.capacity,s.remaining,COALESCE(s.owner_id,0),COALESCE(u.username,'team'),s.shared,s.state,s.location,COALESCE(s.mount_printer,0),COALESCE(s.mount_slot,'')`
	tail := ` FROM spools s LEFT JOIN users u ON u.id=s.owner_id`
	rows, err := q.Query(base + `,COALESCE(a.threshold,0),COALESCE(a.revision,0),COALESCE(v.revision,1)` + tail + ` LEFT JOIN spool_alert_settings a ON a.spool_id=s.id LEFT JOIN record_versions v ON v.kind='spool' AND v.record_id=s.id ORDER BY s.rowid`)
	withSettings := true
	if err != nil && strings.Contains(err.Error(), "no such table") {
		withSettings = false
		rows, err = q.Query(base + tail + " ORDER BY s.rowid")
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Spool{}
	for rows.Next() {
		var s Spool
		var pid int64
		var slot string
		s.Revision = 1
		dest := []any{&s.ID, &s.Label, &s.Brand, &s.Material, &s.ColorName, &s.Color, &s.Capacity, &s.Remaining, &s.OwnerID, &s.Owner, &s.Shared, &s.State, &s.Location, &pid, &slot}
		if withSettings {
			dest = append(dest, &s.LowThreshold, &s.AlertRevision, &s.Revision)
		}
		if err = rows.Scan(dest...); err != nil {
			return nil, err
		}
		if pid != 0 {
			s.Mount = &Mount{pid, normalizedSlot(slot)}
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
func readJobs(q queryer) ([]Job, error) {
	base := `SELECT j.id,j.user_id,u.name,j.filename,j.path,j.size,j.sha256,COALESCE(j.printer_id,0),j.state,j.note,j.created_at,j.queue_order,j.estimate,j.grams,j.spool_id,j.actual_grams,j.remote_path,j.error`
	tail := ` FROM jobs j JOIN users u ON u.id=j.user_id ORDER BY j.queue_order,j.id`
	completion := ",COALESCE(c.finished_at,0),COALESCE(c.collected_at,0),COALESCE(c.settled_at,0)"
	joined := ` FROM jobs j JOIN users u ON u.id=j.user_id LEFT JOIN job_completion c ON c.job_id=j.id ORDER BY j.queue_order,j.id`
	rows, err := q.Query(base + ",j.print_options" + completion + joined)
	withCompletion := true
	if err != nil && (strings.Contains(err.Error(), "no such table") || strings.Contains(err.Error(), "no such column")) {
		withCompletion = false
		rows, err = q.Query(base + ",j.print_options" + tail)
	}
	withOptions := true
	if err != nil && strings.Contains(err.Error(), "no such column") {
		withOptions = false
		rows, err = q.Query(base + tail)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Job{}
	for rows.Next() {
		var j Job
		var used sql.NullInt64
		var options string
		dest := []any{&j.ID, &j.UserID, &j.User, &j.Filename, &j.Path, &j.Bytes, &j.SHA256, &j.Printer, &j.State, &j.Note, &j.CreatedAt, &j.Order, &j.Estimate, &j.Grams, &j.SpoolID, &used, &j.Remote, &j.Error}
		if withOptions {
			dest = append(dest, &options)
		}
		if withCompletion {
			dest = append(dest, &j.FinishedAt, &j.CollectedAt, &j.SettledAt)
		}
		if err = rows.Scan(dest...); err != nil {
			return nil, err
		}
		j.Size = float64(j.Bytes) / (1 << 20)
		j.Created = time.UnixMilli(j.CreatedAt).Format("01-02 15:04")
		j.Color = "blue"
		j.DisplayState = j.State
		if j.FinishedAt > 0 && (j.State == "printing" || j.State == "paused") {
			j.DisplayState = "printed"
		}
		if options != "" {
			if err = json.Unmarshal([]byte(options), &j.PrintOptions); err != nil {
				return nil, fmt.Errorf("invalid stored print options")
			}
		}
		if used.Valid {
			n := int(used.Int64)
			j.ActualGrams = &n
		}
		out = append(out, j)
	}
	return out, rows.Err()
}
func spoolVisible(s Spool, u User) bool  { return s.OwnerID == 0 || s.OwnerID == u.ID || s.Shared }
func spoolEditable(s Spool, u User) bool { return s.OwnerID == u.ID || s.OwnerID == 0 && u.Admin }
func reserve(jobs []Job, id string, exclude int64) int {
	total := 0
	for _, j := range jobs {
		if j.ID != exclude && activeStates[j.State] {
			total += jobUses(j)[id]
		}
	}
	return total
}
func trim(value string, n int) string {
	r := []rune(strings.TrimSpace(value))
	if len(r) > n {
		return string(r[:n])
	}
	return string(r)
}
