package app

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func dualSliceEntries() map[string]string {
	return map[string]string{
		"Metadata/project_settings.config": `{"printer_model":"Bambu Lab X2D","nozzle_diameter":["0.4","0.4"],"physical_extruder_map":["1","0"],"curr_bed_type":"Textured PEI Plate"}`,
		"Metadata/plate_1.gcode":           "; FIXTURE ONLY",
		"Metadata/slice_info.config":       `<config><plate><metadata key="index" value="1"/><metadata key="prediction" value="4952"/><filament id="1" group_id="1" tray_info_idx="GFA00" type="PLA" color="#000000" used_g="4.34"/><filament id="2" group_id="0" tray_info_idx="GFG00" type="PETG" color="#FFFFFF" used_g="33.80"/><nozzle id="0" extruder_id="1" nozzle_diameter="0.4" volume_type="Standard"/><nozzle id="1" extruder_id="2" nozzle_diameter="0.4" volume_type="Standard"/></plate></config>`,
	}
}
func dualOptions() PrintOptions {
	return PrintOptions{Plate: 1, UseAMS: true, BedLeveling: true, Mapping: []AMSTarget{
		{Filament: 0, Source: "external", SpoolID: "pla", Grams: 5},
		{Filament: 1, Source: "ams", SlotID: 3, SpoolID: "petg", Grams: 34},
	}}
}
func dualTelemetry() Telemetry {
	v := x2dReadyTelemetry()
	now := time.Now()
	yes := true
	v.Equipment.Nozzles = append(v.Equipment.Nozzles, NozzleReading{ID: 0, Diameter: "0.4", Type: "HS01", Updated: now})
	v.Equipment.Feeds = append(v.Equipment.Feeds, FeedReading{Slot: "external", Material: "PLA", ProfileID: "GFA00", Present: &yes, Updated: now})
	v.Equipment.Extruders = []ExtruderReading{{ID: 0, HasFilament: &yes, CurrentSource: "external", Updated: now}}
	return v
}
func TestX2DDualRoutesAndRejections(t *testing.T) {
	entries := dualSliceEntries()
	file := filepath.Join(t.TempDir(), "dual.3mf")
	if e := os.WriteFile(file, sliceFixture(t, entries), 0600); e != nil {
		t.Fatal(e)
	}
	info, e := inspectSlice(file)
	if e != nil {
		t.Fatal(e)
	}
	p := info.Plates[0]
	if !p.MultiNozzle || p.Nozzle != nil || p.Filaments[0].Nozzle.Physical != 0 || p.Filaments[0].Nozzle.Logical != 1 || p.Filaments[1].Nozzle.Physical != 1 {
		t.Fatal("filament order confused with nozzle route", p)
	}
	job := Job{Path: file, PrintOptions: ptrDualOptions()}
	if _, _, e = x2dPrintSlice(job, *job.PrintOptions); e != nil {
		t.Fatal(e)
	}
	if e = x2dPrintReady(job, dualTelemetry()); e != nil {
		t.Fatal(e)
	}
	for _, bad := range []struct {
		name   string
		change func(*Telemetry)
	}{
		{"AMS bound to auxiliary", func(v *Telemetry) { zero := 0; v.Equipment.AMSUnits[0].Extruder = &zero }},
		{"stale binding", func(v *Telemetry) { v.Equipment.AMSUnits[0].Updated = time.Time{} }},
		{"wrong external profile", func(v *Telemetry) { v.Equipment.Feeds[1].ProfileID = "GFA01" }},
		{"stale feed", func(v *Telemetry) { v.Equipment.Feeds[1].Updated = time.Time{} }},
		{"future feed", func(v *Telemetry) { v.Equipment.Feeds[1].Updated = time.Now().Add(time.Hour) }},
		{"external not loaded", func(v *Telemetry) { v.Equipment.Extruders[0].HasFilament = nil }},
		{"external wrong nozzle", func(v *Telemetry) { v.Equipment.Extruders[0].ID = 1 }},
		{"stale extruder", func(v *Telemetry) { v.Equipment.Extruders[0].Updated = time.Time{} }},
		{"external wrong source", func(v *Telemetry) { v.Equipment.Extruders[0].CurrentSource = "external:1" }},
		{"wrong diameter", func(v *Telemetry) { v.Equipment.Nozzles[1].Diameter = "0.6" }},
		{"wrong flow", func(v *Telemetry) { v.Equipment.Nozzles[1].Type = "HH00" }},
		{"error", func(v *Telemetry) { v.PrintError = 1 }},
		{"switching", func(v *Telemetry) { v.Equipment.Switching = true }},
	} {
		t.Run(bad.name, func(t *testing.T) {
			v := dualTelemetry()
			bad.change(&v)
			if x2dPrintReady(job, v) == nil {
				t.Fatal("unsafe telemetry accepted")
			}
		})
	}
	for _, bad := range []struct {
		name   string
		change func(*PrintOptions)
	}{
		{"missing source", func(o *PrintOptions) { o.Mapping[0].Source = "" }},
		{"external virtual id injected", func(o *PrintOptions) { o.Mapping[0].AMSID = 255 }},
		{"sources swapped", func(o *PrintOptions) { o.Mapping[0].Source = "ams"; o.Mapping[1].Source = "external" }},
		{"duplicate filament", func(o *PrintOptions) { o.Mapping[1].Filament = 0 }},
		{"missing filament", func(o *PrintOptions) { o.Mapping = o.Mapping[:1] }},
		{"global external", func(o *PrintOptions) { o.UseAMS = false }},
		{"timelapse", func(o *PrintOptions) { o.Timelapse = true }},
		{"flow calibration", func(o *PrintOptions) { o.FlowCalibration = true }},
	} {
		t.Run(bad.name, func(t *testing.T) {
			o := dualOptions()
			bad.change(&o)
			if _, _, e := x2dPrintSlice(job, o); e == nil {
				t.Fatal("unsafe options accepted")
			}
		})
	}
	for _, bad := range []struct {
		name   string
		mutate func(map[string]string)
	}{
		{"missing physical map", func(m map[string]string) {
			m["Metadata/project_settings.config"] = strings.ReplaceAll(m["Metadata/project_settings.config"], `"physical_extruder_map":["1","0"],`, "")
		}},
		{"duplicate nozzle group", func(m map[string]string) {
			m["Metadata/slice_info.config"] = strings.ReplaceAll(m["Metadata/slice_info.config"], `<nozzle id="1"`, `<nozzle id="0"`)
		}},
		{"duplicate physical nozzle", func(m map[string]string) {
			m["Metadata/slice_info.config"] = strings.ReplaceAll(m["Metadata/slice_info.config"], `extruder_id="2"`, `extruder_id="1"`)
		}},
		{"mixed filament", func(m map[string]string) {
			m["Metadata/slice_info.config"] = strings.ReplaceAll(m["Metadata/slice_info.config"], "</plate>", "<mixed_filament/></plate>")
		}},
	} {
		t.Run(bad.name, func(t *testing.T) {
			m := dualSliceEntries()
			bad.mutate(m)
			f := filepath.Join(t.TempDir(), "bad.3mf")
			os.WriteFile(f, sliceFixture(t, m), 0600)
			if _, _, e := x2dPrintSlice(Job{Path: f}, dualOptions()); e == nil {
				t.Fatal("ambiguous nozzle metadata accepted")
			}
		})
	}
}
func ptrDualOptions() *PrintOptions { o := dualOptions(); return &o }

func TestX2DDualLifecycleUsesIndependentStockAndOneStart(t *testing.T) {
	s, u, driver := setup(t)
	p, pla := fixture(t, s, u)
	if e := s.SavePrinter(u, PrinterInput{ID: p.ID, ModelID: "x2d"}); e != nil {
		t.Fatal(e)
	}
	pla.Material = "PLA"
	pla.Mount = &Mount{p.ID, "external"}
	if e := s.SaveSpool(u, pla); e != nil {
		t.Fatal(e)
	}
	petg := pla
	petg.ID = ""
	petg.Revision = 0
	petg.Label = "White PETG"
	petg.Material = "PETG"
	petg.Mount = &Mount{p.ID, "4"}
	if e := s.SaveSpool(u, petg); e != nil {
		t.Fatal(e)
	}
	spools, _ := readSpools(s.DB)
	for _, sp := range spools {
		if sp.Material == "PETG" {
			petg = sp
		}
	}
	raw := sliceFixture(t, dualSliceEntries())
	file := filepath.Join(s.Config.DataDir, "uploads", "dual.3mf")
	os.WriteFile(file, raw, 0600)
	hash := sha256.Sum256(raw)
	id, e := s.CreateJob(u, Schedule{Printer: p.ID, Estimate: 83, Grams: 5, SpoolID: pla.ID}, "dual.3mf", file, hex.EncodeToString(hash[:]), "", int64(len(raw)))
	if e != nil {
		t.Fatal(e)
	}
	o := dualOptions()
	o.Mapping[0].SpoolID = pla.ID
	o.Mapping[1].SpoolID = petg.ID
	bad := o
	bad.Mapping = append([]AMSTarget{}, o.Mapping...)
	bad.Mapping[0].Grams = 4
	if s.SavePrintOptions(u, id, bad) == nil {
		t.Fatal("underreserved PLA accepted")
	}
	if e = s.SavePrintOptions(u, id, o); e != nil {
		t.Fatal(e)
	}
	jobs, _ := readJobs(s.DB)
	if jobUses(jobs[0])[pla.ID] != 5 || jobUses(jobs[0])[petg.ID] != 34 || jobs[0].Grams != 39 {
		t.Fatal("two reservations not kept independently")
	}
	driver.mu.Lock()
	driver.statuses[p.ID] = dualTelemetry()
	driver.mu.Unlock()
	fake := &x2dProjectFake{fakeDevice: driver}
	s.Device = fake
	if e = s.DispatchJob(u, id); e != nil {
		t.Fatal(e)
	}
	waitState(t, s, id, "printing")
	if s.StartJob(u, id) == nil {
		t.Fatal("duplicate start accepted")
	}
	fake.mu.Lock()
	request := fake.request
	uploads := fake.uploads
	fake.mu.Unlock()
	if request.ExternalSources[0] != 255 || len(request.ExternalSources) != 1 || !request.Options.UseAMS || request.Options.Mapping[1].SlotID != 3 || uploads != 1 {
		t.Fatal("mixed feed route lost", request, uploads)
	}
	var starts int
	s.DB.QueryRow("SELECT count(*) FROM printer_commands WHERE job_id=? AND action='start'", id).Scan(&starts)
	if starts != 1 {
		t.Fatal("more than one start")
	}
	spools, _ = readSpools(s.DB)
	for _, sp := range spools {
		if sp.Remaining != 1000 {
			t.Fatal("estimate deducted stock")
		}
	}
}
