package app

import (
	"database/sql"
	"reflect"
	"strings"
)

// Legacy color-confirmation fields are accepted for older clients but ignored.
// Color is inventory metadata, not a mount validation condition.
type SpoolReplacement struct {
	Mount                  *Mount `json:"mount,omitempty"`
	ConfirmColorDifference bool   `json:"confirmColorDifference,omitempty"`
	ReportedColor          string `json:"reportedColor,omitempty"`
	ExistingID             string `json:"existingId,omitempty"`
	ExistingRevision       int64  `json:"existingRevision,omitempty"`
	OldID                  string `json:"oldId"`
	Revision               int64  `json:"revision"`
	ProfileID              string `json:"profileId"`
	Spool                  Spool  `json:"spool"`
}

// A physical replacement changes identity. Never turn the previous spool into
// the new material or carry its remaining weight into a new record.
func (s *Service) ReplaceSpool(actor User, in SpoolReplacement, ops ...OperationKey) (map[string]any, error) {
	var newID string
	err := s.operationTransaction(actor, ops, func(tx *sql.Tx) error {
		fresh, err := currentUser(tx, actor.ID)
		if err != nil {
			return err
		}
		spools, err := readSpools(tx)
		if err != nil {
			return err
		}
		var old *Spool
		for i := range spools {
			if spools[i].ID == in.OldID {
				old = &spools[i]
			}
		}
		emptySlot := in.OldID == ""
		if emptySlot {
			if in.Mount == nil || in.Revision != 0 {
				return invalid("请选择目标打印机和装载位置")
			}
			m := *in.Mount
			m.Slot = normalizedSlot(m.Slot)
			for _, sp := range spools {
				if sp.Mount != nil && *sp.Mount == m {
					return conflict("这个槽位已登记其他耗材，请重新读取后选择更换料卷")
				}
			}
			old = &Spool{Mount: &m}
		} else if old == nil {
			return missing("耗材不存在")
		}
		if !emptySlot && !spoolEditable(*old, fresh) {
			return forbidden("只能修改本人耗材，公共耗材由管理员维护")
		}
		if !emptySlot && (in.Revision < 1 || old.Revision != in.Revision || old.Mount == nil || in.Mount != nil && *in.Mount != *old.Mount) {
			return conflict("耗材记录已变化，请重新读取后再更新")
		}
		if in.ExistingID != "" {
			if in.ExistingID == old.ID || !reflect.DeepEqual(in.Spool, Spool{}) {
				return invalid("请选择独立的库存料卷，不要同时填写新卷资料")
			}
			var target *Spool
			for i := range spools {
				if spools[i].ID == in.ExistingID {
					target = &spools[i]
				}
			}
			if target == nil || !spoolEditable(*target, fresh) {
				return forbidden("无权挂载所选库存料卷")
			}
			if in.ExistingRevision < 1 || target.Revision != in.ExistingRevision || target.Mount != nil || target.State != "available" || target.Remaining <= 0 {
				return conflict("库存料卷已变化、已挂载或不可用，请重新选择")
			}
			in.Spool = *target
			m := *old.Mount
			in.Spool.Mount = &m
		} else if in.ExistingRevision != 0 {
			return invalid("请选择有效的库存料卷")
		}
		if in.ExistingID == "" && (in.Spool.ID != "" || in.Spool.Revision != 0) || in.Spool.Mount == nil || *in.Spool.Mount != *old.Mount {
			return invalid("新料卷必须使用独立记录并挂载到原槽位")
		}
		jobs, err := readJobs(tx)
		if err != nil {
			return err
		}
		for _, j := range jobs {
			if activeStates[j.State] && (jobUses(j)[old.ID] > 0 || in.ExistingID != "" && jobUses(j)[in.ExistingID] > 0) {
				return conflict("原料卷仍有任务预留或待结算，请先处理任务后换卷")
			}
		}
		printers, err := readPrinters(tx)
		if err != nil {
			return err
		}
		enabled := false
		for _, p := range printers {
			if p.ID == old.Mount.PrinterID {
				enabled = p.Enabled
			}
		}
		t := s.Device.Status(old.Mount.PrinterID)
		if !enabled || !t.Connected || !motionFresh(t.Updated) || s.pendingRestore.Load() {
			return conflict("打印机未提供新鲜的已装料信息，未更新耗材")
		}
		found := false
		for _, f := range t.Equipment.Feeds {
			if f.Slot != old.Mount.Slot {
				continue
			}
			if !motionFresh(f.Updated) || f.Present == nil || !*f.Present || f.Material == "" || !strings.EqualFold(in.Spool.Material, f.Material) || in.ProfileID != f.ProfileID {
				return conflict("设备耗材已变化，请重新读取后登记新卷")
			}
			found = true
		}
		if !found {
			return conflict("打印机未提供新鲜的已装料信息，未更新耗材")
		}
		// Both changes commit together, including the normal inventory validation.
		if !emptySlot {
			if _, err = tx.Exec("UPDATE spools SET mount_printer=NULL,mount_slot=NULL WHERE id=?", old.ID); err != nil {
				return err
			}
			if err = bumpSpoolRevision(tx, old.ID); err != nil {
				return err
			}
			if err = recordedSpool(tx, fresh.ID, "unmount", old, old.ID, 0); err != nil {
				return err
			}
		}
		if err = s.saveSpoolTx(tx, fresh, in.Spool); err != nil {
			return err
		}
		return tx.QueryRow("SELECT id FROM spools WHERE mount_printer=? AND mount_slot=?", old.Mount.PrinterID, old.Mount.Slot).Scan(&newID)
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "oldId": in.OldID, "newId": newID}, nil
}
