package device

import (
	"context"
	"layerhub.local/server/internal/app"
	"testing"
	"time"
)

func TestX2DPrintRouteMetadataFreshness(t *testing.T) {
	l := &listener{config: app.Printer{ModelID: "x2d", Configuration: "ams"}, raw: map[string]any{}}
	reportJSON(t, l, `{"device":{"nozzle":{"info":[{"id":0,"diameter":0.4,"type":"HS00"},{"id":1,"diameter":0.4,"type":"HS00"}]}},"ams":{"ams":[{"id":"0","info":"11001103","tray":[{"id":"3","tray_type":"PETG","tray_info_idx":"GFG00"}]}]}}`)
	eq := readEquipment(l)
	if len(eq.Nozzles) != 2 || eq.Nozzles[1].Diameter != "0.4" || eq.Nozzles[1].Type != "HS00" || eq.Nozzles[1].Updated.IsZero() || len(eq.AMSUnits) != 1 || eq.AMSUnits[0].Extruder == nil || *eq.AMSUnits[0].Extruder != 1 || eq.AMSUnits[0].Updated.IsZero() || eq.Feeds[0].ProfileID != "GFG00" {
		t.Fatal(eq)
	}
	stamp := eq.Nozzles[1].Updated
	bind := eq.AMSUnits[0].Updated
	reportJSON(t, l, `{"device":{"extruder":{"info":[{"id":1,"temp":25}]}},"ams":{"ams":[{"id":"0","humidity":"1"}]}}`)
	later := readEquipment(l)
	if !later.Nozzles[1].Updated.Equal(stamp) || !later.AMSUnits[0].Updated.Equal(bind) {
		t.Fatal("unrelated telemetry renewed nozzle or AMS binding")
	}
}

func TestX2DSettingsUseIndependentFreshFieldsAndFanModePermissions(t *testing.T) {
	p := app.Printer{ID: 1, ModelID: "x2d", Configuration: "ams", Enabled: true}
	l := &listener{config: p, raw: map[string]any{}}
	applyReport(l, simulationReport(p))
	eq := readEquipment(l)
	if len(eq.Settings) != 9 {
		t.Fatalf("expected two nozzles, bed, speed, lamp, four fans: %+v", eq.Settings)
	}
	sent := time.Now().Add(-time.Millisecond)
	for key := range l.fields {
		l.fields[key] = sent.Add(-time.Second)
	}
	in := app.SettingInput{Key: "nozzle:0", Value: 180}
	reportJSON(t, l, `{"device":{"extruder":{"info":[{"id":1,"temp":11796505}]}}}`)
	if settingMatches(app.Telemetry{Equipment: readEquipment(l)}, in, sent) {
		t.Fatal("another nozzle confirmed command")
	}
	reportJSON(t, l, `{"command":"set_nozzle_temp","device":{"extruder":{"info":[{"id":0,"temp":11796505}]}}}`)
	if settingMatches(app.Telemetry{Equipment: readEquipment(l)}, in, sent) {
		t.Fatal("command echo confirmed command")
	}
	reportJSON(t, l, `{"device":{"extruder":{"info":[{"id":0,"temp":11796505}]}}}`)
	if !settingMatches(app.Telemetry{Equipment: readEquipment(l)}, in, sent) {
		t.Fatal("fresh selected nozzle target not confirmed")
	}
	section, payload := settingPayload(in)
	if section != "print" || payload["command"] != "set_nozzle_temp" || payload["extruder_index"] != 0 || payload["target_temp"] != 180 {
		t.Fatal("incorrect explicit nozzle command")
	}
	_, payload = settingPayload(app.SettingInput{Key: "fan:10", Value: 25})
	if payload["command"] != "set_fan" || payload["fan_index"] != 10 || payload["speed"] != 25 {
		t.Fatal("incorrect new fan command")
	}
	_, payload = settingPayload(app.SettingInput{Key: "fan:0", Value: 50})
	if payload != nil {
		t.Fatal("protected heatbreak fan exposed")
	}
	reportJSON(t, l, `{"device":{"airduct":{"modeCur":1,"modeList":[{"modeId":1,"ctrl":[16,160,48],"off":[32]}]}}}`)
	if _, ok := findSetting(app.Telemetry{Equipment: readEquipment(l)}, "fan:2"); ok {
		t.Fatal("automatically managed fan is manually writable")
	}
	reportJSON(t, l, `{"device":{"bed_temp":7864347}}`)
	eq = readEquipment(l)
	if *eq.Bed != 27 || *eq.BedTarget != 120 {
		t.Fatal("packed bed temperature lost")
	}
	bed, ok := findSetting(app.Telemetry{Equipment: eq}, "bed")
	if !ok || bed.Max != 100 {
		t.Fatal("cannot turn off preexisting high target with conservative limit")
	}
	l.raw["fun"] = "8000000000"
	if !mqttBedControl(l.raw) {
		t.Fatal("new MQTT bed capability not parsed")
	}
	l.fields["airduct.modeCur"] = time.Now().Add(-time.Minute * 2)
	if _, ok := findSetting(app.Telemetry{Equipment: readEquipment(l)}, "fan:1"); ok {
		t.Fatal("stale fan mode remains writable")
	}
}
func TestX2DSimulatedSettingsPreserveOtherNozzleAndInventoryTelemetry(t *testing.T) {
	m, _ := New("simulated")
	defer m.Close()
	p := app.Printer{ID: 1, ModelID: "x2d", Configuration: "ams", Enabled: true}
	m.Reconcile(context.Background(), []app.Printer{p})
	for _, in := range []app.SettingInput{{Key: "nozzle:0", Value: 180}, {Key: "nozzle:1", Value: 220}, {Key: "bed", Value: 60}, {Key: "fan:10", Value: 30}} {
		in.PrinterID = 1
		if err := m.SetEquipment(context.Background(), p, in); err != nil {
			t.Fatal(err)
		}
	}
	eq := m.Status(1).Equipment
	if *eq.Extruders[0].Target != 180 || *eq.Extruders[1].Target != 220 || *eq.BedTarget != 60 {
		t.Fatal("independent targets changed each other")
	}
	fan, ok := findSetting(app.Telemetry{Equipment: eq}, "fan:10")
	if !ok || fan.Value != 30 {
		t.Fatal("fan simulation missing")
	}
	if len(eq.Feeds) == 0 || eq.Feeds[0].Updated.IsZero() {
		t.Fatal("machine feed observation lacks freshness")
	}
	if err := m.SetEquipment(context.Background(), p, app.SettingInput{Key: "nozzle:0", Value: 301}); err == nil {
		t.Fatal("temperature bound bypassed")
	}
}
