package app

import (
	"fmt"
	"strings"
)

// Initial X2D support uses the physical route encoded by the slicer. It never
// rewrites a nozzle assignment or moves an AMS binding to make a file fit.
func x2dPrintSlice(j Job, o PrintOptions) (SliceInfo, SlicePlate, error) {
	info, err := inspectSlice(j.Path)
	if err != nil {
		return info, SlicePlate{}, err
	}
	for _, plate := range info.Plates {
		if plate.ID != o.Plate {
			continue
		}
		if plate.MultiNozzle || len(plate.Filaments) != 1 || plate.Nozzle == nil {
			return info, plate, conflict("X2D 当前只支持带明确物理喷嘴映射的单喷嘴、单耗材盘，请重新切片")
		}
		n := plate.Nozzle
		if n.Physical < 0 || n.Physical > 1 || n.Volume != "Standard" || n.Logical >= len(info.NozzleDiameters) || info.NozzleDiameters[n.Logical] != n.Diameter {
			return info, plate, conflict("X2D 切片喷嘴规格或流量类型尚未适配")
		}
		if !o.UseAMS || len(o.Mapping) != 1 || o.Mapping[0].AMSID != 0 || o.Mapping[0].SlotID < 0 || o.Mapping[0].SlotID > 3 {
			return info, plate, conflict("X2D 当前仅开放第一组 AMS 四槽供料；外挂与更多 AMS 待验证")
		}
		if o.FlowCalibration || o.Timelapse {
			return info, plate, conflict("X2D 首轮打印验证暂不开放流量校准与延时摄影，请关闭后保存准备")
		}
		if plate.Filaments[0].ProfileID == "" {
			return info, plate, conflict("X2D 切片缺少耗材预设标识，请重新导出切片文件")
		}
		if info.BedType != "Textured PEI Plate" {
			return info, plate, conflict("X2D 当前仅验证纹理 PEI 板，请核对打印板与切片")
		}
		return info, plate, nil
	}
	return info, SlicePlate{}, invalid("请选择有效打印盘")
}

func x2dPrintReady(j Job, t Telemetry) error {
	if j.PrintOptions == nil {
		return conflict("请先保存 X2D 打印盘、颜色与供料准备")
	}
	_, plate, err := x2dPrintSlice(j, *j.PrintOptions)
	if err != nil {
		return err
	}
	if t.PrintError != 0 || len(t.Equipment.HMS) > 0 || t.Equipment.ControlIssue != "" || t.Equipment.Switching {
		return conflict("X2D 有告警、授权限制或正在切换供料，请先核对设备")
	}
	physical := plate.Nozzle.Physical
	nozzleOK := false
	for _, n := range t.Equipment.Nozzles {
		if n.ID == physical && timeFresh(n.Updated) && n.Diameter == plate.Nozzle.Diameter && strings.HasPrefix(n.Type, "HS") {
			nozzleOK = true
		}
	}
	if !nozzleOK {
		return conflict(fmt.Sprintf("等待物理喷嘴 %d 的新规格；需与切片 %s mm / Standard 一致", physical, plate.Nozzle.Diameter))
	}
	bound := false
	for _, unit := range t.Equipment.AMSUnits {
		if unit.ID == 0 && unit.Extruder != nil && *unit.Extruder == physical && timeFresh(unit.Updated) {
			bound = true
		}
	}
	if !bound {
		return conflict(fmt.Sprintf("AMS 单元 0 未确认绑定物理喷嘴 %d；请核对供料连接与切片喷嘴", physical))
	}
	wanted := fmt.Sprint(j.PrintOptions.Mapping[0].SlotID + 1)
	for _, feed := range t.Equipment.Feeds {
		if feed.Slot != wanted {
			continue
		}
		if !timeFresh(feed.Updated) || feed.Present == nil || !*feed.Present || !strings.EqualFold(feed.Material, plate.Filaments[0].Material) || feed.ProfileID != plate.Filaments[0].ProfileID {
			return conflict("所选 AMS 槽位为空、状态过期或实际耗材预设与切片不同，请核对料卷")
		}
		return nil
	}
	return conflict("尚未收到所选 AMS 槽位的实时耗材信息")
}
