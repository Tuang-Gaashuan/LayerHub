package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type equipmentFake struct {
	*fakeDevice
	hold   chan struct{}
	fail   bool
	reject bool
}

func (f *equipmentFake) SetEquipment(ctx context.Context, p Printer, in SettingInput) error {
	if f.hold != nil {
		select {
		case <-f.hold:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if f.fail {
		return context.DeadlineExceeded
	}
	if f.reject {
		return ErrDeviceRejected
	}
	return nil
}
func settingFixture(d *fakeDevice, pid int64, state, file string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	now := time.Now()
	d.statuses[pid] = Telemetry{Connected: true, Updated: now, StateUpdated: now, State: state, File: file, Equipment: Equipment{Settings: []SettingReading{{Key: "nozzle", Value: 0, Min: 0, Max: 300, Unit: "°C", Updated: now}}}}
}
func waitCommand(t *testing.T, s *Service, state string) {
	t.Helper()
	until := time.Now().Add(3 * time.Second)
	for time.Now().Before(until) {
		var got string
		s.DB.QueryRow("SELECT state FROM printer_commands ORDER BY id DESC LIMIT 1").Scan(&got)
		if got == state {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("command did not reach " + state)
}

func TestEquipmentPermissionMutualExclusionAndManualReview(t *testing.T) {
	s, u, d := setup(t)
	p, sp := fixture(t, s, u)
	f := &equipmentFake{fakeDevice: d, hold: make(chan struct{})}
	s.Device = f
	settingFixture(d, p.ID, "idle", "")
	in := SettingInput{PrinterID: p.ID, Key: "nozzle", Value: 200}
	if s.SetEquipment(User{ID: 888}, in) == nil {
		t.Fatal("member changed idle printer")
	}
	bad := in
	bad.Value = 301
	if s.SetEquipment(u, bad) == nil {
		t.Fatal("out of range accepted")
	}
	bad = in
	bad.Key = "raw_gcode"
	if s.SetEquipment(u, bad) == nil {
		t.Fatal("unsupported setting accepted")
	}
	if e := s.SetEquipment(u, in); e != nil {
		t.Fatal(e)
	}
	if s.SetEquipment(u, in) == nil {
		t.Fatal("duplicate pending command")
	}
	if s.SavePrinter(u, PrinterInput{ID: p.ID, Name: "changed"}) == nil {
		t.Fatal("connection changed during command")
	}
	id := newJob(t, s, u, p, sp, 12)
	if s.SendJob(u, id) == nil {
		t.Fatal("transfer started during equipment command")
	}
	close(f.hold)
	waitCommand(t, s, "succeeded")
	f.hold = nil
	f.fail = true
	if e := s.SetEquipment(u, in); e != nil {
		t.Fatal(e)
	}
	waitCommand(t, s, "uncertain")
	if s.SetEquipment(u, in) == nil {
		t.Fatal("retried uncertain command")
	}
	if e := s.SetEquipment(u, SettingInput{PrinterID: p.ID, Key: "resolve_settings"}); e != nil {
		t.Fatal(e)
	}
	waitCommand(t, s, "reviewed")
	if e := s.SaveUser(u, "member", "任务成员", "MemberPassword123", false); e != nil {
		t.Fatal(e)
	}
	var member User
	s.DB.QueryRow("SELECT id,username,name FROM users WHERE username='member'").Scan(&member.ID, &member.Username, &member.Name)
	s.DB.Exec("UPDATE jobs SET user_id=?,state='printing',remote_path='test.gcode' WHERE id=?", member.ID, id)
	settingFixture(d, p.ID, "printing", "test.gcode")
	if s.SetEquipment(User{ID: 999}, in) == nil {
		t.Fatal("other member changed running task")
	}
	f.fail = false
	if e := s.SetEquipment(member, in); e != nil {
		t.Fatal(e)
	}
	waitCommand(t, s, "succeeded")
	settingFixture(d, p.ID, "printing", "external.gcode")
	if s.SetEquipment(u, in) == nil {
		t.Fatal("unmatched external task controlled")
	}
}

func TestExtendedSupplyPrivateTelemetryAndLegacyHTAlias(t *testing.T) {
	s, u, d := setup(t)
	p, sp := fixture(t, s, u)
	for _, slot := range []string{"ams:1:3", "ams:128:0"} {
		sp.Mount = &Mount{p.ID, slot}
		if e := s.SaveSpool(u, sp); e != nil {
			t.Fatal(e)
		}
		sp.Revision++
	}
	for _, slot := range []string{"ams:0:0", "ams:128:1", "ams:254:0", "ams:01:0", "external:1"} {
		sp.Mount = &Mount{p.ID, slot}
		if s.SaveSpool(u, sp) == nil {
			t.Fatal("invalid source " + slot)
		}
	}
	if e := s.SaveUser(u, "private_owner", "个人卷用户", "MemberPassword123", false); e != nil {
		t.Fatal(e)
	}
	var owner User
	s.DB.QueryRow("SELECT id,username,name FROM users WHERE username='private_owner'").Scan(&owner.ID, &owner.Username, &owner.Name)
	private := sp
	private.ID = ""
	private.Owner = owner.Username
	private.Label = "私人详细名称"
	private.Mount = &Mount{p.ID, "external"}
	if e := s.SaveSpool(owner, private); e != nil {
		t.Fatal(e)
	}
	grams := 700
	d.mu.Lock()
	v := d.statuses[p.ID]
	v.AMS = map[string]any{"secret": "RFID"}
	v.Equipment = Equipment{Feeds: []FeedReading{{Slot: "external", Material: "PA-CF", Color: "ABCDEF", Remaining: &grams}}}
	d.statuses[p.ID] = v
	d.mu.Unlock()
	snap, e := s.Snapshot(u)
	if e != nil {
		t.Fatal(e)
	}
	printer := snap["printers"].([]Printer)[0]
	if printer.AMS != nil || printer.Equipment.Feeds[0].Material != "个人耗材" || printer.Equipment.Feeds[0].Remaining != nil {
		t.Fatal("private telemetry leaked")
	}
	snap, e = s.Snapshot(owner)
	if e != nil {
		t.Fatal(e)
	}
	if snap["printers"].([]Printer)[0].Equipment.Feeds[0].Material != "PA-CF" {
		t.Fatal("redaction mutated driver state")
	}
	if e := s.SavePrinter(u, PrinterInput{ID: p.ID, ModelID: "h2d-pro"}); e != nil {
		t.Fatal(e)
	}
	if _, e := s.DB.Exec("UPDATE spools SET mount_slot='5' WHERE id=?", sp.ID); e != nil {
		t.Fatal(e)
	}
	all, e := readSpools(s.DB)
	if e != nil {
		t.Fatal(e)
	}
	for _, saved := range all {
		if saved.ID == sp.ID && saved.Mount.Slot != "ams:128:0" {
			t.Fatal("legacy HT read lost its physical source")
		}
	}
	duplicate := sp
	duplicate.ID = ""
	duplicate.Mount = &Mount{p.ID, "ams:128:0"}
	if s.SaveSpool(u, duplicate) == nil {
		t.Fatal("legacy and new HT aliases occupied the same physical source twice")
	}
}

func TestSlicePreviewMetadataImageAndPermissions(t *testing.T) {
	s, u, _ := setup(t)
	p, sp := fixture(t, s, u)
	var img bytes.Buffer
	png.Encode(&img, image.NewRGBA(image.Rect(0, 0, 8, 8)))
	entries := map[string]string{"Metadata/project_settings.config": `{"printer_model":"Bambu Lab P1S"}`, "Metadata/plate_1.gcode": "; simulated only", "Metadata/slice_info.config": `<config><plate><metadata key="index" value="1"/><metadata key="prediction" value="3700"/><filament id="1" type="PLA" color="#eeeeee" used_g="12.1"/></plate></config>`, "Metadata/plate_1.png": img.String()}
	raw := sliceFixture(t, entries)
	file := filepath.Join(s.Config.DataDir, "uploads", "preview.3mf")
	os.WriteFile(file, raw, 0600)
	hash := sha256.Sum256(raw)
	id, e := s.CreateJob(u, Schedule{Printer: p.ID, Estimate: 65, Grams: 13, SpoolID: sp.ID}, "preview.3mf", file, hex.EncodeToString(hash[:]), "", int64(len(raw)))
	if e != nil {
		t.Fatal(e)
	}
	info, e := s.JobSlice(u, id)
	if e != nil || info.Plates[0].Seconds != 3700 || !info.Plates[0].Thumbnail {
		t.Fatal(info, e)
	}
	if info.Plates[0].Filaments[0].UsedGrams != 12.1 || info.Plates[0].Filaments[0].Grams != 13 {
		t.Fatal("preview precision must not reduce inventory reservation", info)
	}
	got, e := s.SliceThumbnail(u, id, 1)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = png.Decode(bytes.NewReader(got)); e != nil {
		t.Fatal(e)
	}
	if _, e = s.SliceThumbnail(User{ID: 999}, id, 1); e == nil {
		t.Fatal("other user read original file")
	}
	if _, e = s.SliceThumbnail(u, id, 2); e == nil {
		t.Fatal("nonexistent plate returned")
	}
	entries["Metadata/plate_1.png"] = "<svg onload='bad()'/>"
	os.WriteFile(file, sliceFixture(t, entries), 0600)
	if _, e = s.SliceThumbnail(u, id, 1); e == nil {
		t.Fatal("non-PNG passed")
	}
}

func TestAdvancedSupplyRegistrationDoesNotEnableUnverifiedStart(t *testing.T) {
	for _, scenario := range []string{"dual", "extra AMS"} {
		t.Run(scenario, func(t *testing.T) {
			s, u, _ := setup(t)
			p, sp := fixture(t, s, u)
			if scenario == "dual" {
				if e := s.SavePrinter(u, PrinterInput{ID: p.ID, ModelID: "h2d"}); e != nil {
					t.Fatal(e)
				}
			} else {
				sp.Mount = &Mount{p.ID, "ams:1:0"}
				if e := s.SaveSpool(u, sp); e != nil {
					t.Fatal(e)
				}
			}
			id := newJob(t, s, u, p, sp, 12)
			if e := s.SendJob(u, id); e != nil {
				t.Fatal(e)
			}
			waitState(t, s, id, "assigned")
			if s.StartJob(u, id) == nil {
				t.Fatal("registration enabled an unverified dispatch path")
			}
			var state string
			s.DB.QueryRow("SELECT state FROM jobs WHERE id=?", id).Scan(&state)
			if state != "assigned" {
				t.Fatal("rejected start changed task state")
			}
		})
	}
}

func TestEquipmentExplicitRejectionAndAuthorizationGate(t *testing.T) {
	s, u, d := setup(t)
	p, _ := fixture(t, s, u)
	f := &equipmentFake{fakeDevice: d, reject: true}
	s.Device = f
	settingFixture(d, p.ID, "idle", "")
	if err := s.SetEquipment(u, SettingInput{PrinterID: p.ID, Key: "nozzle", Value: 0}); err != nil {
		t.Fatal(err)
	}
	waitCommand(t, s, "rejected")
	d.mu.Lock()
	state := d.statuses[p.ID]
	state.Equipment.ControlIssue = "authorization_required"
	state.Equipment.Settings = append(state.Equipment.Settings, SettingReading{Key: "chamber_light", Value: 1, Min: 0, Max: 1, Updated: time.Now()})
	d.statuses[p.ID] = state
	d.mu.Unlock()
	if err := s.SetEquipment(u, SettingInput{PrinterID: p.ID, Key: "nozzle", Value: 200}); err == nil {
		t.Fatal("authorization failure permitted heating request")
	}
	f.reject = false
	if err := s.SetEquipment(u, SettingInput{PrinterID: p.ID, Key: "chamber_light", Value: 1}); err != nil {
		t.Fatal("authorization notice blocked working lamp", err)
	}
	waitCommand(t, s, "succeeded")
}

func TestPreparationMetadataDoesNotEnableDispatch(t *testing.T) {
	entries := map[string]string{
		"Metadata/project_settings.config": `{"printer_model":"Bambu Lab A1 mini","curr_bed_type":"Textured PEI Plate","nozzle_diameter":["0.4","NaN","3"]}`,
		"Metadata/plate_1.gcode":           "; TEST DATA ONLY",
		"Metadata/slice_info.config":       `<config><plate><metadata key="index" value="1"/><metadata key="prediction" value="6929"/><filament id="1" type="PLA" color="#FFFFFF" used_g="38.57"/><warning msg="not_support_traditional_timelapse"/><warning msg="not_support_traditional_timelapse"/><nozzle extruder_id="1"/></plate></config>`,
	}
	path := filepath.Join(t.TempDir(), "preparation.3mf")
	if err := os.WriteFile(path, sliceFixture(t, entries), 0600); err != nil {
		t.Fatal(err)
	}
	info, err := inspectSlice(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Model != "Bambu Lab A1 mini" || info.BedType != "Textured PEI Plate" || len(info.NozzleDiameters) != 1 || info.NozzleDiameters[0] != "0.4" {
		t.Fatal(info)
	}
	plate := info.Plates[0]
	if plate.MultiNozzle || plate.Seconds != 6929 || len(plate.Filaments) != 1 || plate.Filaments[0].UsedGrams != 38.57 || plate.Filaments[0].Grams != 39 || len(plate.Warnings) != 1 {
		t.Fatal(plate)
	}
	s, u, _ := setup(t)
	printer, spool := fixture(t, s, u)
	raw := sliceFixture(t, entries)
	file := filepath.Join(s.Config.DataDir, "uploads", "different-model.3mf")
	if err := os.WriteFile(file, raw, 0600); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(raw)
	id, err := s.CreateJob(u, Schedule{Printer: printer.ID, Estimate: 116, Grams: 39, SpoolID: spool.ID}, "different-model.3mf", file, hex.EncodeToString(hash[:]), "", int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	err = s.SavePrintOptions(u, id, PrintOptions{Plate: 1, UseAMS: true, Mapping: []AMSTarget{{Filament: 0, AMSID: 0, SlotID: 0, SpoolID: spool.ID, Grams: 39}}})
	if err == nil {
		t.Fatal("preview allowed an A1 mini file to be prepared for another model")
	}
}

func TestX2DPreparationWithoutPhysicalMapStillCannotDispatch(t *testing.T) {
	s, u, driver := setup(t)
	p, sp := fixture(t, s, u)
	if err := s.SavePrinter(u, PrinterInput{ID: p.ID, ModelID: "x2d"}); err != nil {
		t.Fatal(err)
	}
	sp.Mount = &Mount{PrinterID: p.ID, Slot: "1"}
	if err := s.SaveSpool(u, sp); err != nil {
		t.Fatal(err)
	}
	entries := map[string]string{
		"Metadata/project_settings.config": `{"printer_model":"Bambu Lab X2D","nozzle_diameter":["0.4","0.4"]}`,
		"Metadata/plate_1.gcode":           "; TEST DATA ONLY",
		"Metadata/slice_info.config":       `<config><plate><metadata key="index" value="1"/><filament id="1" type="PLA" color="#FFFFFF" used_g="38.49"/><nozzle id="0" extruder_id="1"/></plate></config>`,
	}
	raw := sliceFixture(t, entries)
	file := filepath.Join(s.Config.DataDir, "uploads", "x2d-preparation.3mf")
	if err := os.WriteFile(file, raw, 0600); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(raw)
	id, err := s.CreateJob(u, Schedule{Printer: p.ID, Estimate: 99, Grams: 39, SpoolID: sp.ID}, "x2d-preparation.3mf", file, hex.EncodeToString(hash[:]), "", int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	o := PrintOptions{Plate: 1, UseAMS: true, Mapping: []AMSTarget{{Filament: 0, AMSID: 0, SlotID: 0, SpoolID: sp.ID, Grams: 39}}}
	if err = s.SendJob(u, id); err == nil || !strings.Contains(err.Error(), "供料准备") {
		t.Fatal("an unprepared X2D file bypassed the dispatch gate", err)
	}
	if err = s.SavePrintOptions(User{ID: 999}, id, o); err == nil {
		t.Fatal("another member could edit the preparation")
	}
	o.Mapping[0].Grams = 38
	if err = s.SavePrintOptions(u, id, o); err == nil {
		t.Fatal("preparation reserved less than slice use")
	}
	o.Mapping[0].Grams = 39
	o.Mapping[0].SlotID = 1
	if err = s.SavePrintOptions(u, id, o); err == nil {
		t.Fatal("preparation accepted a different mount")
	}
	o.Mapping[0].SlotID = 0
	if err = s.SavePrintOptions(u, id, o); err != nil {
		t.Fatal("valid single-nozzle preparation was blocked by dispatch capability", err)
	}
	var state, options string
	if err = s.DB.QueryRow("SELECT state,print_options FROM jobs WHERE id=?", id).Scan(&state, &options); err != nil || state != "queued" || options == "" {
		t.Fatal(state, options, err)
	}
	if err = s.SendJob(u, id); err == nil || !strings.Contains(err.Error(), "物理喷嘴映射") {
		t.Fatal("saving preparation enabled file transfer", err)
	}
	driver.mu.Lock()
	uploads := driver.uploads
	driver.mu.Unlock()
	if uploads != 0 {
		t.Fatal("blocked preparation reached device upload")
	}
	if _, err = s.DB.Exec("UPDATE jobs SET state='assigned' WHERE id=?", id); err != nil {
		t.Fatal(err)
	}
	if err = s.StartJob(u, id); err == nil || !strings.Contains(err.Error(), "物理喷嘴映射") {
		t.Fatal("saving preparation enabled print start", err)
	}
	var commands int
	s.DB.QueryRow("SELECT COUNT(*) FROM printer_commands").Scan(&commands)
	if commands != 0 || driver.Status(p.ID).State != "idle" {
		t.Fatal("preparation sent a physical command")
	}
}
