package app

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"strings"
	"time"
)

// This optional extension leaves existing sessions valid. Legacy sessions have
// no client details; never infer their login time from an approximate expiry.
func initSessionDetails(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS session_details(
 hash TEXT PRIMARY KEY REFERENCES sessions(hash) ON DELETE CASCADE,
 created INTEGER NOT NULL,client TEXT NOT NULL);
 CREATE INDEX IF NOT EXISTS sessions_user_expiry ON sessions(user_id,expires);`)
	return err
}

// Store only a coarse label, not raw headers, IP addresses or forwarded headers.
// A user-agent is user-controlled and is not proof of a physical device.
func sessionClient(agent string) string {
	if len(agent) > 1024 {
		agent = agent[:1024]
	}
	browser, system := "浏览器", "未知系统"
	for _, v := range [][2]string{{"Edg/", "Edge"}, {"OPR/", "Opera"}, {"Firefox/", "Firefox"}, {"FxiOS/", "Firefox"}, {"Chrome/", "Chrome"}, {"CriOS/", "Chrome"}, {"Safari/", "Safari"}} {
		if strings.Contains(agent, v[0]) {
			browser = v[1]
			break
		}
	}
	for _, v := range [][2]string{{"Android", "Android"}, {"iPhone", "iOS"}, {"iPad", "iPadOS"}, {"Windows", "Windows"}, {"Macintosh", "macOS"}, {"Linux", "Linux"}} {
		if strings.Contains(agent, v[0]) {
			system = v[1]
			break
		}
	}
	return browser + " · " + system
}

// Revocation IDs are domain-separated keyed aliases. Neither the cookie nor
// its database hash is exposed to another browser, even for the same account.
func (s *Service) sessionID(hash string) string {
	h := hmac.New(sha256.New, s.key)
	h.Write([]byte("session-view:" + hash))
	return hex.EncodeToString(h.Sum(nil))
}

type LoginSession struct {
	ID      string `json:"id"`
	Client  string `json:"client"`
	Created int64  `json:"created"`
	Expires int64  `json:"expires"`
	Current bool   `json:"current"`
}

type SessionList struct {
	OK       bool           `json:"ok"`
	Sessions []LoginSession `json:"sessions"`
	Total    int            `json:"total"`
	Other    int            `json:"other"`
}

func (s *Service) accountSessions(u User, token string) (SessionList, error) {
	out := SessionList{OK: true, Sessions: []LoginSession{}}
	now, ownHash := time.Now().Unix(), tokenHash(token)
	// Consistent list/count; never honor a user ID supplied by the caller.
	tx, err := s.DB.BeginTx(s.ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if err = liveAccountSession(tx, u.ID, ownHash, now); err != nil {
		return out, err
	}
	if err = tx.QueryRow("SELECT COUNT(*) FROM sessions WHERE user_id=? AND expires>?", u.ID, now).Scan(&out.Total); err != nil {
		return out, err
	}
	out.Other = out.Total - 1
	rows, err := tx.Query(`SELECT s.hash,s.expires,COALESCE(d.created,0),COALESCE(d.client,'升级前登录')
 FROM sessions s LEFT JOIN session_details d ON d.hash=s.hash
 WHERE s.user_id=? AND s.expires>? ORDER BY (s.hash=?) DESC,s.expires DESC,s.hash LIMIT 100`, u.ID, now, ownHash)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var v LoginSession
		var hash string
		if err = rows.Scan(&hash, &v.Expires, &v.Created, &v.Client); err != nil {
			rows.Close()
			return out, err
		}
		v.ID, v.Current = s.sessionID(hash), hash == ownHash
		out.Sessions = append(out.Sessions, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	return out, tx.Commit()
}

func liveAccountSession(q queryer, id int64, hash string, now int64) error {
	var count int
	if err := q.QueryRow(`SELECT COUNT(*) FROM sessions s JOIN users u ON u.id=s.user_id
 WHERE s.hash=? AND s.user_id=? AND s.expires>? AND u.disabled=0`, hash, id, now).Scan(&count); err != nil {
		return err
	}
	if count != 1 {
		return &apiError{401, "登录已过期，请重新登录"}
	}
	return nil
}

type SessionRevoke struct {
	SessionID       string `json:"sessionId"`
	Others          bool   `json:"others"`
	CurrentPassword string `json:"currentPassword"`
}

func (s *Service) revokeAccountSessions(u User, token string, in SessionRevoke) (int64, error) {
	if in.Others == (in.SessionID != "") {
		return 0, invalid("请选择一条其他登录，或退出全部其他登录")
	}
	if in.SessionID != "" {
		b, err := hex.DecodeString(in.SessionID)
		if err != nil || len(b) != 32 || in.SessionID != strings.ToLower(in.SessionID) {
			return 0, invalid("登录记录编号无效")
		}
	}
	var removed int64
	err := s.transaction(func(tx *sql.Tx) error {
		now, ownHash := time.Now().Unix(), tokenHash(token)
		if err := liveAccountSession(tx, u.ID, ownHash, now); err != nil {
			return err
		}
		if err := checkPassword(tx, u.ID, in.CurrentPassword); err != nil {
			return err
		}
		var result sql.Result
		var err error
		if in.Others {
			result, err = tx.Exec("DELETE FROM sessions WHERE user_id=? AND hash<>? AND expires>?", u.ID, ownHash, now)
		} else {
			if in.SessionID == s.sessionID(ownHash) {
				return invalid("当前登录请使用个人账户中的退出登录")
			}
			rows, e := tx.Query("SELECT hash FROM sessions WHERE user_id=? AND hash<>? AND expires>?", u.ID, ownHash, now)
			if e != nil {
				return e
			}
			match := ""
			for rows.Next() {
				var hash string
				if e = rows.Scan(&hash); e != nil {
					rows.Close()
					return e
				}
				if in.SessionID == s.sessionID(hash) {
					match = hash
				}
			}
			e = rows.Err()
			rows.Close()
			if e != nil {
				return e
			}
			if match == "" {
				return missing("登录记录已失效或不属于你，请刷新列表")
			}
			result, err = tx.Exec("DELETE FROM sessions WHERE user_id=? AND hash=?", u.ID, match)
		}
		if err != nil {
			return err
		}
		removed, err = result.RowsAffected()
		return err
	})
	return removed, err
}

type AuditPerson struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
}
type RoleEvent struct {
	ID      int64       `json:"id"`
	Actor   AuditPerson `json:"actor"`
	Target  AuditPerson `json:"target"`
	OldRole string      `json:"oldRole"`
	NewRole string      `json:"newRole"`
	Reason  string      `json:"reason"`
	Created int64       `json:"created"`
}
type RoleEvents struct {
	OK         bool        `json:"ok"`
	Events     []RoleEvent `json:"events"`
	NextCursor int64       `json:"nextCursor"`
}

func (s *Service) roleEvents(u User, before int64) (RoleEvents, error) {
	out := RoleEvents{OK: true, Events: []RoleEvent{}}
	if before < 0 {
		return out, invalid("记录游标无效")
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if err = requireOwner(tx, u.ID); err != nil {
		return out, err
	}
	rows, err := tx.Query(`SELECT r.id,a.id,a.username,a.name,t.id,t.username,t.name,r.old_role,r.new_role,r.reason,r.created
 FROM role_audit r JOIN users a ON a.id=r.actor_id JOIN users t ON t.id=r.user_id
 WHERE (?=0 OR r.id<?) ORDER BY r.id DESC LIMIT 51`, before, before)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var v RoleEvent
		if err = rows.Scan(&v.ID, &v.Actor.ID, &v.Actor.Username, &v.Actor.Name, &v.Target.ID, &v.Target.Username, &v.Target.Name, &v.OldRole, &v.NewRole, &v.Reason, &v.Created); err != nil {
			rows.Close()
			return out, err
		}
		out.Events = append(out.Events, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(out.Events) > 50 {
		out.Events = out.Events[:50]
		out.NextCursor = out.Events[49].ID
	}
	return out, tx.Commit()
}
