package app

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

func checkPassword(q queryer, id int64, password string) error {
	var hash string
	if err := q.QueryRow("SELECT password_hash FROM users WHERE id=? AND disabled=0", id).Scan(&hash); err != nil || !verifyPassword(password, hash) {
		return forbidden("当前密码不正确")
	}
	return nil
}
func (s *Service) ChangePassword(u User, current, next string) error {
	if len(next) < 8 || len(next) > 256 {
		return invalid("新密码需要 8–256 位")
	}
	hash, err := hashPassword(next)
	if err != nil {
		return err
	}
	err = s.transaction(func(tx *sql.Tx) error {
		if err := checkPassword(tx, u.ID, current); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE users SET password_hash=? WHERE id=?", hash, u.ID); err != nil {
			return err
		}
		_, err := tx.Exec("DELETE FROM sessions WHERE user_id=?", u.ID)
		return err
	})
	if err == nil && u.Username == s.Config.AdminUser {
		_ = os.Remove(filepath.Join(s.Config.DataDir, "bootstrap-admin.txt"))
	}
	return err
}

type MemberUpdate struct {
	UserID          int64   `json:"userId"`
	Name            *string `json:"name"`
	Disabled        *bool   `json:"disabled"`
	NewPassword     string  `json:"newPassword"`
	CurrentPassword string  `json:"currentPassword"`
	Role            *string `json:"role"`
}

func (s *Service) UpdateMember(actor User, in MemberUpdate) error {
	if !actor.Admin {
		return forbidden("需要管理员权限")
	}
	accessChange := in.Disabled != nil || in.NewPassword != "" || in.Role != nil
	if in.Role != nil && *in.Role != "admin" && *in.Role != "member" {
		return invalid("角色仅可选择管理员或成员；所有者需单独转让")
	}
	if in.UserID == actor.ID && accessChange {
		return invalid("请在个人账户修改自己的密码；不能停用当前账户")
	}
	if !accessChange && in.Name == nil {
		return invalid("没有需要保存的修改")
	}
	var name string
	if in.Name != nil {
		name = strings.TrimSpace(*in.Name)
		if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 32 || strings.ContainsFunc(name, unicode.IsControl) {
			return invalid("显示名称需要 1–32 个字符，不能包含换行或控制字符")
		}
	}
	var hash string
	var err error
	if in.NewPassword != "" {
		if len(in.NewPassword) < 8 || len(in.NewPassword) > 256 {
			return invalid("新密码需要 8–256 位")
		}
		hash, err = hashPassword(in.NewPassword)
		if err != nil {
			return err
		}
	}
	var username string
	err = s.transaction(func(tx *sql.Tx) error {
		fresh, e := currentUser(tx, actor.ID)
		if e != nil {
			return e
		}
		actor = fresh
		if !actor.Admin {
			return forbidden("需要管理员权限")
		}
		var target User
		if e = tx.QueryRow("SELECT id,admin,owner FROM users WHERE id=?", in.UserID).Scan(&target.ID, &target.Admin, &target.Owner); e != nil {
			return missing("成员不存在")
		}
		if in.Role != nil && !actor.Owner {
			return forbidden("只有空间所有者可以调整角色")
		}
		if target.Owner && target.ID != actor.ID {
			return forbidden("不能修改空间所有者")
		}
		if target.Admin && !actor.Owner && target.ID != actor.ID {
			return forbidden("同级管理员由空间所有者管理")
		}
		if accessChange {
			if err := checkPassword(tx, actor.ID, in.CurrentPassword); err != nil {
				return err
			}
		}
		if err := tx.QueryRow("SELECT username FROM users WHERE id=?", in.UserID).Scan(&username); err != nil {
			return missing("成员不存在")
		}
		if in.Name != nil {
			var exists int
			if err := tx.QueryRow("SELECT COUNT(*) FROM users WHERE name=? AND id<>?", name, in.UserID).Scan(&exists); err != nil {
				return err
			}
			if exists > 0 {
				return conflict("显示名称已存在，请换一个名字")
			}
			if _, err := tx.Exec("UPDATE users SET name=? WHERE id=?", name, in.UserID); err != nil {
				return err
			}
		}
		if in.Disabled != nil {
			if _, err := tx.Exec("UPDATE users SET disabled=? WHERE id=?", *in.Disabled, in.UserID); err != nil {
				return err
			}
			if *in.Disabled {
				if _, err := tx.Exec("UPDATE mail_accounts SET enabled=0 WHERE user_id=?;", in.UserID); err != nil {
					return err
				}
				if _, err := tx.Exec("DELETE FROM mail_challenges WHERE user_id=?", in.UserID); err != nil {
					return err
				}
				if _, err := tx.Exec("UPDATE mail_outbox SET state='cancelled',body=X'' WHERE user_id=? AND state IN ('queued','leased')", in.UserID); err != nil {
					return err
				}
			}
		}
		if in.Role != nil {
			admin := *in.Role == "admin"
			if target.Owner {
				return invalid("不能直接降级所有者，请使用所有权转让")
			}
			if admin != target.Admin {
				if _, e = tx.Exec("UPDATE users SET admin=? WHERE id=?", admin, in.UserID); e != nil {
					return e
				}
				if _, e = tx.Exec("INSERT INTO role_audit(actor_id,user_id,old_role,new_role,reason,created) VALUES(?,?,?,?, 'role_change',?)", actor.ID, in.UserID, roleKey(target), *in.Role, time.Now().UnixMilli()); e != nil {
					return e
				}
				if !admin {
					if _, e = tx.Exec(`UPDATE mail_outbox SET state='cancelled',body=X'' WHERE user_id=? AND kind='notification' AND state IN ('queued','leased') AND event_key IN (SELECT event_key||':user:'||user_id FROM user_notifications n WHERE user_id=? AND (category='devices' OR (category='stock' AND EXISTS(SELECT 1 FROM spools WHERE id=n.entity_id AND owner_id IS NULL))))`, in.UserID, in.UserID); e != nil {
						return e
					}
				}
			}
		}
		if hash != "" {
			if _, err := tx.Exec("UPDATE users SET password_hash=? WHERE id=?", hash, in.UserID); err != nil {
				return err
			}
		}
		if accessChange {
			_, err := tx.Exec("DELETE FROM sessions WHERE user_id=?", in.UserID)
			return err
		}
		return nil
	})
	if err == nil && hash != "" && username == s.Config.AdminUser {
		_ = os.Remove(filepath.Join(s.Config.DataDir, "bootstrap-admin.txt"))
	}
	return err
}
