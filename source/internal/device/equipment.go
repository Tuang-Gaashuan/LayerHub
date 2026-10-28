package device

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"

	"layerhub.local/server/internal/app"
)

func object(v any) map[string]any { m, _ := v.(map[string]any); return m }
func list(v any) []any            { a, _ := v.([]any); return a }
func integer(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case float64:
		if !math.IsNaN(n) && !math.IsInf(n, 0) && n >= 0 && n <= 4294967295 && n == math.Trunc(n) {
			return int(n), true
		}
	case string:
		x, e := strconv.Atoi(n)
		return x, e == nil
	}
	return 0, false
}
func intPtr(v any, min, max int) *int {
	n, ok := integer(v)
	if !ok || n < min || n > max {
		return nil
	}
	return &n
}
func legacyModel(id string) bool {
	switch id {
	case "p1p", "p1s", "x1", "x1-carbon", "x1e", "a1", "a1-mini":
		return true
	}
	return false
}
func observeFields(l *listener, report map[string]any, now time.Time) {
	if l.fields == nil {
		l.fields = map[string]time.Time{}
	}
	for k := range report {
		if k != "sequence_id" && k != "command" && k != "result" {
			l.fields[k] = now
		}
	}
	for _, v := range list(report["lights_report"]) {
		r := object(v)
		if _, ok := r["mode"]; ok {
			l.fields["light."+text(r["node"])] = now
		}
	}
	observeModernFields(l, report, now)
}
func readEquipment(l *listener) app.Equipment {
	r := l.raw
	out := app.Equipment{Extruders: []app.ExtruderReading{}, Feeds: []app.FeedReading{}, Settings: []app.SettingReading{}, HMS: []string{}}
	out.Motion = readMotion(l)
	for _, v := range list(object(object(r["device"])["nozzle"])["info"]) {
		x := object(v)
		id, ok := integer(x["id"])
		diameter, e := strconv.ParseFloat(fmt.Sprint(x["diameter"]), 64)
		if ok && id >= 0 && id <= 1 && e == nil && diameter > 0 && diameter <= 2 && len(text(x["type"])) <= 16 {
			out.Nozzles = append(out.Nozzles, app.NozzleReading{ID: id, Diameter: strconv.FormatFloat(diameter, 'f', -1, 64), Type: text(x["type"]), Updated: l.fields[fmt.Sprintf("nozzle.spec.%d", id)]})
		}
	}
	out.Bed = temperatureRounded(r["bed_temper"], 150)
	out.BedExact = temperatureExact(r["bed_temper"], 150)
	out.BedTarget = intPtr(r["bed_target_temper"], 0, 150)
	if n, ok := integer(object(r["device"])["bed_temp"]); ok {
		out.BedExact = nil
		out.Bed = intPtr(n&65535, 0, 150)
		out.BedTarget = intPtr((n>>16)&65535, 0, 150)
	}
	ext := object(object(r["device"])["extruder"])
	if ext == nil {
		ext = object(r["extruder"])
	}
	state, _ := integer(ext["state"])
	count := state & 15
	if count > 0 && count <= 8 {
		out.Switching = (state>>12)&7 != 0 || (state>>19)&1 != 0
		for _, v := range list(ext["info"]) {
			x := object(v)
			id, ok := integer(x["id"])
			if !ok || id < 0 || id >= count {
				continue
			}
			e := app.ExtruderReading{ID: id, Name: fmt.Sprintf("喷嘴 · 设备编号 %d", id), CurrentSource: packedSource(x["snow"]), TargetSource: packedSource(x["star"])}
			if t, ok := integer(x["temp"]); ok {
				e.Temperature = intPtr(t&65535, 0, 500)
				e.Target = intPtr((t>>16)&65535, 0, 500)
			}
			out.Extruders = append(out.Extruders, e)
		}
	} else if legacyModel(l.config.ModelID) {
		out.Extruders = append(out.Extruders, app.ExtruderReading{ID: 0, Name: "喷嘴", Temperature: temperatureRounded(r["nozzle_temper"], 500), TemperatureExact: temperatureExact(r["nozzle_temper"], 500), Target: intPtr(r["nozzle_target_temper"], 0, 500), CurrentSource: legacySource(object(r["ams"])["tray_now"]), TargetSource: legacySource(object(r["ams"])["tray_tar"])})
	}
	for _, v := range list(object(r["ams"])["ams"]) {
		u := object(v)
		id, ok := integer(u["id"])
		if !ok || id < 0 || id > 254 {
			continue
		}
		kind := "AMS"
		if !legacyModel(l.config.ModelID) {
			kind = "AMS · 类型未上报"
		}
		if l.config.ModelID == "a1" || l.config.ModelID == "a1-mini" {
			kind = "AMS lite"
		}
		if info, err := strconv.ParseUint(text(u["info"]), 16, 32); err == nil {
			kind = map[uint64]string{1: "AMS", 2: "AMS lite", 3: "AMS 2 Pro", 4: "AMS HT"}[info&15]
			if kind == "" {
				kind = "AMS · 类型未知"
			}
		}
		unit := app.AMSUnitReading{ID: id, Kind: kind, HumidityIndex: intPtr(u["humidity"], 1, 5)}
		if info, e := strconv.ParseUint(text(u["info"]), 16, 32); e == nil {
			unit.Extruder = intPtr(int((info>>8)&15), 0, 1)
			unit.Updated = l.fields[fmt.Sprintf("ams.bind.%d", id)]
		}
		if value, err := strconv.ParseFloat(text(u["temp"]), 64); err == nil && !math.IsNaN(value) && !math.IsInf(value, 0) && value >= -20 && value <= 150 {
			unit.Temperature = &value
		}
		out.AMSUnits = append(out.AMSUnits, unit)
		for _, v := range list(u["tray"]) {
			t := object(v)
			slot, ok := integer(t["id"])
			if !ok || slot < 0 || slot > 3 {
				continue
			}
			f := feedReading(t, app.SourceSlot(id, slot), fmt.Sprintf("%s · 单元 %d / 槽 %d", kind, id, slot+1))
			f.Kind = kind
			if bits, err := strconv.ParseUint(text(object(r["ams"])["tray_exist_bits"]), 16, 64); err == nil && id < 16 {
				present := bits&(uint64(1)<<uint(id*4+slot)) != 0
				f.Present = &present
				if !present {
					f.Material = ""
					f.Color = ""
					f.Remaining = nil
					f.RemainingUnit = ""
				}
			}
			out.Feeds = append(out.Feeds, f)
		}
	}
	if t := object(r["vt_tray"]); t != nil {
		out.Feeds = append(out.Feeds, feedReading(t, "external", "外挂料"))
	}
	for _, v := range list(r["vir_slot"]) {
		t := object(v)
		id, ok := integer(t["id"])
		if !ok || id != 254 && id != 255 {
			continue
		}
		slot := "external"
		if id == 254 {
			slot = "external:1"
		}
		out.Feeds = append(out.Feeds, feedReading(t, slot, fmt.Sprintf("外挂料 · 设备编号 %d", id)))
	}
	for _, v := range list(r["hms"]) {
		h := object(v)
		attr, a := integer(h["attr"])
		code, b := integer(h["code"])
		if a && b && attr >= 0 && code >= 0 && len(out.HMS) < 32 {
			out.HMS = append(out.HMS, fmt.Sprintf("%04X-%04X-%04X-%04X", uint32(attr)>>16, uint32(attr)&65535, uint32(code)>>16, uint32(code)&65535))
			if attr == 0x05000500 && code == 0x00010007 {
				out.ControlIssue = "authorization_required"
			}
		}
	}
	out.Camera = cameraReading(l.config, r)
	for i := range out.Feeds {
		out.Feeds[i].Updated = l.fields["feed."+out.Feeds[i].Slot]
	}
	if l.config.ModelID == "x2d" {
		readX2DSettings(l, &out)
		return out
	}
	// Only the known legacy single-extruder protocol is writable in this release.
	if !legacyModel(l.config.ModelID) || count > 1 {
		return out
	}
	add := func(key, label, unit string, value *int, min, max int, field string) {
		if value != nil {
			out.Settings = append(out.Settings, app.SettingReading{Key: key, Label: label, Unit: unit, Value: *value, Min: min, Max: max, Updated: l.fields[field]})
		}
	}
	add("nozzle", "喷嘴目标温度", "°C", intPtr(r["nozzle_target_temper"], 0, 300), 0, 300, "nozzle_target_temper")
	bedMax := 100
	if l.config.ModelID == "a1-mini" {
		bedMax = 80
	}
	add("bed", "热床目标温度", "°C", intPtr(r["bed_target_temper"], 0, bedMax), 0, bedMax, "bed_target_temper")
	add("speed", "打印速度档位", "", intPtr(r["spd_lvl"], 1, 4), 1, 4, "spd_lvl")
	for _, v := range list(r["lights_report"]) {
		// A1 chamber_light is physically validated, but lights_report is static.
		// Keep A1 mini unvalidated and never expose extra placeholder lamps.
		if l.config.ModelID == "a1-mini" || l.config.ModelID == "a1" {
			continue
		}
		t := object(v)
		node := text(t["node"])
		mode := text(t["mode"])
		if (node == "chamber_light" || node == "chamber_light2") && (mode == "on" || mode == "off") {
			n := 0
			if mode == "on" {
				n = 1
			}
			add(node, "机舱灯", "", &n, 0, 1, "light."+node)
		}
	}
	if l.config.ModelID == "a1" {
		value := -1 // Unknown until this connection receives a positive command receipt.
		if l.lampRequest != nil {
			value = *l.lampRequest
		}
		out.Settings = append(out.Settings, app.SettingReading{Key: "chamber_light", Label: "照明灯", Value: value, Min: 0, Max: 1, Updated: l.stateUpdated, Feedback: "command_receipt"})
	}
	for i, f := range []struct{ key, field, label string }{{"part_fan", "cooling_fan_speed", "模型风扇"}, {"aux_fan", "big_fan1_speed", "辅助风扇"}, {"chamber_fan", "big_fan2_speed", "机舱风扇"}} {
		if f.key != "part_fan" {
			flag := "support_aux_fan"
			if f.key == "chamber_fan" {
				flag = "support_chamber_fan"
			}
			supported, reported := r[flag].(bool)
			if reported && !supported || !reported && l.config.ModelID == "p1p" {
				continue
			}
		}
		if f.key != "part_fan" && (l.config.ModelID == "a1" || l.config.ModelID == "a1-mini") {
			continue
		}
		var n *int
		field := f.field
		if gear, ok := integer(r["fan_gear"]); ok {
			n = intPtr((gear>>(8*i))&255, 0, 255)
			field = "fan_gear"
		} else if v := intPtr(r[f.field], 0, 15); v != nil {
			percent := int(math.Floor(float64(*v)/1.5)) * 10
			n = &percent
			add(f.key, f.label, "%", n, 0, 100, field)
			continue
		}
		if n != nil {
			percent := int(math.Round(float64(*n) * 100 / 255))
			n = &percent
		}
		add(f.key, f.label, "%", n, 0, 100, field)
	}
	return out
}
func temperatureExact(v any, max float64) *float64 {
	var n float64
	switch x := v.(type) {
	case float64:
		n = x
	case int:
		n = float64(x)
	default:
		return nil
	}
	if math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || n > max {
		return nil
	}
	n = math.Round(n*100) / 100
	return &n
}
func temperatureRounded(v any, max float64) *int {
	n := temperatureExact(v, max)
	if n == nil {
		return nil
	}
	r := int(math.Round(*n))
	return &r
}
func packedSource(v any) string {
	n, ok := integer(v)
	if !ok || n < 0 || n > 65535 || n == 65535 {
		return ""
	}
	unit := (n >> 8) & 255
	slot := n & 255
	if unit == 255 {
		return "external"
	}
	if unit == 254 {
		return "external:1"
	}
	if slot > 3 {
		return ""
	}
	return app.SourceSlot(unit, slot)
}
func legacySource(v any) string {
	n, ok := integer(v)
	if !ok || n < 0 || n == 255 {
		return ""
	}
	if n == 254 {
		return "external"
	}
	if n >= 128 && n <= 135 {
		return app.SourceSlot(n, 0)
	}
	if n < 64 {
		return app.SourceSlot(n/4, n%4)
	}
	return ""
}
func feedReading(t map[string]any, slot, label string) app.FeedReading {
	f := app.FeedReading{Slot: slot, Label: label, Material: text(t["tray_type"]), ProfileID: text(t["tray_info_idx"]), Color: text(t["tray_color"])}
	if g := intPtr(t["remain_g"], 0, 10000); g != nil {
		f.Remaining = g
		f.RemainingUnit = "g"
	} else if p := intPtr(t["remain"], 0, 100); p != nil {
		f.Remaining = p
		f.RemainingUnit = "%"
	}
	if strings.HasPrefix(slot, "external") {
		material, reported := t["tray_type"].(string)
		color, colorReported := t["tray_color"].(string)
		if reported && colorReported && material == "" && (color == "00000000" || color == "") {
			present := false
			f.Present = &present
			f.Remaining = nil
			f.RemainingUnit = ""
			f.Color = ""
		} else if reported && material != "" {
			present := true
			f.Present = &present
		}
	}
	return f
}

func cameraReading(p app.Printer, report map[string]any) app.CameraReading {
	cam := object(report["ipcam"])
	out := app.CameraReading{Resolution: text(cam["resolution"])}
	if raw := text(cam["rtsp_url"]); raw != "" && raw != "disable" {
		if parsed, err := url.Parse(raw); err == nil && parsed.Scheme == "rtsps" {
			out.Supported = true
			out.Protocol = "rtsps"
			return out
		}
	}
	if (p.ModelID == "p1s" || p.ModelID == "p1p" || p.ModelID == "a1" || p.ModelID == "a1-mini") && text(cam["ipcam_dev"]) == "1" {
		out.Supported = true
		out.Protocol = "jpeg"
	}
	return out
}
func findSetting(t app.Telemetry, key string) (app.SettingReading, bool) {
	for _, v := range t.Equipment.Settings {
		if v.Key == key {
			return v, true
		}
	}
	return app.SettingReading{}, false
}
func settingMatches(t app.Telemetry, in app.SettingInput, sent time.Time) bool {
	r, ok := findSetting(t, in.Key)
	if !ok || !r.Updated.After(sent) {
		return false
	}
	tolerance := 0
	if strings.HasPrefix(in.Key, "fan:") {
		tolerance = 1
	} else if strings.HasSuffix(in.Key, "fan") {
		tolerance = 10
	}
	return abs(r.Value-in.Value) <= tolerance
}
func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
func settingPayload(in app.SettingInput) (string, map[string]any) {
	p := map[string]any{"command": "gcode_line"}
	switch in.Key {
	case "nozzle:0", "nozzle:1":
		id, _ := strconv.Atoi(strings.TrimPrefix(in.Key, "nozzle:"))
		return "print", map[string]any{"command": "set_nozzle_temp", "extruder_index": id, "target_temp": in.Value}
	case "fan:1", "fan:2", "fan:3", "fan:10":
		id, _ := strconv.Atoi(strings.TrimPrefix(in.Key, "fan:"))
		return "print", map[string]any{"command": "set_fan", "fan_index": id, "speed": in.Value}
	case "nozzle":
		p["param"] = fmt.Sprintf("M104 S%d\n", in.Value)
	case "bed":
		p["param"] = fmt.Sprintf("M140 S%d\n", in.Value)
	case "speed":
		p["command"] = "print_speed"
		p["param"] = strconv.Itoa(in.Value)
	case "chamber_light", "chamber_light2":
		mode := "off"
		if in.Value == 1 {
			mode = "on"
		}
		return "system", map[string]any{"command": "ledctrl", "led_node": in.Key, "led_mode": mode, "led_on_time": 500, "led_off_time": 500, "loop_times": 0, "interval_time": 0}
	default:
		fan := map[string]int{"part_fan": 1, "aux_fan": 2, "chamber_fan": 3}[in.Key]
		if fan == 0 {
			return "", nil
		}
		p["param"] = fmt.Sprintf("M106 P%d S%d\n", fan, int(math.Round(float64(in.Value)*255/100)))
	}
	return "print", p
}
func (m *Manager) SetEquipment(ctx context.Context, p app.Printer, in app.SettingInput) error {
	r, ok := findSetting(m.Status(p.ID), in.Key)
	if !ok || in.Value < r.Min || in.Value > r.Max {
		return errors.New("setting unavailable or out of range")
	}
	if m.mode == "simulated" {
		m.mu.Lock()
		defer m.mu.Unlock()
		l := m.items[p.ID]
		if l == nil {
			return errors.New("device missing")
		}
		if applyX2DSimulationSetting(l, in) {
			return nil
		}
		field := map[string]string{"nozzle": "nozzle_target_temper", "bed": "bed_target_temper", "speed": "spd_lvl"}[in.Key]
		if field != "" {
			applyReport(l, map[string]any{field: in.Value})
		} else if strings.HasPrefix(in.Key, "chamber_light") {
			if p.ModelID == "a1" {
				n := in.Value
				l.lampRequest = &n
			}
			a := []any{}
			for _, v := range list(l.raw["lights_report"]) {
				x := object(v)
				if text(x["node"]) == in.Key {
					mode := "off"
					if in.Value == 1 {
						mode = "on"
					}
					x = map[string]any{"node": in.Key, "mode": mode}
				}
				a = append(a, x)
			}
			applyReport(l, map[string]any{"lights_report": a})
		} else {
			shift := map[string]int{"part_fan": 0, "aux_fan": 8, "chamber_fan": 16}[in.Key]
			gear, _ := integer(l.raw["fan_gear"])
			gear = gear & ^(255<<shift) | (int(math.Round(float64(in.Value)*255/100)) << shift)
			applyReport(l, map[string]any{"fan_gear": gear})
		}
		return nil
	}
	section, payload := settingPayload(in)
	if payload == nil {
		return errors.New("unsupported setting command")
	}
	if p.ModelID == "x2d" && in.Key == "bed" {
		m.mu.Lock()
		l := m.items[p.ID]
		modern := l != nil && mqttBedControl(l.raw)
		m.mu.Unlock()
		if modern {
			section, payload = "print", map[string]any{"command": "set_bed_temp", "temp": in.Value}
		}
	}
	if p.ModelID == "a1" && in.Key == "chamber_light" {
		return m.commandSectionFeedback(ctx, p, section, payload, func(app.Telemetry, time.Time) bool { return false }, true)
	}
	return m.commandSection(ctx, p, section, payload, func(t app.Telemetry, sent time.Time) bool { return settingMatches(t, in, sent) })
}

func simulationReport(p app.Printer) map[string]any {
	r := map[string]any{"gcode_state": "IDLE", "nozzle_temper": 25, "nozzle_target_temper": 0, "bed_temper": 25, "bed_target_temper": 0, "spd_lvl": 2, "fan_gear": 0, "lights_report": []any{map[string]any{"node": "chamber_light", "mode": "on"}}, "vt_tray": map[string]any{"tray_type": "PLA", "tray_color": "D4C8B8FF"}}
	if p.Configuration == "ams" {
		r["ams"] = map[string]any{"tray_now": "255", "ams": []any{map[string]any{"id": "0", "tray": []any{map[string]any{"id": "0", "tray_type": "PLA", "tray_color": "EEEAE2FF", "remain": 82}, map[string]any{"id": "1", "tray_type": "PLA", "tray_color": "3159EDFF", "remain": 68}}}}}
	}
	if p.ModelID == "x2d" {
		r["device"] = map[string]any{"bed_temp": 25, "extruder": map[string]any{"state": 2, "info": []any{map[string]any{"id": 0, "temp": 25}, map[string]any{"id": 1, "temp": 25}}}, "airduct": map[string]any{"modeCur": 0, "modeList": []any{map[string]any{"modeId": 0, "ctrl": []any{16, 32, 48, 160}}}, "parts": []any{map[string]any{"id": 16, "state": 0, "range": 100 << 16}, map[string]any{"id": 32, "state": 0, "range": 100 << 16}, map[string]any{"id": 48, "state": 0, "range": 100 << 16}, map[string]any{"id": 160, "state": 0, "range": 100 << 16}}}}
	}
	if p.ModelID == "h2d" || p.ModelID == "h2d-pro" {
		r["device"] = map[string]any{"extruder": map[string]any{"state": 2, "info": []any{map[string]any{"id": 0, "temp": 210 | (220 << 16), "snow": 0, "star": 0}, map[string]any{"id": 1, "temp": 25, "snow": 254 << 8, "star": 254 << 8}}}}
		r["vir_slot"] = []any{map[string]any{"id": 254, "tray_type": "TPU", "tray_color": "F1AE66FF"}}
	}
	return r
}
