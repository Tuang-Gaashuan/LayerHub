package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type x2dProjectFake struct {
	*fakeDevice
	request PrintRequest
	reject  bool
}

func (d *x2dProjectFake) Control(context.Context, Printer, string) error {
	return errors.New("unexpected control in print test")
}

func (d *x2dProjectFake) StartPrint(ctx context.Context, p Printer, r PrintRequest) error {
	d.mu.Lock()
	d.request = r
	d.mu.Unlock()
	if d.reject {
		return ErrDeviceRejected
	}
	return d.Start(ctx, p, r.Remote)
}
func x2dReadyTelemetry() Telemetry {
	now := time.Now()
	one := 1
	present := true
	return Telemetry{Connected: true, Updated: now, StateUpdated: now, State: "idle", Equipment: Equipment{
		Nozzles:  []NozzleReading{{ID: 1, Diameter: "0.4", Type: "HS00", Updated: now}},
		AMSUnits: []AMSUnitReading{{ID: 0, Extruder: &one, Updated: now}},
		Feeds:    []FeedReading{{Slot: "4", Material: "PETG", ProfileID: "GFG00", Present: &present, Updated: now}},
	}}
}
func TestX2DSingleNozzleLifecycleAndLivePreflight(t *testing.T) {
	for _, reject := range []bool{false, true} {
		t.Run(map[bool]string{false: "confirmed", true: "rejected"}[reject], func(t *testing.T) {
			s, u, driver := setup(t)
			p, sp := fixture(t, s, u)
			if err := s.SavePrinter(u, PrinterInput{ID: p.ID, ModelID: "x2d"}); err != nil {
				t.Fatal(err)
			}
			sp.Material = "PETG"
			sp.Mount = &Mount{p.ID, "4"}
			if err := s.SaveSpool(u, sp); err != nil {
				t.Fatal(err)
			}
			raw := sliceFixture(t, map[string]string{
				"Metadata/project_settings.config": `{"printer_model":"Bambu Lab X2D","nozzle_diameter":["0.4","0.4"],"physical_extruder_map":["1","0"],"curr_bed_type":"Textured PEI Plate"}`,
				"Metadata/plate_1.gcode":           "; TEST ONLY",
				"Metadata/slice_info.config":       `<config><plate><metadata key="index" value="1"/><filament id="1" group_id="0" tray_info_idx="GFG00" type="PETG" used_g="37.02"/><nozzle id="0" extruder_id="1" nozzle_diameter="0.4" volume_type="Standard"/></plate></config>`,
			})
			file := filepath.Join(s.Config.DataDir, "uploads", "x2d.3mf")
			if err := os.WriteFile(file, raw, 0600); err != nil {
				t.Fatal(err)
			}
			hash := sha256.Sum256(raw)
			id, err := s.CreateJob(u, Schedule{Printer: p.ID, Estimate: 123, Grams: 38, SpoolID: sp.ID}, "x2d.3mf", file, hex.EncodeToString(hash[:]), "", int64(len(raw)))
			if err != nil {
				t.Fatal(err)
			}
			o := PrintOptions{Plate: 1, UseAMS: true, BedLeveling: true, Mapping: []AMSTarget{{Filament: 0, AMSID: 0, SlotID: 3, SpoolID: sp.ID, Grams: 38}}}
			if err = s.SavePrintOptions(u, id, o); err != nil {
				t.Fatal(err)
			}
			jobs, _ := readJobs(s.DB)
			job := jobs[0]
			_, plate, err := x2dPrintSlice(job, o)
			if err != nil || plate.Nozzle.Physical != 1 || plate.Nozzle.Logical != 0 || plate.Filaments[0].ProfileID != "GFG00" {
				t.Fatal("logical index was mistaken for a physical nozzle", plate, err)
			}
			for _, bad := range []struct {
				name   string
				mutate func(*Telemetry)
			}{
				{"wrong AMS binding", func(v *Telemetry) { zero := 0; v.Equipment.AMSUnits[0].Extruder = &zero }},
				{"stale AMS binding", func(v *Telemetry) { v.Equipment.AMSUnits[0].Updated = time.Time{} }},
				{"wrong diameter", func(v *Telemetry) { v.Equipment.Nozzles[0].Diameter = "0.6" }},
				{"wrong nozzle flow", func(v *Telemetry) { v.Equipment.Nozzles[0].Type = "HH00" }},
				{"stale nozzle", func(v *Telemetry) { v.Equipment.Nozzles[0].Updated = time.Time{} }},
				{"wrong material subclass", func(v *Telemetry) { v.Equipment.Feeds[0].ProfileID = "GFG02" }},
				{"wrong material", func(v *Telemetry) { v.Equipment.Feeds[0].Material = "PLA" }},
				{"empty slot", func(v *Telemetry) { b := false; v.Equipment.Feeds[0].Present = &b }},
				{"stale slot", func(v *Telemetry) { v.Equipment.Feeds[0].Updated = time.Time{} }},
				{"switching", func(v *Telemetry) { v.Equipment.Switching = true }},
				{"HMS", func(v *Telemetry) { v.Equipment.HMS = []string{"not resolved"} }},
				{"authorization", func(v *Telemetry) { v.Equipment.ControlIssue = "authorization_required" }},
			} {
				v := x2dReadyTelemetry()
				bad.mutate(&v)
				if x2dPrintReady(job, v) == nil {
					t.Fatal("unsafe live condition accepted", bad.name)
				}
			}
			if err = x2dPrintReady(job, x2dReadyTelemetry()); err != nil {
				t.Fatal(err)
			}
			driver.mu.Lock()
			driver.statuses[p.ID] = x2dReadyTelemetry()
			driver.mu.Unlock()
			fake := &x2dProjectFake{fakeDevice: driver, reject: reject}
			s.Device = fake
			if !reject {
				s.Config.EnablePrintStart = false
				if s.DispatchJob(u, id) == nil {
					t.Fatal("global dispatch switch bypassed")
				}
				s.Config.EnablePrintStart = true
				if err = s.DispatchJob(u, id); err != nil {
					t.Fatal(err)
				}
			} else {
				if err = s.SendJob(u, id); err != nil {
					t.Fatal(err)
				}
				waitState(t, s, id, "assigned")
				s.Config.EnablePrintStart = false
				if s.StartJob(u, id) == nil {
					t.Fatal("global start switch bypassed")
				}
				s.Config.EnablePrintStart = true
				if err = s.StartJob(u, id); err != nil {
					t.Fatal(err)
				}
			}
			if reject {
				waitState(t, s, id, "assigned")
				var state string
				s.DB.QueryRow("SELECT state FROM printer_commands WHERE job_id=? AND action='start'", id).Scan(&state)
				if state != "rejected" {
					t.Fatal(state)
				}
			} else {
				waitState(t, s, id, "printing")
			}
			fake.mu.Lock()
			request := fake.request
			uploads := fake.uploads
			fake.mu.Unlock()
			if request.BedType != "pte" || request.Options.Mapping[0].SlotID != 3 || uploads != 1 {
				t.Fatal("incorrect print route", request, uploads)
			}
			spools, _ := readSpools(s.DB)
			if spools[0].Remaining != 1000 {
				t.Fatal("print start deducted estimated stock")
			}
		})
	}
}
