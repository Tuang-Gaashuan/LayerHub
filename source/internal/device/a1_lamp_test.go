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

func TestA1LampReceiptIsSeparateFromStaticHardwareReport(t *testing.T) {
	for _, mode := range []string{"accepted", "no-reply", "wrong-command", "rejected"} {
		t.Run(mode, func(t *testing.T) {
			p := app.Printer{ID: 1, ModelID: "a1", Enabled: true, Serial: "TEST"}
			l := &listener{config: p, raw: map[string]any{}, connected: true}
			m, _ := New("live")
			m.items[1] = l
			reportJSON(t, l, `{"gcode_state":"FINISH","lights_report":[{"node":"chamber_light","mode":"on"}]}`)
			count := 0
			l.client = motionClient{publish: func(_ string, qos byte, retain bool, v interface{}) mqtt.Token {
				if _, ok := v.(string); ok {
					return absentAckToken{}
				}
				var b map[string]map[string]any
				json.Unmarshal(v.([]byte), &b)
				r := b["system"]
				if r == nil {
					t.Fatal("unexpected section")
				}
				count++
				if qos != 0 || retain {
					t.Fatal("replay risk")
				}
				m.mu.Lock()
				defer m.mu.Unlock()
				seq := r["sequence_id"].(string)
				if mode != "no-reply" {
					ack := map[string]any{"command": "ledctrl", "result": "success"}
					if mode == "wrong-command" {
						ack["command"] = "different"
					}
					if mode == "rejected" {
						ack["errno"] = 1
					}
					l.replies[seq] = ack
				}
				return absentAckToken{}
			}}
			ctx, cancel := context.WithTimeout(context.Background(), 350*time.Millisecond)
			defer cancel()
			e := m.SetEquipment(ctx, p, app.SettingInput{Key: "chamber_light", Value: 0})
			if count != 1 {
				t.Fatal("not sent exactly once", count)
			}
			reading, _ := findSetting(m.Status(1), "chamber_light")
			if mode == "accepted" {
				if !errors.Is(e, app.ErrDeviceAccepted) || reading.Value != 0 || reading.Feedback != "command_receipt" {
					t.Fatal(e, reading)
				}
			} else {
				if e == nil || errors.Is(e, app.ErrDeviceAccepted) || reading.Value != -1 {
					t.Fatal("false confirmation", e, reading)
				}
			}
			if text(object(list(l.raw["lights_report"])[0])["mode"]) != "on" {
				t.Fatal("raw hardware data overwritten")
			}
		})
	}
}
func TestA1MiniStillDoesNotExposeUnvalidatedLampOrExtraFans(t *testing.T) {
	for _, model := range []string{"a1", "a1-mini"} {
		l := &listener{config: app.Printer{ModelID: model}, raw: map[string]any{}}
		reportJSON(t, l, `{"gcode_state":"IDLE","fan_gear":0,"lights_report":[{"node":"chamber_light","mode":"on"},{"node":"chamber_light2","mode":"on"}]}`)
		for _, s := range readEquipment(l).Settings {
			if s.Key == "chamber_light2" || s.Key == "aux_fan" || s.Key == "chamber_fan" || model == "a1-mini" && s.Key == "chamber_light" {
				t.Fatal(model, s)
			}
		}
	}
}
