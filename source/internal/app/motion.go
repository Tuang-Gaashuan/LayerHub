package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// MotionReading carries observations, never inferred coordinates or homing.
type MotionReading struct {
	NativeHome     bool      `json:"nativeHome"`
	NativeJog      bool      `json:"nativeJog"`
	CapabilitiesAt time.Time `json:"capabilitiesAt"`
	HomeAt         time.Time `json:"homeAt"`
	HomeMask       *int      `json:"homeMask"`
}
type MotionInput struct {
	PrinterID int64  `json:"printerId"`
	Action    string `json:"action"`
	Axis      string `json:"axis"`
	Direction int    `json:"direction"`
	Step      int    `json:"step"`
	RequestID string `json:"requestId"`
	Clear     bool   `json:"clear"`
	CommandID int64  `json:"commandId"`
}
type MotionController interface {
	Move(context.Context, Printer, MotionInput) error
}

func motionFresh(t time.Time) bool {
	return !t.IsZero() && time.Since(t) >= -time.Second && time.Since(t) < 5*time.Second
}
func ValidateMotion(t Telemetry, in MotionInput) error {
	if !t.Connected || !motionFresh(t.StateUpdated) || !motionFresh(t.Updated) || !motionFresh(t.Equipment.Motion.CapabilitiesAt) {
		return conflict("移动状态未知或过期")
	}
	if t.RawState != "IDLE" && t.RawState != "FINISH" {
		return conflict("仅空闲设备允许手动移动")
	}
	if len(t.Equipment.HMS) > 0 || t.PrintError != 0 || t.Equipment.Switching || t.Equipment.ControlIssue != "" {
		return conflict("请先处理设备告警或供料切换")
	}
	m := t.Equipment.Motion
	if in.Action == "home" {
		if !m.NativeHome {
			return conflict("设备未提供原生归位能力")
		}
		return nil
	}
	if !m.NativeJog || !motionFresh(m.HomeAt) || m.HomeMask == nil || *m.HomeMask != 7 {
		return conflict("请先等待三轴回零确认")
	}
	return nil
}
func (s *Service) MovePrinter(actor User, in MotionInput) error {
	if !regexp.MustCompile(`^[A-Za-z0-9_-]{16,80}$`).MatchString(in.RequestID) || !in.Clear {
		return invalid("请现场确认运动路径清空")
	}
	if in.Action != "home" && in.Action != "jog" && in.Action != "review" {
		return invalid("无效的移动动作")
	}
	if in.Action == "jog" && (in.Axis != "X" && in.Axis != "Y" && in.Axis != "Z" || in.Direction != 1 && in.Direction != -1 || in.Step != 1) {
		return invalid("仅支持 XYZ 轴单次 1 mm 移动")
	}
	if in.Action != "jog" && (in.Axis != "" || in.Direction != 0 || in.Step != 0) {
		return invalid("无效的移动参数")
	}
	driver, ok := s.Device.(MotionController)
	if !ok || s.Config.DeviceMode == "offline" {
		return conflict("设备驱动未提供移动控制")
	}
	prefix := fmt.Sprintf("requestID=%s;action=%s;axis=%s;dir=%d;step=%d;", in.RequestID, in.Action, in.Axis, in.Direction, in.Step)
	var id int64
	replay := false
	err := s.transaction(func(tx *sql.Tx) error {
		u, e := currentUser(tx, actor.ID)
		if e != nil {
			return e
		}
		if !u.Admin {
			return forbidden("手动移动仅管理员可操作")
		}
		var enabled bool
		var model string
		if e = tx.QueryRow("SELECT enabled,model_id FROM printers WHERE id=?", in.PrinterID).Scan(&enabled, &model); e != nil {
			return missing("设备不存在")
		}
		if !enabled || model != "x2d" {
			return conflict("此机型尚未完成移动适配")
		}
		var prior, priorAction string
		e = tx.QueryRow("SELECT message,action FROM printer_commands WHERE printer_id=? AND user_id=? AND substr(message,1,?)=? ORDER BY id LIMIT 1", in.PrinterID, u.ID, len("requestID="+in.RequestID+";"), "requestID="+in.RequestID+";").Scan(&prior, &priorAction)
		if e == nil {
			if strings.HasPrefix(prior, prefix) && priorAction == "motion:"+in.Action {
				replay = true
				return nil
			}
			return conflict("移动请求编号已使用")
		}
		if e != sql.ErrNoRows {
			return e
		}
		t := s.Device.Status(in.PrinterID)
		if in.Action == "review" {
			if !t.Connected || !motionFresh(t.StateUpdated) || t.RawState != "IDLE" && t.RawState != "FINISH" || len(t.Equipment.HMS) > 0 || t.PrintError != 0 {
				return conflict("请等待设备停止并处理告警")
			}
			r, e := tx.Exec("UPDATE printer_commands SET state='reviewed',message=message||' · 管理员现场确认已停稳、无异常',finished_at=? WHERE id=? AND printer_id=? AND action LIKE 'motion:%' AND state='uncertain'", time.Now().UnixMilli(), in.CommandID, in.PrinterID)
			if e != nil {
				return e
			}
			n, _ := r.RowsAffected()
			if n != 1 {
				return conflict("移动核对记录已变化")
			}
			return nil
		}
		if e = ValidateMotion(t, in); e != nil {
			return e
		}
		jobs, e := readJobs(tx)
		if e != nil {
			return e
		}
		for _, j := range jobs {
			if j.Printer == in.PrinterID && activeStates[j.State] && j.State != "queued" {
				return conflict("请先完成本机任务的取件和用量登记")
			}
		}
		var count int
		if e = tx.QueryRow("SELECT count(*) FROM printer_commands WHERE printer_id=? AND state IN ('pending','uncertain')", in.PrinterID).Scan(&count); e != nil {
			return e
		}
		if count > 0 {
			return conflict("请等待或核对上一条设备指令")
		}
		r, e := tx.Exec("INSERT INTO printer_commands(printer_id,job_id,user_id,action,state,message,created_at) VALUES(?,0,?,?,'pending',?,?)", in.PrinterID, u.ID, "motion:"+in.Action, prefix+"移动验证：仅发送一次", time.Now().UnixMilli())
		if e != nil {
			return e
		}
		id, e = r.LastInsertId()
		return e
	})
	if err != nil || replay || in.Action == "review" {
		return err
	}
	s.workers.Add(1)
	go func() {
		defer s.workers.Done()
		p, e := s.devicePrinter(in.PrinterID)
		if e == nil {
			ctx, cancel := context.WithTimeout(s.ctx, 80*time.Second)
			defer cancel()
			e = driver.Move(ctx, p, in)
		}
		state, msg := "uncertain", "移动结果待现场核对，没有自动重试"
		if errors.Is(e, ErrDeviceRejected) {
			state, msg = "rejected", "设备已拒绝移动，没有自动重试"
		} else if e == nil && in.Action == "home" {
			state, msg = "succeeded", "三轴回零状态已确认"
		} else if e == nil {
			msg = "设备已接收 1 mm 点动，请现场确认已停稳、无异常"
		}
		_ = s.transaction(func(tx *sql.Tx) error {
			_, err := tx.Exec("UPDATE printer_commands SET state=?,message=message||' · '||?,finished_at=? WHERE id=? AND state='pending'", state, msg, time.Now().UnixMilli(), id)
			return err
		})
	}()
	return nil
}
