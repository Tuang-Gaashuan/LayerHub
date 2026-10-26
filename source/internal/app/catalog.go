package app

import (
	"database/sql"
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"
)

func (s *Service) SaveUser(actor User, username, name, password string, admin bool) error {
	if !actor.Admin {
		return forbidden("需要管理员权限")
	}
	username = strings.TrimSpace(username)
	name = trim(name, 32)
	if !regexp.MustCompile(`^[a-zA-Z0-9_]{3,24}$`).MatchString(username) || len(password) < 8 || len(password) > 256 || name == "" {
		return invalid("用户名需为 3–24 位字母、数字或下划线，密码至少 8 位")
	}
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	return s.transaction(func(tx *sql.Tx) error {
		fresh, e := currentUser(tx, actor.ID)
		if e != nil {
			return e
		}
		if !fresh.Admin {
			return forbidden("需要管理员权限")
		}
		if admin && !fresh.Owner {
			return forbidden("只有空间所有者可以创建管理员")
		}
		var exists int
		if err := tx.QueryRow("SELECT COUNT(*) FROM users WHERE username=? OR name=?", username, name).Scan(&exists); err != nil {
			return err
		}
		if exists > 0 {
			return conflict("用户名或显示名称已存在")
		}
		r, err := tx.Exec("INSERT INTO users(username,name,password_hash,admin) VALUES(?,?,?,?)", username, name, hash, admin)
		if err != nil {
			return err
		}
		id, err := r.LastInsertId()
		if err != nil {
			return err
		}
		role := "member"
		if admin {
			role = "admin"
		}
		_, err = tx.Exec("INSERT INTO role_audit(actor_id,user_id,old_role,new_role,reason,created) VALUES(?,?,'',?,'account_created',?)", actor.ID, id, role, time.Now().UnixMilli())
		return err
	})
}

type PrinterInput struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	IP            string `json:"ip"`
	Serial        string `json:"serial"`
	ModelID       string `json:"modelId"`
	Configuration string `json:"configuration"`
	Code          string `json:"code"`
	Fingerprint   string `json:"fingerprint"`
	Enabled       *bool  `json:"enabled"`
}

func (s *Service) SavePrinter(actor User, in PrinterInput) error {
	if !actor.Admin {
		return forbidden("需要管理员权限")
	}
	err := s.transaction(func(tx *sql.Tx) error {
		printers, err := readPrinters(tx)
		if err != nil {
			return err
		}
		var old *Printer
		for i := range printers {
			if printers[i].ID == in.ID {
				old = &printers[i]
			}
		}
		if in.ID != 0 && old == nil {
			return missing("打印机不存在")
		}
		if old != nil {
			if in.Name == "" {
				in.Name = old.Name
			}
			if in.IP == "" {
				in.IP = old.IP
			}
			if in.Serial == "" {
				in.Serial = old.Serial
			}
			if in.ModelID == "" {
				in.ModelID = old.ModelID
			}
			if in.Configuration == "" {
				in.Configuration = old.Configuration
			}
			if in.Fingerprint == "" {
				in.Fingerprint = old.Fingerprint
			}
		}
		if trim(in.Name, 40) == "" || net.ParseIP(in.IP) == nil || trim(in.Serial, 60) == "" || models[in.ModelID] == "" || (in.Configuration != "bare" && in.Configuration != "ams") {
			return invalid("请填写有效的设备名称、IP、序列号、机型与配置")
		}
		if in.Fingerprint != "" && !regexp.MustCompile(`^[0-9a-fA-F]{64}$`).MatchString(in.Fingerprint) {
			return invalid("TLS 指纹必须是 64 位 SHA-256 十六进制")
		}
		var duplicate int
		if err = tx.QueryRow("SELECT COUNT(*) FROM printers WHERE id!=? AND (ip=? OR serial=?)", in.ID, in.IP, in.Serial).Scan(&duplicate); err != nil {
			return err
		}
		if duplicate > 0 {
			return conflict("IP 或序列号已存在")
		}
		enabled := true
		if old != nil {
			enabled = old.Enabled
		}
		if in.Enabled != nil {
			enabled = *in.Enabled
		}
		if old != nil {
			var commands int
			if e := tx.QueryRow("SELECT COUNT(*) FROM printer_commands WHERE printer_id=? AND state IN ('pending','uncertain')", in.ID).Scan(&commands); e != nil {
				return e
			}
			if commands > 0 {
				return conflict("请先等待或核对设备指令，再修改连接配置")
			}
			jobs, e := readJobs(tx)
			if e != nil {
				return e
			}
			for _, j := range jobs {
				if j.Printer == in.ID && j.State != "queued" && activeStates[j.State] {
					return conflict("设备有传输或执行中的任务，暂不能修改")
				}
			}
			spools, e := readSpools(tx)
			if e != nil {
				return e
			}
			p := Printer{ModelID: in.ModelID, Configuration: in.Configuration}
			for _, sp := range spools {
				if sp.Mount != nil && sp.Mount.PrinterID == in.ID && !validSlot(p, sp.Mount.Slot) {
					return conflict("请先调整原 AMS 槽位上的耗材装载记录")
				}
			}
		}
		var encrypted []byte
		if in.Code != "" {
			if len(in.Code) > 128 {
				return invalid("设备访问码过长")
			}
			encrypted, err = s.seal(in.Code)
			if err != nil {
				return err
			}
		} else if old == nil {
			if s.Config.DeviceMode == "live" {
				return invalid("真实设备模式需要 Access Code")
			}
			encrypted, err = s.seal("")
			if err != nil {
				return err
			}
		} else {
			if err = tx.QueryRow("SELECT code FROM printers WHERE id=?", in.ID).Scan(&encrypted); err != nil {
				return err
			}
		}
		if old == nil {
			_, err = tx.Exec("INSERT INTO printers(name,ip,serial,model_id,configuration,enabled,code,fingerprint) VALUES(?,?,?,?,?,?,?,?)", trim(in.Name, 40), in.IP, trim(in.Serial, 60), in.ModelID, in.Configuration, enabled, encrypted, strings.ToLower(in.Fingerprint))
		} else {
			_, err = tx.Exec("UPDATE printers SET name=?,ip=?,serial=?,model_id=?,configuration=?,enabled=?,code=?,fingerprint=? WHERE id=?", trim(in.Name, 40), in.IP, trim(in.Serial, 60), in.ModelID, in.Configuration, enabled, encrypted, strings.ToLower(in.Fingerprint), in.ID)
		}
		return err
	})
	if err == nil {
		printers, e := s.printers()
		if e != nil {
			return e
		}
		for i := range printers {
			p, e := s.devicePrinter(printers[i].ID)
			if e != nil {
				return e
			}
			printers[i] = p
		}
		s.Device.Reconcile(s.ctx, printers)
	}
	return err
}
func (s *Service) SaveSpool(actor User, in Spool) error {
	if in.Mount != nil {
		m := *in.Mount
		m.Slot = normalizedSlot(m.Slot)
		in.Mount = &m
	}
	if in.Label = trim(in.Label, 60); in.Label == "" || trim(in.Brand, 40) == "" || trim(in.ColorName, 30) == "" || trim(in.Material, 20) == "" {
		return invalid("请填写耗材名称、品牌、材质和颜色")
	}
	if in.Capacity < 1 || in.Capacity > 10000 || in.Remaining < 0 || in.Remaining > in.Capacity || !regexp.MustCompile(`^#[0-9a-fA-F]{6}$`).MatchString(in.Color) {
		return invalid("净余量、标称重量或颜色无效")
	}
	if in.Remaining == 0 {
		in.State = "empty"
	}
	if in.State != "available" && in.State != "drying" && in.State != "reserved" && in.State != "empty" || in.State == "empty" && in.Remaining > 0 {
		return invalid("耗材状态无效")
	}
	return s.transaction(func(tx *sql.Tx) error {
		spools, err := readSpools(tx)
		if err != nil {
			return err
		}
		jobs, err := readJobs(tx)
		if err != nil {
			return err
		}
		var old *Spool
		for i := range spools {
			if spools[i].ID == in.ID {
				old = &spools[i]
			}
		}
		if in.ID != "" && old == nil {
			return missing("耗材不存在")
		}
		if old != nil && !spoolEditable(*old, actor) {
			return forbidden("只能修改本人耗材，公共耗材由管理员维护")
		}
		if old != nil && in.Revision > 0 && in.Revision != old.Revision {
			return conflict("耗材记录已被其他成员更新，请重新打开后再保存")
		}
		if old != nil && old.LowThreshold > in.Capacity {
			return conflict("提醒阈值大于新的标称净重，请先修改提醒设置")
		}
		var owner any
		if in.Owner != "team" {
			if in.Owner != actor.Username {
				return forbidden("只能登记自己的个人耗材")
			}
			owner = actor.ID
		} else {
			if old != nil && old.OwnerID != 0 && !actor.Admin {
				return forbidden("个人耗材转为公共耗材需要管理员处理")
			}
			in.Shared = false
		}
		if in.Remaining < reserve(jobs, in.ID, 0) {
			return conflict("余量不能低于队列预留量")
		}
		for _, j := range jobs {
			if jobUses(j)[in.ID] == 0 {
				continue
			}
			if activeStates[j.State] && j.UserID != actor.ID && in.Owner != "team" && !in.Shared {
				return conflict("其他成员仍在使用这卷耗材，暂不能设为私有")
			}
			if j.State != "queued" && activeStates[j.State] {
				if in.Mount == nil || in.Mount.PrinterID != j.Printer || old != nil && old.Mount != nil && *old.Mount != *in.Mount {
					return conflict("耗材正在使用，暂不能卸下或转移")
				}
			}
		}
		var pid, slot any
		if in.Mount != nil {
			printers, e := readPrinters(tx)
			if e != nil {
				return e
			}
			found := false
			for _, p := range printers {
				if p.ID == in.Mount.PrinterID && validSlot(p, in.Mount.Slot) {
					found = true
				}
			}
			if !found {
				return invalid("打印机或槽位无效")
			}
			for _, sp := range spools {
				if sp.ID != in.ID && sp.Mount != nil && *sp.Mount == *in.Mount {
					return conflict("这个槽位已登记其他耗材，请先卸下原卷")
				}
			}
			pid = in.Mount.PrinterID
			slot = in.Mount.Slot
		}
		if old == nil {
			in.ID = "s" + randomID(12)
		}
		_, err = tx.Exec(`INSERT INTO spools(id,label,brand,material,color_name,color,capacity,remaining,owner_id,shared,state,location,mount_printer,mount_slot) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET label=excluded.label,brand=excluded.brand,material=excluded.material,color_name=excluded.color_name,color=excluded.color,capacity=excluded.capacity,remaining=excluded.remaining,owner_id=excluded.owner_id,shared=excluded.shared,state=excluded.state,location=excluded.location,mount_printer=excluded.mount_printer,mount_slot=excluded.mount_slot`, in.ID, in.Label, trim(in.Brand, 40), trim(in.Material, 20), trim(in.ColorName, 30), in.Color, in.Capacity, in.Remaining, owner, in.Shared, in.State, trim(in.Location, 80), pid, slot)
		if err != nil {
			return err
		}
		next := int64(1)
		if old != nil {
			next = old.Revision + 1
		}
		_, err = tx.Exec("INSERT INTO record_versions(kind,record_id,revision) VALUES('spool',?,?) ON CONFLICT(kind,record_id) DO UPDATE SET revision=excluded.revision", in.ID, next)
		if err == nil && in.Owner != "team" && !in.Shared {
			_, err = tx.Exec(`UPDATE mail_outbox SET state='cancelled',body=X'' WHERE kind='notification' AND state IN ('queued','leased') AND user_id<>? AND event_key IN (SELECT event_key||':user:'||user_id FROM user_notifications WHERE entity_type='spool' AND entity_id=?)`, actor.ID, in.ID)
		}
		return err
	})
}
func (s *Service) Snapshot(u User) (map[string]any, error) {
	var e error
	u, e = currentUser(s.DB, u.ID)
	if e != nil {
		return nil, e
	}
	u.Language = accountLanguage(s.DB, u.ID)
	printers, err := s.printers()
	if err != nil {
		return nil, err
	}
	spools, err := readSpools(s.DB)
	if err != nil {
		return nil, err
	}
	jobs, err := readJobs(s.DB)
	if err != nil {
		return nil, err
	}
	visible := []Spool{}
	visibleIDs := map[string]bool{}
	occupiedSlots := []Mount{}
	for _, sp := range spools {
		if spoolVisible(sp, u) {
			visible = append(visible, sp)
			visibleIDs[sp.ID] = true
		} else if sp.Mount != nil {
			occupiedSlots = append(occupiedSlots, *sp.Mount)
		}
	}
	for i := range jobs {
		if jobs[i].PrintOptions != nil {
			for k := range jobs[i].PrintOptions.Mapping {
				if !visibleIDs[jobs[i].PrintOptions.Mapping[k].SpoolID] {
					jobs[i].PrintOptions.Mapping[k].SpoolID = ""
				}
			}
		}
		if !visibleIDs[jobs[i].SpoolID] {
			jobs[i].SpoolID = ""
		}
		if jobs[i].UserID != u.ID && !u.Admin {
			jobs[i].Note = ""
			jobs[i].Error = ""
		}
	}
	for i := range printers {
		p := &printers[i]
		t := s.Device.Status(p.ID)
		p.Connected = t.Connected && timeFresh(t.Updated)
		p.Updated = t.Updated.Unix()
		p.State = t.State
		if !p.Enabled || !p.Connected {
			p.State = "offline"
		}
		p.Percent = t.Percent
		p.Remaining = fmt.Sprintf("%d 分钟", t.Remaining)
		p.Layer = t.Layer
		p.Layers = t.Layers
		p.Nozzle = t.Nozzle
		p.Bed = t.Bed
		p.File = t.File
		p.Warning = t.Error
		p.RawState = t.RawState
		p.PrintError = t.PrintError
		if t.State == "failed" {
			p.Warning = "设备任务失败或已停止，请核对设备与实际用量"
		}
		p.AMS = t.AMS
		p.Equipment = t.Equipment
		redactEquipment(p, occupiedSlots)
		// Raw RFID/tray payloads are administrator telemetry; private inventory remains separate.
		if !u.Admin {
			p.AMS = nil
			p.Serial = ""
			p.Fingerprint = ""
		}
		for _, j := range jobs {
			if j.Printer == p.ID && (j.State == "printing" || j.State == "starting" || j.State == "paused" || j.State == "stopped") {
				p.User = j.User
				if p.Connected && (p.State == "idle" || j.State == "stopped") {
					p.State = "confirming"
				}
			}
		}
	}
	members := []Member{}
	rows, err := s.DB.Query("SELECT id,username,name,admin,owner,disabled FROM users ORDER BY id")
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var m Member
		if err = rows.Scan(&m.ID, &m.Username, &m.Name, &m.Admin, &m.Owner, &m.Disabled); err != nil {
			rows.Close()
			return nil, err
		}
		m.Role = "成员"
		if m.Admin {
			m.Role = "管理员"
		}
		if m.Owner {
			m.Role = "空间所有者"
		}
		m.Color = "blue"
		members = append(members, m)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	completed := 0
	for _, j := range jobs {
		if j.State == "done" {
			completed++
		}
	}
	summary, err := s.notificationSummary(u)
	if err != nil {
		return nil, err
	}
	return map[string]any{"printers": printers, "spools": visible, "occupiedSlots": occupiedSlots, "jobs": jobs, "members": members, "user": u, "completed": completed, "notifications": []any{}, "notificationSummary": summary, "serverTime": time.Now().UnixMilli(), "capabilities": map[string]any{"deviceMode": s.Config.DeviceMode, "printStart": s.Config.EnablePrintStart, "restorePending": s.pendingRestore.Load(), "accountManagement": true, "backups": u.Owner, "ownerRoles": true, "accountSecurity": true, "printerControl": true, "settlement": true, "projectPrint": true, "x2dSingleNozzlePrint": true, "notificationCenter": true, "stockAlerts": true, "teamProjects": true, "modelWorkbench": true, "slicing": s.slicingReady()}}, nil
}
