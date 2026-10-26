package device

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"layerhub.local/server/internal/app"
	"path/filepath"
	"strings"
	"time"
)

// SetSimulationDuration changes only the virtual clock, preserving its progress.
// It is unavailable for real devices and is not exposed as an HTTP API.
func (m *Manager) SetSimulationDuration(d time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.mode != "simulated" || d < time.Second || d > 24*time.Hour {
		return errors.New("simulation duration is invalid")
	}
	for _, l := range m.items {
		if l.started.IsZero() {
			continue
		}
		anchor := time.Now()
		if !l.pausedAt.IsZero() {
			anchor = l.pausedAt
		}
		elapsed := anchor.Sub(l.started)
		fraction := float64(elapsed) / float64(m.simulationDuration)
		l.started = anchor.Add(-time.Duration(fraction * float64(d)))
		l.raw["mc_remaining_time"] = max(1, int((float64(d)*(1-fraction)+float64(time.Minute)-1)/float64(time.Minute)))
	}
	m.simulationDuration = d
	return nil
}
func printPayload(r app.PrintRequest) map[string]any {
	p := map[string]any{"command": "gcode_file", "param": "/" + r.Remote}
	if r.Options == nil {
		return p
	}
	o := r.Options
	p = map[string]any{"command": "project_file", "param": fmt.Sprintf("Metadata/plate_%d.gcode", o.Plate), "url": "ftp:///" + r.Remote, "subtask_name": r.TaskName, "project_id": "0", "profile_id": "0", "task_id": "0", "subtask_id": "0", "file": "", "md5": "", "bed_type": "auto", "bed_levelling": o.BedLeveling, "flow_cali": o.FlowCalibration, "vibration_cali": false, "layer_inspect": false, "timelapse": o.Timelapse, "use_ams": o.UseAMS}
	if r.BedType != "" {
		p["bed_type"] = r.BedType
	}
	max := 0
	for _, v := range o.Mapping {
		if v.Filament > max {
			max = v.Filament
		}
	}
	legacy := make([]int, max+1)
	modern := make([]map[string]int, max+1)
	for i := range legacy {
		legacy[i] = -1
		modern[i] = map[string]int{"ams_id": 255, "slot_id": 255}
	}
	for _, v := range o.Mapping {
		if o.UseAMS {
			legacy[v.Filament] = v.AMSID*4 + v.SlotID
			modern[v.Filament] = map[string]int{"ams_id": v.AMSID, "slot_id": v.SlotID}
		}
	}
	p["ams_mapping"] = legacy
	p["ams_mapping2"] = modern
	return p
}
func matchesPrint(t app.Telemetry, r app.PrintRequest) bool {
	for _, name := range []string{t.File, t.GcodeFile} {
		base := filepath.Base(strings.ReplaceAll(name, "\\", "/"))
		if base != "" && (strings.EqualFold(base, r.Remote) || strings.EqualFold(base, r.TaskName)) {
			return true
		}
	}
	return false
}
func (m *Manager) StartPrint(ctx context.Context, p app.Printer, r app.PrintRequest) error {
	if m.mode == "simulated" {
		m.mu.Lock()
		defer m.mu.Unlock()
		l := m.items[p.ID]
		if l == nil {
			return errors.New("device not found")
		}
		tray := "254"
		if r.Options != nil && r.Options.UseAMS && len(r.Options.Mapping) > 0 {
			target := r.Options.Mapping[0]
			tray = fmt.Sprint(target.AMSID*4 + target.SlotID)
		}
		applyReport(l, map[string]any{"gcode_state": "RUNNING", "subtask_name": r.TaskName, "gcode_file": r.Remote, "mc_percent": 0, "mc_remaining_time": 1, "nozzle_temper": 210, "nozzle_target_temper": 210, "bed_temper": 60, "bed_target_temper": 60, "ams": map[string]any{"tray_now": tray, "tray_tar": tray}, "layer_num": 1, "total_layer_num": 148})
		l.started = time.Now()
		l.pausedAt = time.Time{}
		return nil
	}
	return m.command(ctx, p, printPayload(r), func(t app.Telemetry, sent time.Time) bool {
		return t.StateUpdated.After(sent) && t.TaskUpdated.After(sent) && t.State == "printing" && matchesPrint(t, r)
	})
}
func (m *Manager) Control(ctx context.Context, p app.Printer, action string) error {
	if action != "pause" && action != "resume" && action != "stop" {
		return errors.New("unsupported control")
	}
	if m.mode == "simulated" {
		m.mu.Lock()
		defer m.mu.Unlock()
		l := m.items[p.ID]
		if l == nil {
			return errors.New("device not found")
		}
		state := "PAUSE"
		if action == "pause" {
			l.pausedAt = time.Now()
		}
		if action == "resume" {
			state = "RUNNING"
			if !l.pausedAt.IsZero() {
				l.started = l.started.Add(time.Since(l.pausedAt))
			}
			l.pausedAt = time.Time{}
		}
		if action == "stop" {
			state = "IDLE"
			l.started = time.Time{}
			l.pausedAt = time.Time{}
		}
		applyReport(l, map[string]any{"gcode_state": state})
		return nil
	}
	return m.command(ctx, p, map[string]any{"command": action, "param": ""}, func(t app.Telemetry, sent time.Time) bool {
		if !t.StateUpdated.After(sent) {
			return false
		}
		switch action {
		case "pause":
			return t.State == "pause"
		case "resume":
			return t.State == "printing"
		default:
			return t.RawState == "IDLE" || t.RawState == "FINISH" || t.RawState == "FAILED"
		}
	})
}
func (m *Manager) command(ctx context.Context, p app.Printer, payload map[string]any, done func(app.Telemetry, time.Time) bool) error {
	return m.commandSection(ctx, p, "print", payload, done)
}
func (m *Manager) commandSection(ctx context.Context, p app.Printer, section string, payload map[string]any, done func(app.Telemetry, time.Time) bool) error {
	if m.mode != "live" {
		return errors.New("device connection disabled")
	}
	seq := fmt.Sprint(100000 + m.sequence.Add(1)%1000000000)
	m.mu.Lock()
	l := m.items[p.ID]
	if l == nil || l.client == nil || !l.client.IsConnectionOpen() {
		m.mu.Unlock()
		return errors.New("MQTT not connected")
	}
	client := l.client
	if l.replies == nil {
		l.replies = map[string]map[string]any{}
	}
	l.replies[seq] = nil
	m.mu.Unlock()
	defer func() { m.mu.Lock(); delete(l.replies, seq); m.mu.Unlock() }()
	payload["sequence_id"] = seq
	body, _ := json.Marshal(map[string]any{section: payload})
	sent := time.Now()
	token := client.Publish("device/"+p.Serial+"/request", 1, false, body)
	if !token.WaitTimeout(10*time.Second) || token.Error() != nil {
		return errors.New("command publish unconfirmed")
	}
	// A read-only snapshot also confirms unchanged requested values. Never resend
	// a physical command just because its acknowledgement was lost.
	switch text(payload["command"]) {
	case "gcode_line", "set_nozzle_temp", "set_bed_temp", "set_fan", "ledctrl", "print_speed":
		client.Publish("device/"+p.Serial+"/request", 0, false, `{"pushing":{"sequence_id":"0","command":"pushall","version":1,"push_target":1}}`)
	}
	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return errors.New("device execution unconfirmed; inspect device before retrying")
		case <-ticker.C:
			m.mu.Lock()
			ack := l.replies[seq]
			result := strings.ToLower(text(ack["result"]))
			bad := result == "fail" || result == "failed" || result == "error" || number(ack["reason_code"]) > 0
			m.mu.Unlock()
			if bad {
				return fmt.Errorf("%w: reason_code=%d", app.ErrDeviceRejected, number(ack["reason_code"]))
			}
			t := m.Status(p.ID)
			if t.Connected && done(t, sent) {
				return nil
			}
		}
	}
}
