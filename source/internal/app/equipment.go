package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

func (s *Service) SetEquipment(u User, in SettingInput) error {
	if s.Config.DeviceMode == "offline" {
		return forbidden("设备连接关闭")
	}
	driver, ok := s.Device.(EquipmentController)
	if !ok {
		return conflict("驱动没有提供设备设置")
	}
	var commandID int64
	err := s.transaction(func(tx *sql.Tx) error {
		var enabled bool
		if e := tx.QueryRow("SELECT enabled FROM printers WHERE id=?", in.PrinterID).Scan(&enabled); e != nil {
			return missing("设备不存在")
		}
		if !enabled {
			return conflict("设备已停用")
		}
		t := s.Device.Status(in.PrinterID)
		if !t.Connected || !timeFresh(t.Updated) || !timeFresh(t.StateUpdated) {
			return conflict("设备状态未知或过期，请等待新状态")
		}
		if t.Equipment.ControlIssue == "authorization_required" && in.Key != "chamber_light" && in.Key != "chamber_light2" && in.Key != "resolve_settings" {
			return conflict("打印机报告指令授权校验失败，请在打印机上核对局域网与开发者模式，并处理授权告警后重试")
		}
		jobs, e := readJobs(tx)
		if e != nil {
			return e
		}
		var current *Job
		for i := range jobs {
			j := &jobs[i]
			if j.Printer != in.PrinterID {
				continue
			}
			if j.State == "starting" || j.State == "uploading" {
				return conflict("设备正在传输或启动任务")
			}
			if j.State == "printing" || j.State == "paused" {
				current = j
			}
		}
		if t.State == "printing" || t.State == "pause" {
			if current == nil || !jobMatches(*current, t) {
				return conflict("当前是未认领或不匹配的设备任务")
			}
			if current.UserID != u.ID && !u.Admin {
				return forbidden("只有任务本人或管理员可以调整设备")
			}
		} else if !u.Admin {
			return forbidden("空闲设备设置由管理员操作")
		}
		if t.State != "idle" && t.State != "printing" && t.State != "pause" {
			return conflict("当前设备状态不允许设置")
		}
		if current != nil && current.Error != "" {
			return conflict("任务执行结果待核对")
		}
		var pending int
		if e = tx.QueryRow("SELECT COUNT(*) FROM printer_commands WHERE printer_id=? AND state='pending'", in.PrinterID).Scan(&pending); e != nil {
			return e
		}
		if pending > 0 {
			return conflict("请等待上一条设备指令确认")
		}
		if in.Key == "resolve_settings" {
			var n int
			if e = tx.QueryRow("SELECT COUNT(*) FROM printer_commands WHERE printer_id=? AND state='uncertain' AND action LIKE 'set:%'", in.PrinterID).Scan(&n); e != nil {
				return e
			}
			if n == 0 {
				return conflict("没有待核对的设备设置")
			}
			if !u.Admin {
				var other int
				if e = tx.QueryRow("SELECT COUNT(*) FROM printer_commands WHERE printer_id=? AND state='uncertain' AND action LIKE 'set:%' AND user_id!=?", in.PrinterID, u.ID).Scan(&other); e != nil {
					return e
				}
				if other > 0 {
					return forbidden("请由原操作者或管理员核对")
				}
			}
			_, e = tx.Exec("UPDATE printer_commands SET state='reviewed',message=message || ' · 已由 ' || ? || ' 核对设备实际状态',finished_at=? WHERE printer_id=? AND state='uncertain' AND action LIKE 'set:%'", u.Name, time.Now().UnixMilli(), in.PrinterID)
			return e
		}
		var uncertain int
		if e = tx.QueryRow("SELECT COUNT(*) FROM printer_commands WHERE printer_id=? AND state='uncertain'", in.PrinterID).Scan(&uncertain); e != nil {
			return e
		}
		if uncertain > 0 {
			return conflict("上次指令结果不明，请先核对")
		}
		var setting *SettingReading
		for i := range t.Equipment.Settings {
			v := &t.Equipment.Settings[i]
			if v.Key == in.Key {
				setting = v
			}
		}
		if setting == nil || !timeFresh(setting.Updated) {
			return conflict("设备未提供此项的有效新状态")
		}
		if in.Value < setting.Min || in.Value > setting.Max {
			return invalid("设置值超出允许范围")
		}
		if in.Key == "speed" && t.State != "printing" && t.State != "pause" {
			return conflict("速度档位在打印任务中调整")
		}
		jobID := int64(0)
		if current != nil {
			jobID = current.ID
		}
		r, e := tx.Exec("INSERT INTO printer_commands(printer_id,job_id,user_id,action,state,message,created_at) VALUES(?,?,?,?,'pending',?,?)", in.PrinterID, jobID, u.ID, "set:"+in.Key, fmt.Sprintf("目标 %d %s", in.Value, setting.Unit), time.Now().UnixMilli())
		if e != nil {
			return e
		}
		commandID, e = r.LastInsertId()
		return e
	})
	if err != nil || in.Key == "resolve_settings" {
		return err
	}
	s.workers.Add(1)
	go func() {
		defer s.workers.Done()
		p, e := s.devicePrinter(in.PrinterID)
		if e == nil {
			ctx, cancel := context.WithTimeout(s.ctx, 25*time.Second)
			defer cancel()
			e = driver.SetEquipment(ctx, p, in)
		}
		_ = s.transaction(func(tx *sql.Tx) error {
			state, message := "succeeded", "设备设置已确认"
			if e != nil {
				state = "uncertain"
				message = "设备设置结果未确认，请现场核对"
				if errors.Is(e, ErrDeviceRejected) {
					state, message = "rejected", "打印机已明确拒绝设置，请检查设备授权或告警；没有自动重试"
				}
			}
			_, err := tx.Exec("UPDATE printer_commands SET state=?,message=message || ' · ' || ?,finished_at=? WHERE id=?", state, message, time.Now().UnixMilli(), commandID)
			return err
		})
	}()
	return nil
}

func redactEquipment(p *Printer, hidden []Mount) {
	p.Equipment.Feeds = append([]FeedReading(nil), p.Equipment.Feeds...)
	for _, m := range hidden {
		if m.PrinterID != p.ID {
			continue
		}
		p.AMS = nil
		for i := range p.Equipment.Feeds {
			f := &p.Equipment.Feeds[i]
			if f.Slot == m.Slot || p.ModelID == "h2d-pro" && m.Slot == "5" && f.Slot == "ams:128:0" {
				f.Material = "个人耗材"
				f.Color = ""
				f.ProfileID = ""
				f.Remaining = nil
				f.RemainingUnit = ""
			}
		}
	}
}
