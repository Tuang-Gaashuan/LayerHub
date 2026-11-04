package app

import (
	"strings"
	"testing"
	"time"
)

func TestOffLevelFanRejectedBeforeCommandCreation(t *testing.T) {
	s, u, d := setup(t)
	p, _ := fixture(t, s, u)
	s.Device = &equipmentFake{fakeDevice: d}
	settingFixture(d, p.ID, "idle", "")
	d.mu.Lock()
	v := d.statuses[p.ID]
	for _, key := range []string{"fan:1", "fan:2", "fan:3", "fan:10", "part_fan", "aux_fan", "chamber_fan"} {
		v.Equipment.Settings = append(v.Equipment.Settings, SettingReading{Key: key, Value: 0, Min: 0, Max: 100, Updated: time.Now()})
	}
	d.statuses[p.ID] = v
	d.mu.Unlock()
	for _, r := range v.Equipment.Settings[1:] {
		for _, value := range []int{1, 25, 75, 99} {
			err := s.SetEquipment(u, SettingInput{PrinterID: p.ID, Key: r.Key, Value: value})
			if err == nil || !strings.Contains(err.Error(), "10%") {
				t.Fatalf("%s %d: %v", r.Key, value, err)
			}
		}
	}
	var n int
	if err := s.DB.QueryRow("SELECT COUNT(*) FROM printer_commands").Scan(&n); err != nil || n != 0 {
		t.Fatalf("rejected values created command records: count=%d err=%v", n, err)
	}
	for _, value := range []int{20, 0} {
		if err := s.SetEquipment(u, SettingInput{PrinterID: p.ID, Key: "fan:1", Value: value}); err != nil {
			t.Fatal(err)
		}
		waitCommand(t, s, "succeeded")
	}
}
