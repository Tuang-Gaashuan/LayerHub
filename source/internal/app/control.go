package app

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

type ControlInput struct {
	PrinterID int64  `json:"printerId"`
	JobID     int64  `json:"jobId"`
	Action    string `json:"action"`
}
type CommandRecord struct {
	ID      int64  `json:"id"`
	JobID   int64  `json:"jobId"`
	Action  string `json:"action"`
	State   string `json:"state"`
	Message string `json:"message"`
	User    string `json:"user"`
	Created int64  `json:"created"`
}

func (s *Service) CommandHistory(u User, pid int64) ([]CommandRecord, error) {
	rows, err := s.DB.Query("SELECT c.id,c.job_id,c.action,c.state,c.message,u.name,c.created_at FROM printer_commands c JOIN users u ON u.id=c.user_id WHERE c.printer_id=? ORDER BY c.id DESC LIMIT 12", pid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CommandRecord{}
	for rows.Next() {
		var r CommandRecord
		if err = rows.Scan(&r.ID, &r.JobID, &r.Action, &r.State, &r.Message, &r.User, &r.Created); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func (s *Service) ControlPrinter(u User, in ControlInput) error {
	if in.Action != "pause" && in.Action != "resume" && in.Action != "stop" && in.Action != "resolve" {
		return invalid("控制动作无效")
	}
	if s.Config.DeviceMode == "offline" {
		return forbidden("设备连接关闭")
	}
	driver, ok := s.Device.(PrinterController)
	if !ok {
		return conflict("驱动尚未提供控制能力")
	}
	var job Job
	var commandID int64
	err := s.transaction(func(tx *sql.Tx) error {
		jobs, e := readJobs(tx)
		if e != nil {
			return e
		}
		for _, j := range jobs {
			if j.ID == in.JobID && j.Printer == in.PrinterID {
				job = j
			}
		}
		if job.ID == 0 {
			return missing("请选择本系统在这台设备上的任务")
		}
		if job.UserID != u.ID && !u.Admin {
			return forbidden("只有任务本人或管理员可以控制")
		}
		var enabled bool
		if e = tx.QueryRow("SELECT enabled FROM printers WHERE id=?", in.PrinterID).Scan(&enabled); e != nil || !enabled {
			return conflict("设备已停用")
		}
		t := s.Device.Status(in.PrinterID)
		if !t.Connected || !timeFresh(t.Updated) {
			return conflict("设备离线或状态过期")
		}
		var pending, uncertain int
		if e = tx.QueryRow("SELECT COUNT(*) FROM printer_commands WHERE printer_id=? AND state='pending'", in.PrinterID).Scan(&pending); e != nil {
			return e
		}
		if pending > 0 {
			return conflict("设备指令正在等待确认")
		}
		if e = tx.QueryRow("SELECT COUNT(*) FROM printer_commands WHERE printer_id=? AND state='uncertain'", in.PrinterID).Scan(&uncertain); e != nil {
			return e
		}
		if in.Action == "resolve" {
			if job.State != "assigned" && job.State != "starting" && job.State != "printing" && job.State != "paused" && job.State != "stopped" {
				return conflict("该任务没有可核对的执行结果")
			}
			var unrelated int
			if e = tx.QueryRow("SELECT COUNT(*) FROM printer_commands WHERE printer_id=? AND (job_id!=? OR action LIKE 'set:%') AND state='uncertain'", in.PrinterID, job.ID).Scan(&unrelated); e != nil {
				return e
			}
			if unrelated > 0 {
				return forbidden("请使用对应入口核对原任务或设备设置")
			}
			if uncertain == 0 && job.Error == "" {
				return conflict("没有需要核对的结果")
			}
			state := ""
			switch t.State {
			case "printing":
				state = "printing"
			case "pause":
				state = "paused"
			case "idle", "failed":
				if job.State == "assigned" {
					state = "assigned"
				} else {
					state = "stopped"
				}
			}
			if state == "" {
				return conflict("设备状态尚不明确")
			}
			if (state == "printing" || state == "paused") && !jobMatches(job, t) {
				return conflict("设备当前打印任务无法与此任务匹配")
			}
			if _, e = tx.Exec("UPDATE jobs SET state=?,error='' WHERE id=?", state, job.ID); e != nil {
				return e
			}
			if _, e = tx.Exec("UPDATE printer_commands SET state='reviewed',message='用户核对设备后确认',finished_at=? WHERE printer_id=? AND state='uncertain'", time.Now().UnixMilli(), in.PrinterID); e != nil {
				return e
			}
		} else {
			if uncertain > 0 || job.Error != "" {
				return conflict("上次结果未确认，请先核对设备")
			}
			if job.State != "printing" && job.State != "paused" {
				return conflict("此任务尚未处于可控制状态")
			}
			if !jobMatches(job, t) {
				return conflict("设备当前任务与所选任务不匹配")
			}
			if in.Action == "pause" && t.State != "printing" || in.Action == "resume" && t.State != "pause" || in.Action == "stop" && t.State != "printing" && t.State != "pause" {
				return conflict("设备状态已变化，请刷新后再操作")
			}
		}
		state := "pending"
		if in.Action == "resolve" {
			state = "succeeded"
		}
		r, e := tx.Exec("INSERT INTO printer_commands(printer_id,job_id,user_id,action,state,created_at) VALUES(?,?,?,?,?,?)", in.PrinterID, job.ID, u.ID, in.Action, state, time.Now().UnixMilli())
		if e != nil {
			return e
		}
		commandID, e = r.LastInsertId()
		return e
	})
	if err != nil {
		return err
	}
	if in.Action == "resolve" {
		return nil
	}
	s.workers.Add(1)
	go func() {
		defer s.workers.Done()
		p, e := s.devicePrinter(in.PrinterID)
		if e == nil {
			ctx, cancel := context.WithTimeout(s.ctx, 25*time.Second)
			defer cancel()
			e = driver.Control(ctx, p, in.Action)
			if e == nil && in.Action != "stop" && !jobMatches(job, s.Device.Status(p.ID)) {
				e = fmt.Errorf("device task changed during control")
			}
		}
		_ = s.transaction(func(tx *sql.Tx) error {
			state := "succeeded"
			message := "设备状态已确认"
			if e != nil {
				state = "uncertain"
				message = "指令结果未确认，请核对设备后处理"
			} else {
				next := map[string]string{"pause": "paused", "resume": "printing", "stop": "stopped"}[in.Action]
				if _, err := tx.Exec("UPDATE jobs SET state=?,error='' WHERE id=?", next, job.ID); err != nil {
					return err
				}
			}
			if e != nil {
				if _, err := tx.Exec("UPDATE jobs SET error=? WHERE id=?", message, job.ID); err != nil {
					return err
				}
			}
			_, err := tx.Exec("UPDATE printer_commands SET state=?,message=?,finished_at=? WHERE id=?", state, message, time.Now().UnixMilli(), commandID)
			return err
		})
	}()
	return nil
}
func jobMatches(j Job, t Telemetry) bool {
	name := fmt.Sprintf("job-%d", j.ID)
	return sameTaskName(t.File, j.Remote) || sameTaskName(t.GcodeFile, j.Remote) || sameTaskName(t.File, name)
}
func sameTaskName(a, b string) bool {
	return a != "" && b != "" && strings.EqualFold(filepath.Base(strings.ReplaceAll(a, "\\", "/")), b)
}
