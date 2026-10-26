package device

import (
	"fmt"
	"layerhub.local/server/internal/app"
	"strconv"
	"time"
)

func observeModernFields(l *listener, r map[string]any, now time.Time) {
	device := object(r["device"])
	for _, v := range list(object(device["nozzle"])["info"]) {
		x := object(v)
		if id, ok := integer(x["id"]); ok && x["diameter"] != nil && x["type"] != nil {
			l.fields[fmt.Sprintf("nozzle.spec.%d", id)] = now
		}
	}
	ext := object(device["extruder"])
	if ext == nil {
		ext = object(r["extruder"])
	}
	for _, v := range list(ext["info"]) {
		x := object(v)
		if id, ok := integer(x["id"]); ok {
			if _, present := x["temp"]; present {
				l.fields[fmt.Sprintf("nozzle.%d", id)] = now
			}
		}
	}
	if _, ok := device["bed_temp"]; ok {
		l.fields["bed_temp"] = now
	}
	air := object(device["airduct"])
	for _, key := range []string{"modeCur", "modeList"} {
		if _, ok := air[key]; ok {
			l.fields["airduct."+key] = now
		}
	}
	for _, v := range list(air["parts"]) {
		x := object(v)
		if id, ok := integer(x["id"]); ok {
			if _, present := x["state"]; present {
				l.fields[fmt.Sprintf("airduct.part.%d", id)] = now
			}
		}
	}
	ams := object(r["ams"])
	for _, v := range list(ams["ams"]) {
		u := object(v)
		id, ok := integer(u["id"])
		if !ok {
			continue
		}
		if _, present := u["info"]; present {
			l.fields[fmt.Sprintf("ams.bind.%d", id)] = now
		}
		for _, v := range list(u["tray"]) {
			t := object(v)
			slot, ok := integer(t["id"])
			if ok && len(t) > 1 {
				l.fields["feed."+app.SourceSlot(id, slot)] = now
			}
		}
	}
	if object(r["vt_tray"]) != nil {
		l.fields["feed.external"] = now
	}
	for _, v := range list(r["vir_slot"]) {
		x := object(v)
		id, ok := integer(x["id"])
		if ok && len(x) > 1 {
			if id == 254 {
				l.fields["feed.external:1"] = now
			}
			if id == 255 {
				l.fields["feed.external"] = now
			}
		}
	}
}
func mqttBedControl(r map[string]any) bool {
	bits, err := strconv.ParseUint(text(r["fun"]), 16, 64)
	return err == nil && (bits>>39)&1 == 1
}
func readX2DSettings(l *listener, out *app.Equipment) {
	add := func(key, label, unit string, value *int, min, max int, field string) {
		if value != nil {
			out.Settings = append(out.Settings, app.SettingReading{Key: key, Label: label, Unit: unit, Value: *value, Min: min, Max: max, Updated: l.fields[field]})
		}
	}
	if !out.Switching {
		for _, x := range out.Extruders {
			if x.ID == 0 || x.ID == 1 {
				add(fmt.Sprintf("nozzle:%d", x.ID), fmt.Sprintf("喷嘴 %d 目标温度", x.ID), "°C", x.Target, 0, 300, fmt.Sprintf("nozzle.%d", x.ID))
			}
		}
	}
	field := "bed_target_temper"
	if _, ok := object(l.raw["device"])["bed_temp"]; ok {
		field = "bed_temp"
	}
	add("bed", "热床目标温度", "°C", out.BedTarget, 0, 100, field)
	add("speed", "打印速度档位", "", intPtr(l.raw["spd_lvl"], 1, 4), 1, 4, "spd_lvl")
	for _, v := range list(l.raw["lights_report"]) {
		x := object(v)
		node, mode := text(x["node"]), text(x["mode"])
		if (node == "chamber_light" || node == "chamber_light2") && (mode == "on" || mode == "off") {
			n := 0
			if mode == "on" {
				n = 1
			}
			add(node, "机舱灯", "", &n, 0, 1, "light."+node)
		}
	}
	air := object(object(l.raw["device"])["airduct"])
	current, known := integer(air["modeCur"])
	if !known || time.Since(l.fields["airduct.modeCur"]) > 90*time.Second || time.Since(l.fields["airduct.modeList"]) > 90*time.Second {
		return
	}
	allowed := map[int]bool{}
	for _, v := range list(air["modeList"]) {
		mode := object(v)
		id, ok := integer(mode["modeId"])
		if !ok || id != current {
			continue
		}
		for _, v := range list(mode["ctrl"]) {
			if n, ok := integer(v); ok {
				allowed[n>>4] = true
			}
		}
		for _, v := range list(mode["off"]) {
			if n, ok := integer(v); ok {
				allowed[n>>4] = false
			}
		}
	}
	labels := map[int]string{1: "模型风扇", 2: "辅助风扇 1", 3: "排风风扇", 10: "辅助风扇 2"}
	for _, v := range list(air["parts"]) {
		x := object(v)
		raw, ok := integer(x["id"])
		if !ok || raw&15 != 0 {
			continue
		}
		id := (raw >> 4) & 255
		label := labels[id]
		if label == "" || !allowed[id] {
			continue
		}
		state, sok := integer(x["state"])
		r, rok := integer(x["range"])
		if !sok || !rok {
			continue
		}
		min, max := r&65535, (r>>16)&65535
		if min < 0 || max > 100 || min > max {
			continue
		}
		n := intPtr(state&255, min, max)
		add(fmt.Sprintf("fan:%d", id), label, "%", n, min, max, fmt.Sprintf("airduct.part.%d", raw))
	}
}
func applyX2DSimulationSetting(l *listener, in app.SettingInput) bool {
	if l.config.ModelID != "x2d" {
		return false
	}
	if in.Key == "nozzle:0" || in.Key == "nozzle:1" {
		id, _ := strconv.Atoi(in.Key[len("nozzle:"):])
		for _, x := range list(object(object(l.raw["device"])["extruder"])["info"]) {
			t := object(x)
			n, _ := integer(t["id"])
			if n == id {
				temp, _ := integer(t["temp"])
				applyReport(l, map[string]any{"device": map[string]any{"extruder": map[string]any{"info": []any{map[string]any{"id": id, "temp": temp&65535 | in.Value<<16}}}}})
				return true
			}
		}
	}
	if len(in.Key) > 4 && in.Key[:4] == "fan:" {
		id, _ := strconv.Atoi(in.Key[4:])
		applyReport(l, map[string]any{"device": map[string]any{"airduct": map[string]any{"parts": []any{map[string]any{"id": id << 4, "state": in.Value}}}}})
		return true
	}
	if in.Key == "bed" {
		n, _ := integer(object(l.raw["device"])["bed_temp"])
		applyReport(l, map[string]any{"device": map[string]any{"bed_temp": n&65535 | in.Value<<16}})
		return true
	}
	return false
}
