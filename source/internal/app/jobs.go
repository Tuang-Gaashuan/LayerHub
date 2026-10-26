package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"
)

type Schedule struct {
	JobID    int64                `json:"jobId"`
	Printer  int64                `json:"printer"`
	Estimate int                  `json:"estimate"`
	Grams    int                  `json:"grams"`
	SpoolID  string               `json:"spoolId"`
	Expected *ScheduleExpectation `json:"expected,omitempty"`
}
type ScheduleExpectation struct {
	Printer  int64  `json:"printer"`
	Estimate int    `json:"estimate"`
	Grams    int    `json:"grams"`
	SpoolID  string `json:"spoolId"`
	Order    int64  `json:"order"`
}

func timeFresh(t time.Time) bool { return !t.IsZero() && time.Since(t) < 90*time.Second }
func validateSchedule(tx *sql.Tx, u User, in Schedule, exclude int64) error {
	if in.Estimate < 1 || in.Estimate > 100000 || in.Grams < 1 || in.Grams > 10000 {
		return invalid("预计时长或用量无效")
	}
	if in.Printer != 0 {
		var enabled bool
		if err := tx.QueryRow("SELECT enabled FROM printers WHERE id=?", in.Printer).Scan(&enabled); err != nil || !enabled {
			return invalid("目标设备不存在或已停用")
		}
	}
	spools, err := readSpools(tx)
	if err != nil {
		return err
	}
	jobs, err := readJobs(tx)
	if err != nil {
		return err
	}
	for _, sp := range spools {
		if sp.ID == in.SpoolID {
			if !spoolVisible(sp, u) {
				return forbidden("无权使用这卷耗材")
			}
			if sp.State != "available" {
				return conflict("耗材当前不可用")
			}
			if sp.Remaining-reserve(jobs, sp.ID, exclude) < in.Grams {
				return conflict("耗材可分配余量不足")
			}
			return nil
		}
	}
	return missing("请选择存在且可用的耗材")
}
func (s *Service) CreateJob(u User, in Schedule, filename, path, hash, note string, size int64) (int64, error) {
	if len(hash) != 64 || size < 1 {
		return 0, invalid("上传文件校验信息无效")
	}
	var id int64
	err := s.transaction(func(tx *sql.Tx) error {
		if err := validateSchedule(tx, u, in, 0); err != nil {
			return err
		}
		var order int64
		if err := tx.QueryRow("SELECT COALESCE(MAX(queue_order),0)+1 FROM jobs").Scan(&order); err != nil {
			return err
		}
		var pid any
		if in.Printer != 0 {
			pid = in.Printer
		}
		r, err := tx.Exec(`INSERT INTO jobs(user_id,filename,path,size,sha256,printer_id,state,note,created_at,queue_order,estimate,grams,spool_id) VALUES(?,?,?,?,?,?,'queued',?,?,?,?,?,?)`, u.ID, filename, path, size, hash, pid, trim(note, 200), time.Now().UnixMilli(), order, in.Estimate, in.Grams, in.SpoolID)
		if err != nil {
			return err
		}
		id, err = r.LastInsertId()
		return err
	})
	return id, err
}
func (s *Service) ScheduleJob(u User, in Schedule) error {
	return s.transaction(func(tx *sql.Tx) error {
		jobs, err := readJobs(tx)
		if err != nil {
			return err
		}
		for _, j := range jobs {
			if j.ID != in.JobID {
				continue
			}
			if j.UserID != u.ID && !u.Admin {
				return forbidden("无权调整其他用户的任务")
			}
			if j.State != "queued" {
				return conflict("只有等待任务可以调整")
			}
			if e := in.Expected; e != nil && (e.Printer != j.Printer || e.Estimate != j.Estimate || e.Grams != j.Grams || e.Order != j.Order || e.SpoolID != "" && e.SpoolID != j.SpoolID) {
				return conflict("任务安排已被其他成员修改，请重新打开任务后再保存")
			}
			if err = validateSchedule(tx, u, in, j.ID); err != nil {
				return err
			}
			order := j.Order
			if in.Printer != j.Printer {
				if err = tx.QueryRow("SELECT COALESCE(MAX(queue_order),0)+1 FROM jobs").Scan(&order); err != nil {
					return err
				}
			}
			var pid any
			if in.Printer != 0 {
				pid = in.Printer
			}
			_, err = tx.Exec("UPDATE jobs SET printer_id=?,estimate=?,grams=?,spool_id=?,queue_order=?,error='',print_options='' WHERE id=?", pid, in.Estimate, in.Grams, in.SpoolID, order, j.ID)
			return err
		}
		return missing("任务不存在")
	})
}
func (s *Service) DeleteJob(u User, id int64) error {
	var path string
	err := s.transaction(func(tx *sql.Tx) error {
		jobs, err := readJobs(tx)
		if err != nil {
			return err
		}
		for _, j := range jobs {
			if j.ID != id {
				continue
			}
			if j.UserID != u.ID && !u.Admin {
				return forbidden("无权删除其他用户的任务")
			}
			if j.State != "queued" && j.State != "assigned" && j.State != "failed" {
				return conflict("该任务正在执行，不能删除")
			}
			path = j.Path
			_, err = tx.Exec("DELETE FROM jobs WHERE id=?", id)
			return err
		}
		return missing("任务不存在")
	})
	if err == nil && path != "" {
		_ = os.Remove(path)
	}
	return err
}
func (s *Service) CompleteJob(u User, id int64, used int) error {
	return s.CompleteJobUses(u, id, used, nil)
}
func (s *Service) CompleteJobUses(u User, id int64, used int, actual map[string]int) error {
	return s.transaction(func(tx *sql.Tx) error {
		return s.completeJobUsesTx(tx, u, id, used, actual)
	})
}
func (s *Service) completeJobUsesTx(tx *sql.Tx, u User, id int64, used int, actual map[string]int) error {
	jobs, err := readJobs(tx)
	if err != nil {
		return err
	}
	var job *Job
	for i := range jobs {
		if jobs[i].ID == id {
			job = &jobs[i]
		}
	}
	if job == nil {
		return missing("任务不存在")
	}
	if job.UserID != u.ID && !u.Admin {
		return forbidden("无权确认其他用户的任务")
	}
	if job.State != "printing" && job.State != "paused" && job.State != "stopped" {
		return conflict("任务尚未打印，或已确认完成")
	}
	var pending int
	if err = tx.QueryRow("SELECT COUNT(*) FROM printer_commands WHERE printer_id=? AND state IN ('pending','uncertain')", job.Printer).Scan(&pending); err != nil {
		return err
	}
	if pending > 0 {
		return conflict("先确认设备控制结果，再登记实际用量")
	}
	if s.Config.DeviceMode == "live" {
		t := s.Device.Status(job.Printer)
		if !t.Connected || !timeFresh(t.Updated) || t.State != "idle" && t.State != "failed" {
			return conflict("设备尚未确认结束，请先完成或停止打印")
		}
	}
	uses := jobUses(*job)
	if actual == nil {
		if len(uses) > 1 {
			return invalid("请分别填写每卷耗材的实际用量")
		}
		actual = map[string]int{job.SpoolID: used}
	}
	if len(actual) != len(uses) {
		return invalid("实际用量需覆盖本任务每卷耗材")
	}
	spools, err := readSpools(tx)
	if err != nil {
		return err
	}
	total := 0
	count := 0
	for _, sp := range spools {
		if _, exists := uses[sp.ID]; exists {
			grams, ok := actual[sp.ID]
			if !ok || grams < 0 || grams > sp.Remaining-reserve(jobs, sp.ID, id) {
				return conflict("实际用量超过可分配余量")
			}
			_, err = tx.Exec("UPDATE spools SET remaining=remaining-?,state=CASE WHEN remaining-?=0 THEN 'empty' ELSE state END WHERE id=?", grams, grams, sp.ID)
			if err != nil {
				return err
			}
			total += grams
			if _, err = tx.Exec("INSERT INTO job_consumption(job_id,spool_id,grams) VALUES(?,?,?)", id, sp.ID, grams); err != nil {
				return err
			}
			count++
		}
	}
	if count != len(uses) {
		return missing("关联耗材不存在")
	}
	if err = s.observePrintFinished(tx, *job, s.Device.Status(job.Printer), time.Now().Unix()); err != nil {
		return err
	}
	next := "done"
	if job.State == "stopped" {
		next = "cancelled"
	}
	_, err = tx.Exec("UPDATE jobs SET state=?,actual_grams=?,error='' WHERE id=?", next, total, id)
	return err
}
func (s *Service) executable(tx *sql.Tx, u User, id int64, expected string) (Job, error) {
	jobs, err := readJobs(tx)
	if err != nil {
		return Job{}, err
	}
	var j Job
	found := false
	for _, candidate := range jobs {
		if candidate.ID == id {
			j = candidate
			found = true
		}
	}
	if !found {
		return j, missing("任务不存在")
	}
	if j.UserID != u.ID && !u.Admin {
		return j, forbidden("无权操作其他用户的任务")
	}
	if j.State != expected {
		return j, conflict("任务状态已变化，请刷新")
	}
	if j.Printer == 0 {
		return j, conflict("请先明确确认目标打印机")
	}
	var enabled bool
	var model string
	if err = tx.QueryRow("SELECT enabled,model_id FROM printers WHERE id=?", j.Printer).Scan(&enabled, &model); err != nil || !enabled {
		return j, conflict("设备已停用")
	}
	if strings.EqualFold(filepath.Ext(j.Filename), ".3mf") && strings.HasPrefix(model, "h2") && model != "h2s" {
		return j, conflict("此机型的多喷嘴启动映射尚未接入；准备可保存，暂不能传输或启动")
	}
	for _, other := range jobs {
		if other.Printer != j.Printer || other.ID == j.ID {
			continue
		}
		if other.State != "queued" && activeStates[other.State] {
			return j, conflict("设备已有执行中或待确认的任务")
		}
		if other.State == "queued" && other.Order < j.Order {
			return j, conflict("请按本机队列顺序执行任务")
		}
	}
	t := s.Device.Status(j.Printer)
	if !t.Connected || !timeFresh(t.Updated) {
		return j, conflict("设备未连接或状态已过期")
	}
	if t.State != "idle" {
		return j, conflict("设备尚未空闲")
	}
	var uncertain int
	if err = tx.QueryRow("SELECT COUNT(*) FROM printer_commands WHERE printer_id=? AND state IN ('pending','uncertain')", j.Printer).Scan(&uncertain); err != nil {
		return j, err
	}
	if uncertain > 0 {
		return j, conflict("有未确认的设备指令，请先核对设备")
	}
	if j.PrintOptions != nil {
		ps, e := readPrinters(tx)
		if e != nil {
			return j, e
		}
		for _, p := range ps {
			if p.ID == j.Printer {
				err = validatePrintOptions(tx, u, j, p, *j.PrintOptions)
			}
		}
	} else {
		err = validateSchedule(tx, u, Schedule{Printer: j.Printer, Estimate: j.Estimate, Grams: j.Grams, SpoolID: j.SpoolID}, j.ID)
	}
	if err != nil {
		return j, err
	}
	if model == "x2d" {
		if err = x2dPrintReady(j, t); err != nil {
			return j, err
		}
	}
	spools, err := readSpools(tx)
	if err != nil {
		return j, err
	}
	for _, sp := range spools {
		if sp.ID == j.SpoolID && sp.Mount != nil && sp.Mount.PrinterID != j.Printer {
			return j, conflict("耗材装在另一台设备，请先转移并更新装载记录")
		}
	}
	return j, nil
}
func (s *Service) SendJob(u User, id int64) error {
	return s.sendJob(u, id, false)
}

// DispatchJob consumes one explicit print confirmation. It never schedules a
// later automatic print, retries an uncertain start, or resumes after restart.
func (s *Service) DispatchJob(u User, id int64) error {
	if !s.Config.EnablePrintStart {
		return forbidden("远程启动未启用")
	}
	var state string
	if err := s.DB.QueryRow("SELECT state FROM jobs WHERE id=?", id).Scan(&state); err != nil {
		return missing("任务不存在")
	}
	if state == "assigned" {
		return s.StartJob(u, id)
	}
	return s.sendJob(u, id, true)
}

func (s *Service) sendJob(u User, id int64, startAfter bool) error {
	select {
	case s.transfers <- struct{}{}:
	default:
		return conflict("传输通道繁忙，请稍后重试")
	}
	var job Job
	err := s.transaction(func(tx *sql.Tx) error {
		var err error
		job, err = s.executable(tx, u, id, "queued")
		if err != nil {
			return err
		}
		if startAfter {
			if err = s.validateStart(tx, job); err != nil {
				return err
			}
		}
		_, err = tx.Exec("UPDATE jobs SET state='uploading',error='' WHERE id=?", id)
		return err
	})
	if err != nil {
		<-s.transfers
		return err
	}
	s.workers.Add(1)
	go func() {
		defer s.workers.Done()
		defer func() { <-s.transfers }()
		p, err := s.devicePrinter(job.Printer)
		remote := fmt.Sprintf("job-%d-%s%s", job.ID, job.SHA256[:12], strings.ToLower(filepath.Ext(job.Filename)))
		if err == nil {
			ctx, cancel := context.WithTimeout(s.ctx, 10*time.Minute)
			defer cancel()
			err = s.Device.Upload(ctx, p, job.Path, remote, func(n int64) {})
		}
		persistErr := s.transaction(func(tx *sql.Tx) error {
			if err != nil {
				_, e := tx.Exec("UPDATE jobs SET state='queued',error=? WHERE id=? AND state='uploading'", "文件传输失败，请核对连接后重试", job.ID)
				return e
			}
			r, e := tx.Exec("UPDATE jobs SET state='assigned',remote_path=?,error='' WHERE id=? AND state='uploading'", remote, job.ID)
			if e != nil {
				return e
			}
			n, e := r.RowsAffected()
			if e == nil && n != 1 {
				return conflict("传输期间任务状态已变化")
			}
			return e
		})
		if err == nil && persistErr == nil && startAfter && s.ctx.Err() == nil {
			// Recheck ownership, user access, queue, telemetry and unchanged
			// preparation after the potentially long transfer.
			if startErr := s.startJob(u, job.ID, &job); startErr != nil {
				_ = s.transaction(func(tx *sql.Tx) error {
					_, e := tx.Exec("UPDATE jobs SET error=? WHERE id=? AND state='assigned' AND error=''", "文件已传输，自动启动已停止："+startErr.Error(), job.ID)
					return e
				})
			}
		}
	}()
	return nil
}
func (s *Service) StartJob(u User, id int64) error {
	return s.startJob(u, id, nil)
}

func (s *Service) startJob(u User, id int64, expected *Job) error {
	if !s.Config.EnablePrintStart {
		return forbidden("远程启动未启用；请先单独验证真实设备")
	}
	var job Job
	var commandID int64
	err := s.transaction(func(tx *sql.Tx) error {
		var enabled bool
		if e := tx.QueryRow("SELECT NOT disabled,admin FROM users WHERE id=?", u.ID).Scan(&enabled, &u.Admin); e != nil || !enabled {
			return forbidden("账户已停用，请重新登录")
		}
		var e error
		job, e = s.executable(tx, u, id, "assigned")
		if e != nil {
			return e
		}
		if expected != nil && (job.Printer != expected.Printer || job.SpoolID != expected.SpoolID || job.SHA256 != expected.SHA256 || !samePrintPreparation(job, *expected)) {
			return conflict("打印安排已变化，请重新确认")
		}
		if e = s.validateStart(tx, job); e != nil {
			return e
		}
		if job.Error != "" {
			return conflict("上次启动结果未确认，请先核对设备")
		}
		_, e = tx.Exec("UPDATE jobs SET state='starting',error='' WHERE id=?", id)
		if e != nil {
			return e
		}
		r, e := tx.Exec("INSERT INTO printer_commands(printer_id,job_id,user_id,action,state,created_at) VALUES(?,?,?,'start','pending',?)", job.Printer, job.ID, u.ID, time.Now().UnixMilli())
		if e == nil {
			commandID, e = r.LastInsertId()
		}
		return e
	})
	if err != nil {
		return err
	}
	s.workers.Add(1)
	go func() {
		defer s.workers.Done()
		p, err := s.devicePrinter(job.Printer)
		if err == nil {
			ctx, cancel := context.WithTimeout(s.ctx, 45*time.Second)
			defer cancel()
			request := PrintRequest{Remote: job.Remote, TaskName: fmt.Sprintf("job-%d", job.ID), Options: job.PrintOptions}
			if p.ModelID == "x2d" {
				request.BedType = "pte"
			}
			if driver, ok := s.Device.(PrinterController); ok {
				err = driver.StartPrint(ctx, p, request)
			} else if job.PrintOptions != nil {
				err = fmt.Errorf("project driver unavailable")
			} else {
				err = s.Device.Start(ctx, p, job.Remote)
			}
		}
		_ = s.transaction(func(tx *sql.Tx) error {
			state, message := "succeeded", "设备已确认开始打印"
			if err != nil {
				state = "uncertain"
				message = "启动结果未确认，请核对设备"
				if errors.Is(err, ErrDeviceRejected) {
					state = "rejected"
					message = "打印机明确拒绝启动，请核对设备提示"
				}
			}
			if _, e := tx.Exec("UPDATE printer_commands SET state=?,message=?,finished_at=? WHERE id=?", state, message, time.Now().UnixMilli(), commandID); e != nil {
				return e
			}
			if err != nil {
				if errors.Is(err, ErrDeviceRejected) {
					_, e := tx.Exec("UPDATE jobs SET state='assigned',error=? WHERE id=? AND state='starting'", message, id)
					return e
				}
				_, e := tx.Exec("UPDATE jobs SET state='assigned',error='启动未获设备确认，请先核对设备再重试' WHERE id=? AND state='starting'", id)
				return e
			}
			_, e := tx.Exec("UPDATE jobs SET state='printing',error='' WHERE id=? AND state='starting'", id)
			return e
		})
	}()
	return nil
}

func samePrintPreparation(a, b Job) bool {
	return a.Grams == b.Grams && a.Estimate == b.Estimate && reflect.DeepEqual(a.PrintOptions, b.PrintOptions)
}

func (s *Service) validateStart(tx *sql.Tx, job Job) error {
	if !s.Config.EnablePrintStart {
		return forbidden("远程启动未启用")
	}
	if strings.EqualFold(filepath.Ext(job.Filename), ".3mf") && job.PrintOptions == nil {
		return conflict("请先确认 3MF 打印盘、供料映射与打印选项")
	}
	var model string
	if e := tx.QueryRow("SELECT model_id FROM printers WHERE id=?", job.Printer).Scan(&model); e != nil {
		return e
	}
	if model != "x2d" && (dualModel(model) || len(s.Device.Status(job.Printer).Equipment.Extruders) > 1) {
		return conflict("多喷嘴启动仍需实机适配，当前仅开放状态与供料展示")
	}
	spools, e := readSpools(tx)
	if e != nil {
		return e
	}
	for _, sp := range spools {
		if _, used := jobUses(job)[sp.ID]; used && sp.Mount != nil {
			slot := sp.Mount.Slot
			if slot != "external" && slot != "1" && slot != "2" && slot != "3" && slot != "4" {
				return conflict("扩展供料位置尚未通过启动适配，请先使用 Studio 启动")
			}
		}
	}
	return nil
}
