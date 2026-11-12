package app

import (
	"database/sql"
	"encoding/json"
	"reflect"
	"time"
)

const spoolHistorySchema = `CREATE TABLE IF NOT EXISTS spool_events(
 id INTEGER PRIMARY KEY AUTOINCREMENT,spool_id TEXT NOT NULL,actor_id INTEGER NOT NULL,
 action TEXT NOT NULL,job_id INTEGER NOT NULL DEFAULT 0,before_json TEXT NOT NULL,after_json TEXT NOT NULL,
 created INTEGER NOT NULL);
 CREATE INDEX IF NOT EXISTS spool_events_page ON spool_events(spool_id,id DESC);`

func (s *Service) initSpoolHistory() error {
	if _, e := s.DB.Exec(spoolHistorySchema); e != nil {
		return e
	}
	return s.transaction(func(tx *sql.Tx) error {
		spools, e := readSpools(tx)
		if e != nil {
			return e
		}
		for _, sp := range spools {
			var count int
			if e = tx.QueryRow("SELECT count(*) FROM spool_events WHERE spool_id=?", sp.ID).Scan(&count); e != nil {
				return e
			}
			if count == 0 {
				if e = spoolEvent(tx, 0, "baseline", nil, &sp, 0); e != nil {
					return e
				}
			}
		}
		return nil
	})
}

// This runs inside the inventory transaction. A logging failure rolls back stock.
func spoolEvent(tx *sql.Tx, actor int64, action string, before, after *Spool, job int64) error {
	if before != nil && after != nil && action != "consume" && action != "orphan_consume" {
		b, a := *before, *after
		b.Revision = a.Revision
		if reflect.DeepEqual(b, a) {
			return nil
		}
	}
	id := ""
	if before != nil {
		id = before.ID
	}
	if after != nil {
		id = after.ID
	}
	b, e := json.Marshal(before)
	if e != nil {
		return e
	}
	a, e := json.Marshal(after)
	if e != nil {
		return e
	}
	_, e = tx.Exec("INSERT INTO spool_events(spool_id,actor_id,action,job_id,before_json,after_json,created) VALUES(?,?,?,?,?,?,?)", id, actor, action, job, string(b), string(a), time.Now().UnixMilli())
	return e
}

func recordedSpool(tx *sql.Tx, actor int64, action string, before *Spool, id string, job int64) error {
	spools, e := readSpools(tx)
	if e != nil {
		return e
	}
	for _, sp := range spools {
		if sp.ID == id {
			return spoolEvent(tx, actor, action, before, &sp, job)
		}
	}
	return missing("耗材不存在")
}

type SpoolEvent struct {
	ID      int64  `json:"id"`
	ActorID int64  `json:"actorId"`
	Actor   string `json:"actor"`
	Action  string `json:"action"`
	JobID   int64  `json:"jobId"`
	Before  *Spool `json:"before"`
	After   *Spool `json:"after"`
	Created int64  `json:"created"`
}

func (s *Service) SpoolHistory(u User, id string, before int64) (map[string]any, error) {
	out := []SpoolEvent{}
	e := s.transaction(func(tx *sql.Tx) error {
		actor, e := currentUser(tx, u.ID)
		if e != nil {
			return e
		}
		spools, e := readSpools(tx)
		if e != nil {
			return e
		}
		visible := false
		for _, sp := range spools {
			if sp.ID == id {
				visible = spoolVisible(sp, actor)
			}
		}
		if !visible {
			return forbidden("无权查看这卷耗材的流水")
		}
		rows, e := tx.Query(`SELECT e.id,e.actor_id,COALESCE(u.name,''),e.action,e.job_id,e.before_json,e.after_json,e.created FROM spool_events e LEFT JOIN users u ON u.id=e.actor_id WHERE e.spool_id=? AND (?=0 OR e.id<?) ORDER BY e.id DESC LIMIT 51`, id, before, before)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var v SpoolEvent
			var b, a string
			if e = rows.Scan(&v.ID, &v.ActorID, &v.Actor, &v.Action, &v.JobID, &b, &a, &v.Created); e != nil {
				return e
			}
			if e = json.Unmarshal([]byte(b), &v.Before); e != nil {
				return e
			}
			if e = json.Unmarshal([]byte(a), &v.After); e != nil {
				return e
			}
			out = append(out, v)
		}
		return rows.Err()
	})
	if e != nil {
		return nil, e
	}
	next := int64(0)
	if len(out) > 50 {
		out = out[:50]
		next = out[49].ID
	}
	return map[string]any{"ok": true, "events": out, "next": next}, nil
}
