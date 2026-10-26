package app

import (
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"net"
	"net/http"
	"strings"
	"time"
)

// The NAS listens on loopback. Nginx overwrites X-Forwarded-Proto before the
// private SSH tunnel forwards it. Never trust this header from a non-loopback peer.
func (s *Service) secureCookie(r *http.Request) bool {
	if s.Config.CookieSecure || r.TLS != nil {
		return true
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	ip := net.ParseIP(host)
	return err == nil && ip != nil && ip.IsLoopback() && r.Header.Get("X-Forwarded-Proto") == "https"
}

func hashPassword(password string) (string, error) {
	salt := randomID(16)
	key, err := pbkdf2.Key(sha256.New, password, []byte(salt), 200000, 32)
	if err != nil {
		return "", err
	}
	return "pbkdf2$" + salt + "$" + hex.EncodeToString(key), nil
}
func verifyPassword(password, hash string) bool {
	parts := strings.Split(hash, "$")
	if len(parts) != 3 || parts[0] != "pbkdf2" {
		return false
	}
	key, err := pbkdf2.Key(sha256.New, password, []byte(parts[1]), 200000, 32)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(hex.EncodeToString(key)), []byte(parts[2])) == 1
}
func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
func (s *Service) csrf(token string) string {
	h := hmac.New(sha256.New, s.key)
	h.Write([]byte("csrf:" + token))
	return hex.EncodeToString(h.Sum(nil))
}
func (s *Service) session(r *http.Request) (User, string, error) {
	cookie, err := r.Cookie("layerhub_sid")
	if err != nil {
		return User{}, "", &apiError{401, "请先登录"}
	}
	var u User
	err = s.DB.QueryRow(`SELECT u.id,u.username,u.name,u.admin,u.owner FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.hash=? AND s.expires>? AND u.disabled=0`, tokenHash(cookie.Value), time.Now().Unix()).Scan(&u.ID, &u.Username, &u.Name, &u.Admin, &u.Owner)
	if err != nil {
		return User{}, "", &apiError{401, "登录已过期"}
	}
	return u, cookie.Value, nil
}
func (s *Service) login(w http.ResponseWriter, r *http.Request) {
	var in struct{ Username, Password string }
	if err := decode(r, &in); err != nil {
		fail(w, err)
		return
	}
	var u User
	var hash string
	err := s.DB.QueryRow("SELECT id,username,name,admin,owner,password_hash FROM users WHERE username=? AND disabled=0", strings.TrimSpace(in.Username)).Scan(&u.ID, &u.Username, &u.Name, &u.Admin, &u.Owner, &hash)
	if err != nil {
		hash = "pbkdf2$invalid-user$0000000000000000000000000000000000000000000000000000000000000000"
	}
	valid := verifyPassword(in.Password, hash)
	if err != nil || !valid {
		fail(w, &apiError{401, "用户名或密码错误"})
		return
	}
	token := randomID(32)
	err = s.transaction(func(tx *sql.Tx) error {
		// Password verification runs outside the write lock. Recheck the exact
		// credential and enabled state so a concurrent reset cannot mint a session.
		var freshHash string
		if e := tx.QueryRow("SELECT password_hash FROM users WHERE id=? AND disabled=0", u.ID).Scan(&freshHash); e != nil || freshHash != hash {
			return &apiError{401, "账户状态已变化，请重新登录"}
		}
		var e error
		if u, e = currentUser(tx, u.ID); e != nil {
			return e
		}
		now := time.Now().Unix()
		if _, e = tx.Exec("DELETE FROM sessions WHERE expires<=?", now); e != nil {
			return e
		}
		if _, e = tx.Exec("INSERT INTO sessions(hash,user_id,expires) VALUES(?,?,?)", tokenHash(token), u.ID, now+86400); e != nil {
			return e
		}
		_, e = tx.Exec("INSERT INTO session_details(hash,created,client) VALUES(?,?,?)", tokenHash(token), now, sessionClient(r.UserAgent()))
		return e
	})
	if err != nil {
		fail(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "layerhub_sid", Value: token, Path: "/", HttpOnly: true, Secure: s.secureCookie(r), SameSite: http.SameSiteStrictMode, MaxAge: 86400})
	reply(w, 200, map[string]any{"ok": true, "user": u, "csrf": s.csrf(token)})
}
