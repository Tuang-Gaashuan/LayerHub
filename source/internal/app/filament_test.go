package app

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"
)

func filamentFixture() (Printer, Telemetry) {
	now := time.Now()
	yes := true
	min, max := 230, 260
	p := Printer{ID: 1, ModelID: "x2d", Configuration: "ams", Enabled: true}
	t := Telemetry{Connected: true, RawState: "IDLE", State: "idle", Updated: now, StateUpdated: now, Equipment: Equipment{Filament: FilamentReading{Supported: true, Extruder: 1, Updated: now}}}
	for i, color := range []string{"FCE300FF", "000000FF", "001489FF", "FFFFFFFF"} {
		t.Equipment.Feeds = append(t.Equipment.Feeds, FeedReading{Slot: string(rune('1' + i)), Material: "PETG", Color: color, Present: &yes, NozzleMin: &min, NozzleMax: &max, ControlUpdated: now})
	}
	return p, t
}
func TestFilamentPlansUsePhysicalSlotsAndReportedTemperatures(t *testing.T) {
	p, tel := filamentFixture()
	plan, e := PlanFilament(p, tel, FilamentInput{Action: "load", Slot: "3"})
	if e != nil || plan.Slot != 2 || plan.Source != "3" || plan.OldTemp != -1 || plan.NewTemp != 245 {
		t.Fatal(plan, e)
	}
	var body map[string]map[string]any
	json.Unmarshal(FilamentPayload(plan, "test"), &body)
	q := body["print"]
	if q["ams_id"] != float64(0) || q["extruder_id"] != float64(1) || q["target"] != float64(2) || q["slot_id"] != float64(2) {
		t.Fatal(q)
	}
	tel.Equipment.Filament.HasFilament = true
	tel.Equipment.Filament.Source = "1"
	plan, e = PlanFilament(p, tel, FilamentInput{Action: "load", Slot: "4"})
	if e != nil || plan.OldTemp != 245 || plan.Slot != 3 {
		t.Fatal(plan, e)
	}
	plan, e = PlanFilament(p, tel, FilamentInput{Action: "unload"})
	if e != nil || !plan.Unload || plan.Slot != 255 || plan.NewTemp != 245 {
		t.Fatal(plan, e)
	}
	for _, slot := range []string{"0", "5", "external", "01", "ams:0:0"} {
		if _, e = PlanFilament(p, tel, FilamentInput{Action: "load", Slot: slot}); e == nil {
			t.Fatal("unsupported slot", slot)
		}
	}
	if _, e = PlanFilament(p, tel, FilamentInput{Action: "load", Slot: "1"}); e == nil {
		t.Fatal("already loaded source resent")
	}
}
func TestFilamentGuardsAndCompletionNeedFreshIndependentSensorState(t *testing.T) {
	p, tel := filamentFixture()
	sent := time.Now().Add(-time.Second)
	plan, _ := PlanFilament(p, tel, FilamentInput{Action: "load", Slot: "3"})
	for _, change := range []func(*Telemetry){func(v *Telemetry) { v.Connected = false }, func(v *Telemetry) { v.RawState = "RUNNING" }, func(v *Telemetry) { v.Equipment.Filament.Busy = true }, func(v *Telemetry) { v.Equipment.Filament.Step = 7 }, func(v *Telemetry) { v.Equipment.Filament.Supported = false }, func(v *Telemetry) { v.Equipment.Filament.Updated = sent.Add(-time.Minute) }, func(v *Telemetry) { v.Equipment.HMS = []string{"alarm"} }} {
		v := tel
		change(&v)
		if _, e := PlanFilament(p, v, FilamentInput{Action: "load", Slot: "3"}); e == nil {
			t.Fatal("unsafe state accepted")
		}
	}
	if FilamentComplete(tel, plan, sent) {
		t.Fatal("ack/empty state cannot complete load")
	}
	tel.Equipment.Filament.HasFilament = true
	tel.Equipment.Filament.Source = "3"
	tel.Equipment.Filament.Target = "3"
	tel.Equipment.Filament.MainState = 3
	if !FilamentComplete(tel, plan, sent) {
		t.Fatal("completed assist state rejected")
	}
	tel.Equipment.Filament.Step = 7
	if FilamentComplete(tel, plan, sent) {
		t.Fatal("purge treated as completion")
	}
	tel.Equipment.Filament.Step = 0
	tel.Equipment.Filament.Updated = sent.Add(-time.Millisecond)
	if FilamentComplete(tel, plan, sent) {
		t.Fatal("old snapshot completed command")
	}
	p.ModelID = "a1"
	if _, e := PlanFilament(p, tel, FilamentInput{Action: "unload"}); e == nil {
		t.Fatal("A1 unexpectedly opened")
	}
}

type filamentFake struct {
	*fakeDevice
	calls atomic.Int32
	hold  chan struct{}
}

func (f *filamentFake) ChangeFilament(ctx context.Context, p Printer, in FilamentInput) error {
	f.calls.Add(1)
	select {
	case <-f.hold:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func TestFilamentSingleSendSharesCommandLockAndDoesNotChangeJobsOrStock(t *testing.T) {
	s, u, d := setup(t)
	p, _ := fixture(t, s, u)
	s.Config.DeviceMode = "live"
	s.DB.Exec("UPDATE printers SET model_id='x2d',configuration='ams' WHERE id=?", p.ID)
	_, tel := filamentFixture()
	d.mu.Lock()
	d.statuses[p.ID] = tel
	d.mu.Unlock()
	f := &filamentFake{fakeDevice: d, hold: make(chan struct{})}
	s.Device = f
	in := FilamentInput{PrinterID: p.ID, Action: "load", Slot: "3", RequestID: "filament-request-001", Clear: true}
	if e := s.ChangeFilament(User{ID: 999}, in); e == nil {
		t.Fatal("non-admin accepted")
	}
	if e := s.ChangeFilament(u, in); e != nil {
		t.Fatal(e)
	}
	if e := s.ChangeFilament(u, in); e != nil {
		t.Fatal("same request replay should not resend", e)
	}
	other := in
	other.RequestID = "filament-request-002"
	if e := s.ChangeFilament(u, other); e == nil {
		t.Fatal("concurrent filament control")
	}
	if e := s.SetEquipment(u, SettingInput{PrinterID: p.ID, Key: "bed", Value: 0}); e == nil {
		t.Fatal("settings bypassed filament lock")
	}
	close(f.hold)
	waitCommand(t, s, "succeeded")
	if f.calls.Load() != 1 {
		t.Fatal("duplicate physical request")
	}
	var n int
	s.DB.QueryRow("SELECT count(*) FROM job_consumption").Scan(&n)
	if n != 0 {
		t.Fatal("filament operation settled stock")
	}
}
