package device

import (
	"context"
	"encoding/json"
	"errors"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"layerhub.local/server/internal/app"
	"testing"
	"time"
)

type absentAckToken struct{ motionToken }

func (absentAckToken) WaitTimeout(time.Duration) bool {
	panic("must not wait on transport ACK before checking hardware")
}
func TestCommandObservesHardwareWithoutTransportAckAndNeverResends(t *testing.T) {
	for _, mode := range []string{"hardware", "echo-only", "rejected"} {
		t.Run(mode, func(t *testing.T) {
			p := app.Printer{ID: 1, ModelID: "a1", Enabled: true, Serial: "TEST"}
			l := &listener{config: p, raw: map[string]any{}, connected: true}
			m, _ := New("live")
			m.items[1] = l
			reportJSON(t, l, `{"gcode_state":"IDLE","nozzle_target_temper":0}`)
			count := 0
			l.client = motionClient{publish: func(topic string, qos byte, retain bool, v interface{}) mqtt.Token {
				if _, ok := v.(string); ok {
					return absentAckToken{}
				}
				var b map[string]map[string]any
				json.Unmarshal(v.([]byte), &b)
				r := b["print"]
				if r == nil {
					return absentAckToken{}
				}
				count++
				if qos != 0 || retain {
					t.Fatal("physical command can replay")
				}
				seq := r["sequence_id"].(string)
				m.mu.Lock()
				defer m.mu.Unlock()
				switch mode {
				case "hardware":
					time.Sleep(time.Millisecond) // A real report arrives after the send timestamp.
					applyReport(l, map[string]any{"nozzle_target_temper": 200})
				case "echo-only":
					applyReport(l, map[string]any{"command": "gcode_line", "sequence_id": seq, "result": "success", "nozzle_target_temper": 200})
				case "rejected":
					l.replies[seq] = map[string]any{"result": "success", "errno": 1}
				}
				return absentAckToken{}
			}}
			ctx, cancel := context.WithTimeout(context.Background(), 450*time.Millisecond)
			defer cancel()
			in := app.SettingInput{Key: "nozzle", Value: 200}
			err := m.commandSection(ctx, p, "print", map[string]any{"command": "gcode_line", "param": "M104 S200"}, func(tel app.Telemetry, sent time.Time) bool { return settingMatches(tel, in, sent) })
			if count != 1 {
				t.Fatal("resent", count)
			}
			if mode == "hardware" && err != nil {
				t.Fatalf("actual state ignored %v; %+v fields=%+v", err, m.Status(1).Equipment.Settings, l.fields)
			}
			if mode != "hardware" && err == nil {
				t.Fatal("echo/rejection accepted")
			}
			if mode == "rejected" && !errors.Is(err, app.ErrDeviceRejected) {
				t.Fatal(err)
			}
		})
	}
}
func TestA1DecimalTemperaturesAndReceiptOnlyLight(t *testing.T) {
	l := &listener{config: app.Printer{ModelID: "a1"}, raw: map[string]any{}}
	reportJSON(t, l, `{"nozzle_temper":78.03125,"bed_temper":52.59375,"nozzle_target_temper":0,"bed_target_temper":0,"lights_report":[{"node":"chamber_light","mode":"off"}]}`)
	eq := readEquipment(l)
	if eq.BedExact == nil || *eq.BedExact != 52.59 || eq.Extruders[0].TemperatureExact == nil || *eq.Extruders[0].TemperatureExact != 78.03 {
		t.Fatal(eq)
	}
	for _, v := range eq.Settings {
		if v.Key == "chamber_light" && (v.Value != -1 || v.Feedback != "command_receipt") {
			t.Fatal("placeholder claimed as physical light state")
		}
	}
	if _, ok := integer(1.5); ok {
		t.Fatal("protocol integer validation weakened")
	}
}
