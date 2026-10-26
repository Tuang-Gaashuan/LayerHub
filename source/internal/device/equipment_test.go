package device

import (
	"encoding/json"
	"layerhub.local/server/internal/app"
	"testing"
	"time"
)

func reportJSON(t *testing.T, l *listener, raw string) {
	t.Helper()
	var r map[string]any
	if e := json.Unmarshal([]byte(raw), &r); e != nil {
		t.Fatal(e)
	}
	applyReport(l, r)
}

func TestEquipmentDualExtruderDeltaAndMixedFeed(t *testing.T) {
	l := &listener{config: app.Printer{ModelID: "h2d"}, raw: map[string]any{}}
	reportJSON(t, l, `{"device":{"extruder":{"state":2,"info":[{"id":0,"temp":14418130,"snow":0,"star":0},{"id":1,"temp":25,"snow":65024,"star":65024}]}},"ams":{"ams":[{"id":"0","info":"00000003","tray":[{"id":"0","tray_type":"PLA","remain":75}]}]},"vir_slot":[{"id":254,"tray_type":"TPU"}]}`)
	eq := readEquipment(l)
	if len(eq.Extruders) != 2 || *eq.Extruders[0].Temperature != 210 || *eq.Extruders[0].Target != 220 || eq.Extruders[0].CurrentSource != "1" || eq.Extruders[1].CurrentSource != "external:1" {
		t.Fatalf("incorrect per-nozzle state: %+v", eq.Extruders)
	}
	if len(eq.Settings) != 0 || len(eq.Feeds) != 2 || eq.Feeds[0].Kind != "AMS 2 Pro" || eq.Feeds[1].Remaining != nil {
		t.Fatalf("invented capability or remaining: %+v", eq)
	}
	reportJSON(t, l, `{"device":{"extruder":{"info":[{"id":1,"temp":15728840}]}}}`)
	eq = readEquipment(l)
	if len(eq.Extruders) != 2 || *eq.Extruders[0].Target != 220 || *eq.Extruders[1].Target != 240 {
		t.Fatal("partial nozzle delta lost another nozzle")
	}
	reportJSON(t, l, `{"ams":{"ams":[{"id":"0","tray":[{"id":"0","remain_g":0}]}]}}`)
	eq = readEquipment(l)
	if eq.Feeds[0].Remaining == nil || *eq.Feeds[0].Remaining != 0 || eq.Feeds[0].RemainingUnit != "g" {
		t.Fatal("zero remaining treated as missing")
	}
}

func TestSettingsRequireFreshSpecificObservation(t *testing.T) {
	l := &listener{config: app.Printer{ModelID: "p1s"}, raw: map[string]any{}, replies: map[string]map[string]any{"42": nil}}
	reportJSON(t, l, `{"nozzle_target_temper":210,"lights_report":[{"node":"chamber_light","mode":"on"}],"spd_lvl":2}`)
	sent := time.Now().Add(-time.Millisecond)
	for key := range l.fields {
		l.fields[key] = sent.Add(-time.Second)
	}
	in := app.SettingInput{Key: "nozzle", Value: 210}
	if settingMatches(app.Telemetry{Equipment: readEquipment(l)}, in, sent) {
		t.Fatal("old matching target confirmed command")
	}
	reportJSON(t, l, `{"command":"gcode_line","sequence_id":"42","result":"success","nozzle_target_temper":210}`)
	if settingMatches(app.Telemetry{Equipment: readEquipment(l)}, in, sent) {
		t.Fatal("command echo confirmed hardware")
	}
	reportJSON(t, l, `{"mc_percent":60}`)
	if settingMatches(app.Telemetry{Equipment: readEquipment(l)}, in, sent) {
		t.Fatal("unrelated delta confirmed hardware")
	}
	reportJSON(t, l, `{"nozzle_target_temper":210}`)
	if !settingMatches(app.Telemetry{Equipment: readEquipment(l)}, in, sent) {
		t.Fatal("fresh target not accepted")
	}
	l.fields["light.chamber_light"] = sent.Add(-time.Second)
	reportJSON(t, l, `{"lights_report":[{"node":"chamber_light2","mode":"on"}]}`)
	if settingMatches(app.Telemetry{Equipment: readEquipment(l)}, app.SettingInput{Key: "chamber_light", Value: 1}, sent) {
		t.Fatal("other lamp confirmed chamber lamp")
	}
	section, payload := settingPayload(app.SettingInput{Key: "chamber_light", Value: 0})
	if section != "system" || payload["led_node"] != "chamber_light" || payload["led_mode"] != "off" {
		t.Fatal("wrong lamp command namespace")
	}
}

func TestSettingsMalformedAndUnknownFieldsStayUnavailable(t *testing.T) {
	l := &listener{config: app.Printer{ModelID: "p1s"}, raw: map[string]any{}}
	reportJSON(t, l, `{"nozzle_target_temper":"bad","bed_target_temper":9999,"spd_lvl":999,"hms":[{"attr":65538,"code":196612}]}`)
	eq := readEquipment(l)
	if len(eq.Settings) != 0 || eq.Extruders[0].Temperature != nil || eq.HMS[0] != "0001-0002-0003-0004" {
		t.Fatalf("bad telemetry accepted: %+v", eq)
	}
	l.config.ModelID = "p2s"
	reportJSON(t, l, `{"nozzle_target_temper":200}`)
	if len(readEquipment(l).Settings) != 0 {
		t.Fatal("unadapted protocol given write capability")
	}
}

func TestX2DActualDualReadingAMSAndEmptyExternalSlots(t *testing.T) {
	l := &listener{config: app.Printer{ModelID: "x2d"}, raw: map[string]any{}}
	reportJSON(t, l, `{"device":{"extruder":{"state":274,"info":[{"id":0,"temp":31,"snow":65535,"star":65535},{"id":1,"temp":28,"snow":65279,"star":65279}]}},"ams":{"tray_exist_bits":"f","ams":[{"id":"0","info":"11001103","humidity":"1","temp":"28.4","tray":[{"id":"0","tray_type":"PETG","tray_color":"FCE300FF","remain":73},{"id":"1","tray_type":"PETG","tray_color":"000000FF","remain":90}]}]},"vir_slot":[{"id":"254","tray_type":"","tray_color":"00000000","remain":0},{"id":"255","tray_type":"","tray_color":"00000000","remain":0}],"ipcam":{"ipcam_dev":"1","rtsp_url":"rtsps://192.0.2.1:322/streaming/live/1","resolution":"1080p"}}`)
	eq := readEquipment(l)
	if len(eq.Extruders) != 2 || *eq.Extruders[0].Temperature != 31 || *eq.Extruders[1].Temperature != 28 {
		t.Fatal("dual readings merged")
	}
	if len(eq.Settings) != 2 || eq.Settings[0].Key != "nozzle:0" || eq.Settings[1].Key != "nozzle:1" {
		t.Fatal("X2D per-nozzle controls missing or guessed unsupported settings")
	}
	if len(eq.AMSUnits) != 1 || eq.AMSUnits[0].Kind != "AMS 2 Pro" || *eq.AMSUnits[0].HumidityIndex != 1 || *eq.AMSUnits[0].Temperature != 28.4 {
		t.Fatal("AMS type, temperature or humidity semantics lost")
	}
	if !eq.Camera.Supported || eq.Camera.Protocol != "rtsps" {
		t.Fatal("RTSPS camera not recognized")
	}
	if len(eq.Feeds) != 4 || eq.Feeds[0].Present == nil || !*eq.Feeds[0].Present || *eq.Feeds[0].Remaining != 73 || eq.Feeds[0].RemainingUnit != "%" {
		t.Fatal("estimated amount or occupancy lost")
	}
	for _, f := range eq.Feeds[2:] {
		if f.Present == nil || *f.Present || f.Remaining != nil {
			t.Fatal("empty external reported as usable material")
		}
	}
	reportJSON(t, l, `{"ams":{"tray_exist_bits":"e"}}`)
	f := readEquipment(l).Feeds[0]
	if f.Present == nil || *f.Present || f.Material != "" || f.Remaining != nil {
		t.Fatal("removed spool retained stale material details")
	}
}
