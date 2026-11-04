package device

import (
	"context"
	"encoding/json"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"layerhub.local/server/internal/app"
	"strings"
	"testing"
	"time"
)

const filamentFrame = `{"gcode_state":"IDLE","print_error":0,"hms":[],"ams_status":0,"device":{"extruder":{"state":274,"info":[{"id":1,"info":1032,"stat":0,"snow":65279,"star":65279,"temp":25}]}},"ams":{"tray_exist_bits":"f","ams":[{"id":"0","info":"11001103","tray":[{"id":"0","tray_type":"PETG","tray_color":"FCE300FF","nozzle_temp_min":"230","nozzle_temp_max":"260"},{"id":"2","tray_type":"PETG","tray_color":"001489FF","nozzle_temp_min":"230","nozzle_temp_max":"260"}]}]}}`

func TestFilamentReadingSeparatesEmptySensorFromExternalSentinel(t *testing.T) {
	l := &listener{config: app.Printer{ModelID: "x2d"}, raw: map[string]any{}}
	reportJSON(t, l, filamentFrame)
	eq := readEquipment(l)
	if !eq.Filament.Supported || eq.Filament.HasFilament || eq.Filament.Source != "" || eq.Filament.Updated.IsZero() {
		t.Fatal(eq.Filament)
	}
	if eq.Extruders[0].HasFilament == nil || *eq.Extruders[0].HasFilament {
		t.Fatal("empty sentinel became loaded source")
	}
	if eq.Feeds[0].NozzleMin == nil || *eq.Feeds[0].NozzleMin != 230 {
		t.Fatal("temperature not parsed")
	}
	reportJSON(t, l, `{"command":"ams_change_filament","result":"success","device":{"extruder":{"state":274,"info":[{"id":1,"info":1038,"snow":2,"star":2,"stat":0}]}}}`)
	if readEquipment(l).Filament.HasFilament {
		t.Fatal("command echo invented loading")
	}
	l.config.ModelID = "a1"
	if readEquipment(l).Filament.Supported {
		t.Fatal("A1 supported incorrectly")
	}
}
func TestFilamentQoSZeroOneShotAndAckIsNotCompletion(t *testing.T) {
	p := app.Printer{ID: 1, ModelID: "x2d", Enabled: true, Configuration: "ams", Serial: "TEST"}
	m, _ := New("live")
	l := &listener{config: p, raw: map[string]any{}, connected: true}
	m.items[1] = l
	reportJSON(t, l, filamentFrame)
	count := 0
	l.client = motionClient{publish: func(topic string, qos byte, retain bool, v interface{}) mqtt.Token {
		count++
		if qos != 0 || retain || !strings.HasSuffix(topic, "/request") {
			t.Fatal("unsafe transport")
		}
		var body map[string]map[string]any
		json.Unmarshal(v.([]byte), &body)
		req := body["print"]
		if req["command"] != "ams_change_filament" || req["slot_id"] != float64(2) || req["tar_temp"] != float64(245) {
			t.Fatal(req)
		}
		m.mu.Lock()
		l.replies[req["sequence_id"].(string)] = map[string]any{"result": "success"}
		m.mu.Unlock()
		return motionToken{}
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if e := m.ChangeFilament(ctx, p, app.FilamentInput{Action: "load", Slot: "3"}); e == nil || count != 1 {
		t.Fatal("ack completed or resent", count, e)
	}
}
