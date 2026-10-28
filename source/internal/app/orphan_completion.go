package app

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"time"
)

const orphanCompletionSchema = `CREATE TABLE IF NOT EXISTS orphan_completions(printer_id INTEGER NOT NULL REFERENCES printers(id),job_id INTEGER NOT NULL,command_id INTEGER NOT NULL REFERENCES printer_commands(id),reviewed_by INTEGER NOT NULL REFERENCES users(id),uses_json TEXT NOT NULL,note TEXT NOT NULL,confirmed_at INTEGER NOT NULL,PRIMARY KEY(printer_id,job_id));
CREATE TABLE IF NOT EXISTS orphan_completion_holds(printer_id INTEGER NOT NULL REFERENCES printers(id),job_id INTEGER NOT NULL,command_id INTEGER NOT NULL REFERENCES printer_commands(id),observed_at INTEGER NOT NULL,PRIMARY KEY(printer_id,job_id));`

type OrphanCompletion struct {
	JobID     int64 `json:"jobId"`
	CommandID int64 `json:"commandId"`
}

type OrphanCompletionInput struct {
	PrinterID   int64              `json:"printerId"`
	JobID       int64              `json:"jobId"`
	ConfirmTask string             `json:"confirmTask"`
	Collected   bool               `json:"collected"`
	Uses        map[string]float64 `json:"uses"`
	Note        string             `json:"note"`
}

// Reviewing an uncertain start receipt is not collection or material settlement.
// A matching completed orphan remains unavailable until explicitly reconciled.
func pendingOrphanCompletion(q queryer, pid int64, t Telemetry) (*OrphanCompletion, error) {
	var v OrphanCompletion
	e := q.QueryRow(`SELECT h.job_id,h.command_id FROM orphan_completion_holds h LEFT JOIN orphan_completions o ON o.printer_id=h.printer_id AND o.job_id=h.job_id WHERE h.printer_id=? AND o.job_id IS NULL ORDER BY h.observed_at LIMIT 1`, pid).Scan(&v.JobID, &v.CommandID)
	if e == nil {
		return &v, nil
	}
	if e != sql.ErrNoRows {
		return nil, e
	}
	return freshOrphanCompletion(q, pid, t)
}

func freshOrphanCompletion(q queryer, pid int64, t Telemetry) (*OrphanCompletion, error) {
	if !t.Connected || !timeFresh(t.Updated) || !timeFresh(t.StateUpdated) || !timeFresh(t.TaskUpdated) || t.RawState != "FINISH" || t.Percent != 100 {
		return nil, nil
	}
	rows, e := q.Query(`SELECT c.job_id,c.id FROM printer_commands c LEFT JOIN jobs j ON j.id=c.job_id LEFT JOIN orphan_completions o ON o.printer_id=c.printer_id AND o.job_id=c.job_id WHERE c.printer_id=? AND c.action='start' AND c.job_id>0 AND c.state IN ('succeeded','reviewed','uncertain') AND j.id IS NULL AND o.job_id IS NULL ORDER BY c.id DESC`, pid)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	for rows.Next() {
		var v OrphanCompletion
		if e = rows.Scan(&v.JobID, &v.CommandID); e != nil {
			return nil, e
		}
		if jobMatches(Job{ID: v.JobID}, t) {
			return &v, nil
		}
	}
	return nil, rows.Err()
}

// No synthetic job, owner, estimate or completion email is created. Only the
// administrator's explicit stock correction is recorded, atomically and once.
func (s *Service) SettleOrphanCompletion(actor User, in OrphanCompletionInput) error {
	return s.transaction(func(tx *sql.Tx) error {
		u, e := currentUser(tx, actor.ID)
		if e != nil {
			return e
		}
		if !u.Admin {
			return forbidden("只有管理员可以补登记缺失任务")
		}
		if !in.Collected || in.ConfirmTask != fmt.Sprintf("job-%d", in.JobID) {
			return invalid("请核对任务标识并确认已取件清空打印板")
		}
		if len(in.Uses) < 1 || len(in.Uses) > 32 || trim(in.Note, 500) == "" {
			return invalid("请填写实际使用的料卷、用量和核对说明")
		}
		var n int
		if e = tx.QueryRow("SELECT count(*) FROM orphan_completions WHERE printer_id=? AND job_id=?", in.PrinterID, in.JobID).Scan(&n); e != nil {
			return e
		}
		if n > 0 {
			return conflict("此缺失任务已经补登记，请刷新核对记录")
		}
		t := s.Device.Status(in.PrinterID)
		pending, e := pendingOrphanCompletion(tx, in.PrinterID, t)
		if e != nil {
			return e
		}
		if pending == nil || pending.JobID != in.JobID || !t.Connected || !timeFresh(t.Updated) || !timeFresh(t.StateUpdated) || !timeFresh(t.TaskUpdated) || t.RawState != "FINISH" || t.Percent != 100 || !jobMatches(Job{ID: in.JobID}, t) || t.PrintError != 0 || t.Error != "" || len(t.Equipment.HMS) > 0 || t.Equipment.Switching {
			return conflict("缺失任务完成状态不明确或设备有告警，请刷新核对")
		}
		if e = tx.QueryRow("SELECT count(*) FROM printer_commands WHERE printer_id=? AND state IN ('pending','uncertain')", in.PrinterID).Scan(&n); e != nil {
			return e
		}
		if n > 0 {
			return conflict("请先核对未确认的设备指令")
		}
		jobs, e := readJobs(tx)
		if e != nil {
			return e
		}
		for _, j := range jobs {
			if j.Printer == in.PrinterID && j.State != "queued" && activeStates[j.State] {
				return conflict("设备还有执行中或待确认任务")
			}
		}
		spools, e := readSpools(tx)
		if e != nil {
			return e
		}
		found := 0
		for _, sp := range spools {
			g, ok := in.Uses[sp.ID]
			if !ok {
				continue
			}
			found++
			if !spoolEditable(sp, u) {
				return forbidden("不能修改其他成员的私有耗材")
			}
			if !finite(g) || g < 0 || g > 100000 || math.Abs(g*1000-math.Round(g*1000)) > .00001 {
				return invalid("用量须为非负数，最多保留三位小数")
			}
			booked := int(math.Ceil(float64(math.Round(g*1000)) / 1000))
			if booked > sp.Remaining-reserve(jobs, sp.ID, 0) {
				return conflict("实际用量超过可分配余量")
			}
			if _, e = tx.Exec("UPDATE spools SET remaining=remaining-?,state=CASE WHEN remaining-?=0 THEN 'empty' ELSE state END WHERE id=?", booked, booked, sp.ID); e != nil {
				return e
			}
		}
		if found != len(in.Uses) {
			return missing("关联耗材不存在")
		}
		body, e := json.Marshal(in.Uses)
		if e != nil {
			return e
		}
		_, e = tx.Exec("INSERT INTO orphan_completions(printer_id,job_id,command_id,reviewed_by,uses_json,note,confirmed_at) VALUES(?,?,?,?,?,?,?)", in.PrinterID, in.JobID, pending.CommandID, u.ID, string(body), trim(in.Note, 500), time.Now().Unix())
		return e
	})
}
