package app

import (
	"database/sql"
	"encoding/json"
	"time"
)

const maintenanceSchema = `
CREATE TABLE IF NOT EXISTS maintenance(id INTEGER PRIMARY KEY CHECK(id=1),until_at INTEGER NOT NULL DEFAULT 0,revision INTEGER NOT NULL DEFAULT 0,ack_revision INTEGER NOT NULL DEFAULT 0,ack_at INTEGER NOT NULL DEFAULT 0);
INSERT OR IGNORE INTO maintenance(id) VALUES(1);
CREATE TABLE IF NOT EXISTS maintenance_audit(id INTEGER PRIMARY KEY,actor_id INTEGER NOT NULL,until_at INTEGER NOT NULL,created INTEGER NOT NULL);
`

type Maintenance struct {
	Until       int64 `json:"until"`
	Revision    int64 `json:"revision"`
	AckRevision int64 `json:"ackRevision"`
	AckAt       int64 `json:"ackAt"`
}

func readMaintenance(q queryer) (m Maintenance, err error) {
	err = q.QueryRow("SELECT until_at,revision,ack_revision,ack_at FROM maintenance WHERE id=1").Scan(&m.Until, &m.Revision, &m.AckRevision, &m.AckAt)
	return
}
func (s *Service) maintenanceStatus(u User) (any, error) {
	if !u.Admin {
		return nil, forbidden("仅管理员可以管理检修模式")
	}
	m, e := readMaintenance(s.DB)
	return map[string]any{"ok": true, "maintenance": m, "serverNow": time.Now().Unix()}, e
}
func (s *Service) setMaintenance(u User, minutes int, revision int64) error {
	if !u.Admin {
		return forbidden("仅管理员可以管理检修模式")
	}
	if minutes != 0 && minutes != 30 && minutes != 60 && minutes != 120 && minutes != 240 {
		return conflict("请选择有效的检修时长")
	}
	return s.transaction(func(tx *sql.Tx) error {
		now := time.Now().Unix()
		until := int64(0)
		if minutes > 0 {
			until = now + int64(minutes)*60
		}
		r, e := tx.Exec("UPDATE maintenance SET until_at=?,revision=revision+1,ack_at=0 WHERE id=1 AND revision=?", until, revision)
		if e != nil {
			return e
		}
		n, _ := r.RowsAffected()
		if n != 1 {
			return conflict("检修设置已变化，请刷新后重试")
		}
		if _, e = tx.Exec("INSERT INTO maintenance_audit(actor_id,until_at,created) VALUES(?,?,?)", u.ID, until, now); e != nil {
			return e
		}
		if minutes == 0 {
			return nil
		}
		// Cancel queued/leased offline alerts, including alerts created by older versions.
		rows, e := tx.Query("SELECT id,body FROM mail_outbox WHERE state IN ('queued','leased') AND event_key LIKE 'printer:%'")
		if e != nil {
			return e
		}
		ids := []string{}
		for rows.Next() {
			var id string
			var body []byte
			if e = rows.Scan(&id, &body); e != nil {
				rows.Close()
				return e
			}
			raw, err := s.unseal(body)
			if err != nil {
				rows.Close()
				return err
			}
			var content struct{ Subject string }
			if e = json.Unmarshal([]byte(raw), &content); e != nil {
				rows.Close()
				return e
			}
			if content.Subject == "LayerHub · "+"打印机持续离线" || content.Subject == "LayerHub · "+languageText("en", "打印机持续离线") {
				ids = append(ids, id)
			}
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		for _, id := range ids {
			if _, e = tx.Exec("UPDATE mail_outbox SET state='cancelled',body=X'',lease='',lease_until=0 WHERE id=?", id); e != nil {
				return e
			}
		}
		// Restart the debounce at the end, rather than replaying maintenance outages.
		_, e = tx.Exec("INSERT INTO mail_observations(key,value,candidate,since,seq) SELECT 'printer:'||id,'ok','ok',?,0 FROM printers WHERE true ON CONFLICT(key) DO UPDATE SET value='ok',candidate='ok',since=excluded.since,seq=mail_observations.seq+1", now)
		return e
	})
}
