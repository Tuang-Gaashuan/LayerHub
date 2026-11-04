package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Inspection records are account-private drafts. They never schedule printers.
const inspectionSchema = `CREATE TABLE IF NOT EXISTS file_inspections(id TEXT PRIMARY KEY,user_id INTEGER NOT NULL REFERENCES users(id),filename TEXT NOT NULL,path TEXT NOT NULL,digest TEXT NOT NULL,bytes INTEGER NOT NULL,state TEXT NOT NULL,review TEXT NOT NULL DEFAULT '',error TEXT NOT NULL DEFAULT '',created INTEGER NOT NULL);`

func (s *Service) initInspections() error {
	if _, e := s.DB.Exec(inspectionSchema); e != nil {
		return e
	}
	if _, e := s.DB.Exec("UPDATE file_inspections SET state='queued' WHERE state='parsing'"); e != nil {
		return e
	}
	if s.Config.DisableBackgroundEvents {
		return nil
	}
	s.workers.Add(1)
	go func() {
		defer s.workers.Done()
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			select {
			case <-s.ctx.Done():
				return
			case <-tick.C:
				if !s.pendingRestore.Load() {
					s.runInspection()
				}
			}
		}
	}()
	return nil
}
func (s *Service) queueInspection(u User, name, path, digest string, size int64) (string, error) {
	id := randomID(20)
	err := s.transaction(func(tx *sql.Tx) error {
		var n int
		if e := tx.QueryRow("SELECT count(*) FROM file_inspections WHERE user_id=?", u.ID).Scan(&n); e != nil {
			return e
		}
		if n >= 8 {
			return conflict("后台文件已达 8 个，请先清理已完成文件")
		}
		_, e := tx.Exec("INSERT INTO file_inspections(id,user_id,filename,path,digest,bytes,state,created) VALUES(?,?,?,?,?,?,'queued',?)", id, u.ID, name, path, digest, size, time.Now().Unix())
		return e
	})
	return id, err
}
func (s *Service) runInspection() {
	// Also exclude the legacy synchronous parser and uploads from the heavy scan.
	if !s.fileInspectBusy.CompareAndSwap(false, true) {
		return
	}
	defer s.fileInspectBusy.Store(false)
	var id, path, digest string
	var size int64
	if s.DB.QueryRow("SELECT id,path,digest,bytes FROM file_inspections WHERE state='queued' ORDER BY created,id LIMIT 1").Scan(&id, &path, &digest, &size) != nil {
		return
	}
	claim, e := s.DB.Exec("UPDATE file_inspections SET state='parsing' WHERE id=? AND state='queued'", id)
	if e != nil {
		return
	}
	changed, _ := claim.RowsAffected()
	if changed != 1 {
		return
	}
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Minute)
	defer cancel()
	review, e := inspectPrintFile(ctx, path, digest, size)
	if s.ctx.Err() != nil {
		s.DB.Exec("UPDATE file_inspections SET state='queued' WHERE id=?", id)
		return
	}
	if e != nil {
		s.DB.Exec("UPDATE file_inspections SET state='failed',error=? WHERE id=?", "文件解析失败，请核对是否为有效切片 3MF", id)
		return
	}
	raw, e := json.Marshal(review)
	if e != nil {
		return
	}
	s.DB.Exec("UPDATE file_inspections SET state='ready',review=?,error='' WHERE id=?", string(raw), id)
}
func (s *Service) inspections(u User, id string) (map[string]any, error) {
	rows, e := s.DB.Query("SELECT id,filename,bytes,state,error,created,review FROM file_inspections WHERE user_id=? AND (?='' OR id=?) ORDER BY created DESC", u.ID, id, id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var key, name, state, note, raw string
		var size, created int64
		if e = rows.Scan(&key, &name, &size, &state, &note, &created, &raw); e != nil {
			return nil, e
		}
		v := map[string]any{"id": key, "filename": name, "bytes": size, "state": state, "error": note, "created": created}
		if id != "" && state == "ready" {
			v["review"] = json.RawMessage(raw)
		}
		out = append(out, v)
	}
	return map[string]any{"ok": true, "inspections": out}, rows.Err()
}

// Copy a retained draft into immutable job storage; draft cleanup cannot remove jobs.
func (s *Service) importInspection(u User, r *http.Request) (map[string]any, error) {
	var in struct {
		InspectionID string      `json:"inspectionId"`
		Batch        ImportBatch `json:"batch"`
		Note         string      `json:"note"`
	}
	if e := decode(r, &in); e != nil {
		return nil, e
	}
	var path, name, digest string
	var size int64
	e := s.DB.QueryRow("SELECT path,filename,digest,bytes FROM file_inspections WHERE id=? AND user_id=? AND state='ready'", in.InspectionID, u.ID).Scan(&path, &name, &digest, &size)
	if e == sql.ErrNoRows {
		return nil, missing("没有找到本账户已完成解析的文件")
	}
	if e != nil {
		return nil, e
	}
	src, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer src.Close()
	dst, e := os.CreateTemp(filepath.Join(s.Config.DataDir, "uploads"), "import-*")
	if e != nil {
		return nil, e
	}
	keep := false
	defer func() {
		dst.Close()
		if !keep {
			os.Remove(dst.Name())
		}
	}()
	n, e := io.Copy(dst, io.LimitReader(src, size+1))
	if e != nil {
		return nil, e
	}
	if n != size {
		return nil, invalid("后台文件已变化，请重新上传")
	}
	if e = dst.Close(); e != nil {
		return nil, e
	}
	receipt, e := s.CreateImportBatch(u, in.Batch, name, dst.Name(), digest, in.Note, size)
	if e != nil {
		return nil, e
	}
	keep = !receipt.Replayed
	return map[string]any{"ok": true, "receipt": receipt}, nil
}

func (s *Service) deleteInspection(u User, id string) error {
	var path string
	err := s.transaction(func(tx *sql.Tx) error {
		var state string
		e := tx.QueryRow("SELECT path,state FROM file_inspections WHERE id=? AND user_id=?", id, u.ID).Scan(&path, &state)
		if e == sql.ErrNoRows {
			return missing("文件不存在")
		}
		if e != nil {
			return e
		}
		if state == "parsing" {
			return conflict("文件正在解析，请完成后删除")
		}
		_, e = tx.Exec("DELETE FROM file_inspections WHERE id=? AND user_id=?", id, u.ID)
		return e
	})
	if err == nil {
		os.Remove(path)
	}
	return err
}
