package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type FilamentReading struct {
	Supported   bool      `json:"supported"`
	Extruder    int       `json:"extruder"`
	HasFilament bool      `json:"hasFilament"`
	Busy        bool      `json:"busy"`
	Step        int       `json:"step"`
	MainState   int       `json:"mainState"`
	Source      string    `json:"source"`
	Target      string    `json:"target"`
	Updated     time.Time `json:"updated"`
}
type FilamentInput struct {
	PrinterID int64  `json:"printerId"`
	Action    string `json:"action"`
	Slot      string `json:"slot"`
	RequestID string `json:"requestId"`
	Clear     bool   `json:"clear"`
	CommandID int64  `json:"commandId"`
}
type FilamentPlan struct {
	Unload           bool
	Slot             int
	Source           string
	OldTemp, NewTemp int
}
type FilamentController interface {
	ChangeFilament(context.Context, Printer, FilamentInput) error
}

func filamentIdle(t Telemetry) error {
	f := t.Equipment.Filament
	if !t.Connected || !motionFresh(t.Updated) || !motionFresh(t.StateUpdated) || !motionFresh(f.Updated) {
		return conflict("供料状态未知或过期，请重新读取设备状态")
	}
	if t.RawState != "IDLE" && t.RawState != "FINISH" {
		return conflict("仅空闲设备允许进换退料")
	}
	if !f.Supported {
		return conflict("当前仅开放 X2D 主喷嘴与 AMS 2 Pro 的 PETG 供料控制")
	}
	if t.PrintError != 0 || len(t.Equipment.HMS) > 0 || t.Equipment.ControlIssue != "" || t.Equipment.Switching || f.Busy || f.Step != 0 || (f.MainState != 0 && f.MainState != 3) {
		return conflict("请先等待供料结束并处理设备告警")
	}
	return nil
}
func feedTemperature(t Telemetry, slot string) (int, error) {
	for _, f := range t.Equipment.Feeds {
		if f.Slot != slot {
			continue
		}
		if f.Present == nil || !*f.Present || f.Material != "PETG" || !motionFresh(f.ControlUpdated) || f.NozzleMin == nil || f.NozzleMax == nil || *f.NozzleMin < 200 || *f.NozzleMax > 300 || *f.NozzleMin > *f.NozzleMax || !regexp.MustCompile(`^[0-9a-fA-F]{6}([0-9a-fA-F]{2})?$`).MatchString(f.Color) {
			return 0, conflict("料槽需提供新鲜的 PETG、颜色与有效温度范围")
		}
		return (*f.NozzleMin + *f.NozzleMax) / 2, nil
	}
	return 0, conflict("设备未提供此料槽")
}
func PlanFilament(p Printer, t Telemetry, in FilamentInput) (FilamentPlan, error) {
	plan := FilamentPlan{OldTemp: -1}
	if p.ModelID != "x2d" || !p.Enabled || p.Configuration != "ams" {
		return plan, conflict("当前仅开放 X2D 主喷嘴与 AMS 2 Pro 的 PETG 供料控制")
	}
	if e := filamentIdle(t); e != nil {
		return plan, e
	}
	f := t.Equipment.Filament
	if f.HasFilament {
		if f.Source != "1" && f.Source != "2" && f.Source != "3" && f.Source != "4" {
			return plan, conflict("当前料源未确认，不能自动换料")
		}
		v, e := feedTemperature(t, f.Source)
		if e != nil {
			return plan, e
		}
		plan.OldTemp = v
	}
	switch in.Action {
	case "load":
		if in.Slot != "1" && in.Slot != "2" && in.Slot != "3" && in.Slot != "4" {
			return plan, invalid("请选择 AMS 2 Pro 的 1 至 4 号料槽")
		}
		if f.HasFilament && f.Source == in.Slot {
			return plan, conflict("该料槽已装入主喷嘴")
		}
		temp, e := feedTemperature(t, in.Slot)
		if e != nil {
			return plan, e
		}
		n, _ := strconv.Atoi(in.Slot)
		plan.Slot = n - 1
		plan.Source = in.Slot
		plan.NewTemp = temp
	case "unload":
		if in.Slot != "" {
			return plan, invalid("退料不应指定新料槽")
		}
		if !f.HasFilament {
			return plan, conflict("主喷嘴当前未检测到已装入的耗材")
		}
		plan.Unload = true
		plan.Slot = 255
		plan.NewTemp = plan.OldTemp
	default:
		return plan, invalid("无效的供料动作")
	}
	return plan, nil
}
func FilamentPayload(p FilamentPlan, sequence string) []byte {
	b, _ := json.Marshal(map[string]any{"print": map[string]any{"command": "ams_change_filament", "sequence_id": sequence, "ams_id": 0, "slot_id": p.Slot, "target": p.Slot, "curr_temp": p.OldTemp, "tar_temp": p.NewTemp, "extruder_id": 1}})
	return b
}
func FilamentComplete(t Telemetry, p FilamentPlan, sent time.Time) bool {
	f := t.Equipment.Filament
	if filamentIdle(t) != nil || !f.Updated.After(sent) {
		return false
	}
	if p.Unload {
		return !f.HasFilament && f.Source == ""
	}
	return f.HasFilament && f.Source == p.Source && f.Target == p.Source
}
func (s *Service) ChangeFilament(actor User, in FilamentInput) error {
	if !in.Clear || !regexp.MustCompile(`^[A-Za-z0-9_-]{16,80}$`).MatchString(in.RequestID) {
		return invalid("请现场确认打印板及供料路径安全")
	}
	if in.Action != "load" && in.Action != "unload" && in.Action != "review" {
		return invalid("无效的供料动作")
	}
	driver, ok := s.Device.(FilamentController)
	if !ok || s.Config.DeviceMode != "live" {
		return conflict("设备驱动未开放实机供料控制")
	}
	prefix := fmt.Sprintf("requestID=%s;action=%s;slot=%s;", in.RequestID, in.Action, in.Slot)
	var id int64
	replay := false
	e := s.transaction(func(tx *sql.Tx) error {
		u, e := currentUser(tx, actor.ID)
		if e != nil {
			return e
		}
		if !u.Admin {
			return forbidden("进换退料仅管理员可操作")
		}
		p := Printer{ID: in.PrinterID}
		if e = tx.QueryRow("SELECT model_id,configuration,enabled FROM printers WHERE id=?", p.ID).Scan(&p.ModelID, &p.Configuration, &p.Enabled); e != nil {
			return missing("设备不存在")
		}
		if !p.Enabled || p.ModelID != "x2d" {
			return conflict("当前仅开放 X2D 主喷嘴与 AMS 2 Pro 的 PETG 供料控制")
		}
		var prior string
		e = tx.QueryRow("SELECT message FROM printer_commands WHERE printer_id=? AND user_id=? AND substr(message,1,?)=? LIMIT 1", p.ID, u.ID, len("requestID="+in.RequestID+";"), "requestID="+in.RequestID+";").Scan(&prior)
		if e == nil {
			if strings.HasPrefix(prior, prefix) {
				replay = true
				return nil
			}
			return conflict("供料请求编号已使用")
		}
		if e != sql.ErrNoRows {
			return e
		}
		t := s.Device.Status(p.ID)
		if in.Action == "review" {
			if e = filamentIdle(t); e != nil {
				return e
			}
			var action, message string
			if e = tx.QueryRow("SELECT action,message FROM printer_commands WHERE id=? AND printer_id=? AND state='uncertain'", in.CommandID, p.ID).Scan(&action, &message); e != nil {
				return conflict("供料核对记录已变化")
			}
			f := t.Equipment.Filament
			if action == "filament:unload" {
				if f.HasFilament {
					return conflict("设备尚未确认退料完成")
				}
			} else if action == "filament:load" {
				match := regexp.MustCompile(`;slot=([1-4]);`).FindStringSubmatch(message)
				if len(match) != 2 || !f.HasFilament || f.Source != match[1] || f.Target != match[1] {
					return conflict("设备料源与本次目标不一致，请先现场处理")
				}
			} else {
				return conflict("不是可核对的供料记录")
			}
			_, e = tx.Exec("UPDATE printer_commands SET state='reviewed',message=message||' · 管理员现场确认供料结束、停稳无异常',finished_at=? WHERE id=? AND state='uncertain'", time.Now().UnixMilli(), in.CommandID)
			return e
		}
		if _, e = PlanFilament(p, t, in); e != nil {
			return e
		}
		hold, e := pendingOrphanCompletion(tx, p.ID, t)
		if e != nil {
			return e
		}
		if hold != nil {
			return conflict("请先完成本机任务的取件和用量登记")
		}
		jobs, e := readJobs(tx)
		if e != nil {
			return e
		}
		for _, j := range jobs {
			if j.Printer == p.ID && activeStates[j.State] && j.State != "queued" {
				return conflict("请先完成本机任务的取件和用量登记")
			}
		}
		var n int
		if e = tx.QueryRow("SELECT COUNT(*) FROM printer_commands WHERE printer_id=? AND state IN ('pending','uncertain')", p.ID).Scan(&n); e != nil {
			return e
		}
		if n > 0 {
			return conflict("请等待或核对上一条设备指令")
		}
		r, e := tx.Exec("INSERT INTO printer_commands(printer_id,job_id,user_id,action,state,message,created_at) VALUES(?,0,?,?,'pending',?,?)", p.ID, u.ID, "filament:"+in.Action, prefix+"仅发送一次，等待设备流程结束", time.Now().UnixMilli())
		if e != nil {
			return e
		}
		id, e = r.LastInsertId()
		return e
	})
	if e != nil || replay || in.Action == "review" {
		return e
	}
	s.workers.Add(1)
	go func() {
		defer s.workers.Done()
		p, e := s.devicePrinter(in.PrinterID)
		if e == nil {
			ctx, cancel := context.WithTimeout(s.ctx, 8*time.Minute)
			defer cancel()
			e = driver.ChangeFilament(ctx, p, in)
		}
		state, message := "succeeded", "设备已确认供料流程结束，请现场检查出料或回卷"
		if e != nil {
			state, message = "uncertain", "供料结果未确认，请现场核对；没有自动重试"
			if errors.Is(e, ErrDeviceRejected) {
				state, message = "rejected", "设备已拒绝供料指令，没有自动重试"
			}
		}
		_ = s.transaction(func(tx *sql.Tx) error {
			_, err := tx.Exec("UPDATE printer_commands SET state=?,message=message||' · '||?,finished_at=? WHERE id=? AND state='pending'", state, message, time.Now().UnixMilli(), id)
			return err
		})
	}()
	return nil
}
