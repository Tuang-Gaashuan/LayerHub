package app

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// An owner remains admin=true for device/task compatibility. Only this single
// extra flag grants ownership operations; initial usernames never bypass it.
func initOwnership(db *sql.DB, initialUsername string) error {
	tx, e := db.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var version int
	if e = tx.QueryRow("PRAGMA user_version").Scan(&version); e != nil {
		return e
	}
	if version > 5 {
		return fmt.Errorf("database version is newer than this program")
	}
	rows, e := tx.Query("PRAGMA table_info(users)")
	if e != nil {
		return e
	}
	hasOwner := false
	for rows.Next() {
		var id, required, pk int
		var name, kind string
		var def any
		if e = rows.Scan(&id, &name, &kind, &required, &def, &pk); e != nil {
			rows.Close()
			return e
		}
		hasOwner = hasOwner || name == "owner"
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	if !hasOwner {
		if _, e = tx.Exec("ALTER TABLE users ADD COLUMN owner INTEGER NOT NULL DEFAULT 0 CHECK(owner IN (0,1))"); e != nil {
			return e
		}
	}
	if _, e = tx.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS one_workspace_owner ON users(owner) WHERE owner=1;
 CREATE TABLE IF NOT EXISTS workspace_security(id INTEGER PRIMARY KEY CHECK(id=1),owner_initialized INTEGER NOT NULL CHECK(owner_initialized=1));
 CREATE TABLE IF NOT EXISTS role_audit(id INTEGER PRIMARY KEY,actor_id INTEGER NOT NULL REFERENCES users(id),user_id INTEGER NOT NULL REFERENCES users(id),old_role TEXT NOT NULL,new_role TEXT NOT NULL,reason TEXT NOT NULL,created INTEGER NOT NULL);`); e != nil {
		return e
	}
	var initialized, count, valid int
	if e = tx.QueryRow("SELECT COUNT(*) FROM workspace_security WHERE id=1").Scan(&initialized); e != nil {
		return e
	}
	if e = tx.QueryRow("SELECT COUNT(*),COALESCE(SUM(admin=1 AND disabled=0),0) FROM users WHERE owner=1").Scan(&count, &valid); e != nil {
		return e
	}
	if initialized == 0 {
		if count == 0 {
			var id int64
			if e = tx.QueryRow("SELECT id FROM users WHERE username=? AND admin=1 AND disabled=0", initialUsername).Scan(&id); e != nil {
				return fmt.Errorf("cannot initialize ownership: initial administrator is missing or disabled")
			}
			if _, e = tx.Exec("UPDATE users SET owner=1 WHERE id=?", id); e != nil {
				return e
			}
			if _, e = tx.Exec("INSERT INTO role_audit(actor_id,user_id,old_role,new_role,reason,created) VALUES(?,?,'admin','owner','initial_migration',?)", id, id, time.Now().UnixMilli()); e != nil {
				return e
			}
			count, valid = 1, 1
		}
		if _, e = tx.Exec("INSERT INTO workspace_security(id,owner_initialized) VALUES(1,1)"); e != nil {
			return e
		}
	}
	if count != 1 || valid != 1 {
		return fmt.Errorf("workspace requires exactly one enabled owner; automatic reassignment refused")
	}
	if _, e = tx.Exec("PRAGMA user_version=5"); e != nil {
		return e
	}
	return tx.Commit()
}
func currentUser(q queryer, id int64) (User, error) {
	var u User
	e := q.QueryRow("SELECT id,username,name,admin,owner FROM users WHERE id=? AND disabled=0", id).Scan(&u.ID, &u.Username, &u.Name, &u.Admin, &u.Owner)
	if e != nil {
		return u, forbidden("账户已停用或权限已变化，请重新登录")
	}
	return u, nil
}
func requireOwner(q queryer, id int64) error {
	u, e := currentUser(q, id)
	if e != nil {
		return e
	}
	if !u.Owner || !u.Admin {
		return forbidden("此操作仅限空间所有者")
	}
	return nil
}
func roleKey(u User) string {
	if u.Owner {
		return "owner"
	}
	if u.Admin {
		return "admin"
	}
	return "member"
}

func backupHasOwner(db *sql.DB) (bool, error) {
	rows, e := db.Query("PRAGMA table_info(users)")
	if e != nil {
		return false, e
	}
	found := false
	for rows.Next() {
		var id, required, pk int
		var name, kind string
		var def any
		if e = rows.Scan(&id, &name, &kind, &required, &def, &pk); e != nil {
			rows.Close()
			return false, e
		}
		found = found || name == "owner"
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return false, e
	}
	return found, nil
}
func validateBackupOwnership(db *sql.DB, version int) error {
	has, e := backupHasOwner(db)
	if e != nil {
		return invalid("备份角色结构无效")
	}
	if !has {
		if version >= 4 {
			return invalid("备份缺少空间所有者")
		}
		return nil
	}
	var all, valid, marker, bad int
	if db.QueryRow("SELECT COUNT(*),COALESCE(SUM(admin=1 AND disabled=0),0) FROM users WHERE owner=1").Scan(&all, &valid) != nil || all != 1 || valid != 1 {
		return invalid("备份必须包含唯一启用的空间所有者")
	}
	if db.QueryRow("SELECT COUNT(*) FROM workspace_security WHERE id=1 AND owner_initialized=1").Scan(&marker) != nil || marker != 1 {
		return invalid("备份所有者初始化记录缺失")
	}
	if db.QueryRow("SELECT COUNT(*) FROM users WHERE owner NOT IN (0,1) OR admin NOT IN (0,1) OR disabled NOT IN (0,1)").Scan(&bad) != nil || bad != 0 {
		return invalid("备份账号权限无效")
	}
	return nil
}

type OwnershipTransfer struct {
	UserID          int64  `json:"userId"`
	CurrentPassword string `json:"currentPassword"`
	ConfirmUsername string `json:"confirmUsername"`
}

func (s *Service) TransferOwnership(actor User, in OwnershipTransfer) error {
	s.restoreMu.Lock()
	defer s.restoreMu.Unlock()
	if e := requireOwner(s.DB, actor.ID); e != nil {
		return e
	}
	if s.restoreTicket != "" {
		return conflict("请先取消备份恢复预览，再转让所有权")
	}
	return s.transaction(func(tx *sql.Tx) error {
		if e := requireOwner(tx, actor.ID); e != nil {
			return e
		}
		if in.UserID == actor.ID {
			return invalid("请明确选择另一个启用账户")
		}
		if e := checkPassword(tx, actor.ID, in.CurrentPassword); e != nil {
			return e
		}
		target, e := currentUser(tx, in.UserID)
		if e != nil {
			return invalid("接任账户不存在或已停用")
		}
		if strings.TrimSpace(in.ConfirmUsername) != target.Username {
			return invalid("请输入接任者的完整登录用户名确认")
		}
		if _, e = tx.Exec("UPDATE users SET owner=0 WHERE id=?", actor.ID); e != nil {
			return e
		}
		if _, e = tx.Exec("UPDATE users SET owner=1,admin=1 WHERE id=?", target.ID); e != nil {
			return e
		}
		now := time.Now().UnixMilli()
		if _, e = tx.Exec("INSERT INTO role_audit(actor_id,user_id,old_role,new_role,reason,created) VALUES(?,?,'owner','admin','ownership_transfer',?),(?,?,?,?, 'ownership_transfer',?)", actor.ID, actor.ID, now, actor.ID, target.ID, roleKey(target), "owner", now); e != nil {
			return e
		}
		_, e = tx.Exec("DELETE FROM sessions WHERE user_id IN (?,?)", actor.ID, target.ID)
		return e
	})
}
