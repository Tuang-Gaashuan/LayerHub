package app

import (
	"database/sql"
	"fmt"
	"time"
)

func nextJobID(tx *sql.Tx) (int64, error) {
	var id int64
	err := tx.QueryRow("SELECT COALESCE(MAX(v),0)+1 FROM (SELECT id AS v FROM jobs UNION ALL SELECT job_id AS v FROM printer_commands UNION ALL SELECT CAST(j.value AS INTEGER) AS v FROM job_import_batches b,json_each(b.job_ids) j)").Scan(&id)
	if id < 1 && err == nil {
		return 0, fmt.Errorf("job ID exhausted")
	}
	return id, err
}

type OrphanReview struct {
	PrinterID   int64  `json:"printerId"`
	CommandID   int64  `json:"commandId"`
	ConfirmTask string `json:"confirmTask"`
	Confirmed   bool   `json:"confirmed"`
}

// A missing job cannot be reconstructed from telemetry. Review only the audit
// lock after an administrator checks the identified machine; never invent stock,
// ownership, files, settlement or completed-job notifications.
func (s *Service) ReviewOrphan(actor User, in OrphanReview) error {
	return s.transaction(func(tx *sql.Tx) error {
		u, e := currentUser(tx, actor.ID)
		if e != nil {
			return e
		}
		if !u.Admin {
			return forbidden("只有管理员可以核对孤立指令")
		}
		if !in.Confirmed {
			return invalid("请先现场核对打印机")
		}
		var pid, jid int64
		var action, state string
		if e = tx.QueryRow("SELECT printer_id,job_id,action,state FROM printer_commands WHERE id=?", in.CommandID).Scan(&pid, &jid, &action, &state); e != nil {
			return missing("指令记录不存在")
		}
		if pid != in.PrinterID || jid < 1 || action != "start" || in.ConfirmTask != fmt.Sprintf("job-%d", jid) {
			return conflict("指令或设备任务标识不匹配")
		}
		var count int
		if e = tx.QueryRow("SELECT count(*) FROM jobs WHERE id=?", jid).Scan(&count); e != nil {
			return e
		}
		if count != 0 {
			return conflict("原任务仍存在，请使用任务核对入口")
		}
		if state == "reviewed" {
			return nil
		}
		if state != "uncertain" {
			return conflict("此指令当前不能核对")
		}
		if e = tx.QueryRow("SELECT count(*) FROM printer_commands WHERE printer_id=? AND state='pending'", pid).Scan(&count); e != nil {
			return e
		}
		if count > 0 {
			return conflict("请等待正在提交的设备指令")
		}
		t := s.Device.Status(pid)
		if !t.Connected || !timeFresh(t.StateUpdated) || !timeFresh(t.TaskUpdated) || !timeFresh(t.Updated) || !jobMatches(Job{ID: jid}, t) {
			return conflict("设备任务身份未知或过期，不能解除锁定")
		}
		if t.RawState != "RUNNING" && t.RawState != "PAUSE" && t.RawState != "FINISH" {
			return conflict("设备状态尚不能明确核对")
		}
		if t.PrintError != 0 || len(t.Equipment.HMS) > 0 {
			return conflict("请先处理设备告警")
		}
		_, e = tx.Exec("UPDATE printer_commands SET state='reviewed',message=message||?,finished_at=? WHERE id=? AND state='uncertain'", fmt.Sprintf(" · 管理员 %d 现场核对孤立任务 %s，设备状态 %s；仅解除指令锁，不重建任务或结算", u.ID, in.ConfirmTask, t.RawState), time.Now().UnixMilli(), in.CommandID)
		return e
	})
}
