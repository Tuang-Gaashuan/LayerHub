package app

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"regexp"
	"time"
)

const operationSchema = `CREATE TABLE IF NOT EXISTS operation_receipts(user_id INTEGER NOT NULL,operation_id TEXT NOT NULL,digest TEXT NOT NULL,kind TEXT NOT NULL,completed INTEGER NOT NULL,PRIMARY KEY(user_id,operation_id));`

type OperationKey struct{ ID, Digest, Kind string }

func requestOperation(r *http.Request, body any) OperationKey {
	id := r.Header.Get("X-LayerHub-Operation")
	if id == "" {
		return OperationKey{}
	}
	raw, _ := json.Marshal(body)
	sum := sha256.Sum256(append([]byte(r.Method+" "+r.URL.Path+"\n"), raw...))
	return OperationKey{id, hex.EncodeToString(sum[:]), r.URL.Path}
}
func (s *Service) operationTransaction(u User, ops []OperationKey, fn func(*sql.Tx) error) error {
	return s.transaction(func(tx *sql.Tx) error {
		if len(ops) == 0 || ops[0].ID == "" {
			return fn(tx)
		}
		key := ops[0]
		if len(ops) != 1 || !regexp.MustCompile(`^[a-f0-9]{32}$`).MatchString(key.ID) {
			return invalid("操作标识无效，请刷新后重试")
		}
		if _, e := currentUser(tx, u.ID); e != nil {
			return e
		}
		var digest string
		e := tx.QueryRow("SELECT digest FROM operation_receipts WHERE user_id=? AND operation_id=?", u.ID, key.ID).Scan(&digest)
		if e == nil {
			if digest != key.Digest {
				return conflict("操作标识已用于其他内容，请重新读取结果")
			}
			return conflict("这次操作已经保存，请读取结果，不要重复提交")
		}
		if e != sql.ErrNoRows {
			return e
		}
		if e = fn(tx); e != nil {
			return e
		}
		_, e = tx.Exec("INSERT INTO operation_receipts(user_id,operation_id,digest,kind,completed) VALUES(?,?,?,?,?)", u.ID, key.ID, key.Digest, key.Kind, time.Now().UnixMilli())
		return e
	})
}
func (s *Service) OperationResult(u User, id string) (map[string]any, error) {
	if !regexp.MustCompile(`^[a-f0-9]{32}$`).MatchString(id) {
		return nil, invalid("操作标识无效，请刷新后重试")
	}
	var kind string
	var completed int64
	e := s.DB.QueryRow("SELECT kind,completed FROM operation_receipts WHERE user_id=? AND operation_id=?", u.ID, id).Scan(&kind, &completed)
	if e == sql.ErrNoRows {
		return map[string]any{"ok": true, "state": "unknown"}, nil
	}
	if e != nil {
		return nil, e
	}
	return map[string]any{"ok": true, "state": "committed", "kind": kind, "completedAt": completed}, nil
}
