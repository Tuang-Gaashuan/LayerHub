package app

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type controlFake struct {
	*fakeDevice
	holdControl  chan struct{}
	controlError bool
}

func (f *controlFake) StartPrint(ctx context.Context, p Printer, r PrintRequest) error {
	return f.Start(ctx, p, r.Remote)
}
func (f *controlFake) Control(ctx context.Context, p Printer, action string) error {
	if f.holdControl != nil {
		select {
		case <-f.holdControl:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if f.controlError {
		return context.DeadlineExceeded
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	v := f.statuses[p.ID]
	v.State = map[string]string{"pause": "pause", "resume": "printing", "stop": "idle"}[action]
	v.Updated = time.Now()
	f.statuses[p.ID] = v
	return nil
}
func TestControlOwnershipSinglePendingAndConsumption(t *testing.T) {
	s, u, d := setup(t)
	p, sp := fixture(t, s, u)
	driver := &controlFake{fakeDevice: d}
	s.Device = driver
	id := newJob(t, s, u, p, sp, 20)
	if e := s.SendJob(u, id); e != nil {
		t.Fatal(e)
	}
	waitState(t, s, id, "assigned")
	if e := s.StartJob(u, id); e != nil {
		t.Fatal(e)
	}
	waitState(t, s, id, "printing")
	in := ControlInput{PrinterID: p.ID, JobID: id, Action: "pause"}
	if s.ControlPrinter(User{ID: 999}, in) == nil {
		t.Fatal("other user controlled printer")
	}
	driver.holdControl = make(chan struct{})
	if e := s.ControlPrinter(u, in); e != nil {
		t.Fatal(e)
	}
	if s.ControlPrinter(u, in) == nil {
		t.Fatal("duplicate control accepted")
	}
	if s.CompleteJob(u, id, 5) == nil {
		t.Fatal("completed during pending command")
	}
	close(driver.holdControl)
	waitState(t, s, id, "paused")
	if e := s.SaveSpool(u, sp); e == nil {
		t.Fatal("paused spool allowed to unload")
	}
	driver.holdControl = nil
	in.Action = "resume"
	if e := s.ControlPrinter(u, in); e != nil {
		t.Fatal(e)
	}
	waitState(t, s, id, "printing")
	in.Action = "stop"
	if e := s.ControlPrinter(u, in); e != nil {
		t.Fatal(e)
	}
	waitState(t, s, id, "stopped")
	jobs, _ := readJobs(s.DB)
	if reserve(jobs, sp.ID, 0) != 20 {
		t.Fatal("stop released consumption reservation too early")
	}
	if e := s.CompleteJob(u, id, 7); e != nil {
		t.Fatal(e)
	}
	if s.CompleteJob(u, id, 7) == nil {
		t.Fatal("deducted twice")
	}
	var left int
	s.DB.QueryRow("SELECT remaining FROM spools WHERE id=?", sp.ID).Scan(&left)
	if left != 993 {
		t.Fatal(left)
	}
	history, e := s.CommandHistory(u, p.ID)
	if e != nil || len(history) != 4 || history[0].State != "succeeded" {
		t.Fatalf("audit: %+v %v", history, e)
	}
}
func TestUncertainControlRequiresReviewAndMatchingTask(t *testing.T) {
	s, u, d := setup(t)
	p, sp := fixture(t, s, u)
	driver := &controlFake{fakeDevice: d, controlError: true}
	s.Device = driver
	id := newJob(t, s, u, p, sp, 20)
	s.SendJob(u, id)
	waitState(t, s, id, "assigned")
	s.StartJob(u, id)
	waitState(t, s, id, "printing")
	in := ControlInput{p.ID, id, "pause"}
	if e := s.ControlPrinter(u, in); e != nil {
		t.Fatal(e)
	}
	until := time.Now().Add(time.Second)
	for time.Now().Before(until) {
		rows, _ := s.CommandHistory(u, p.ID)
		if rows[0].State == "uncertain" {
			break
		}
		time.Sleep(time.Millisecond * 5)
	}
	if s.ControlPrinter(u, in) == nil {
		t.Fatal("uncertain command retried")
	}
	if s.CompleteJob(u, id, 1) == nil {
		t.Fatal("uncertain completion")
	}
	d.mu.Lock()
	v := d.statuses[p.ID]
	original := v.File
	v.File = "foreign.gcode"
	d.statuses[p.ID] = v
	d.mu.Unlock()
	in.Action = "resolve"
	if s.ControlPrinter(u, in) == nil {
		t.Fatal("unrelated print reconciled")
	}
	d.mu.Lock()
	v.File = original
	d.statuses[p.ID] = v
	d.mu.Unlock()
	if e := s.ControlPrinter(u, in); e != nil {
		t.Fatal(e)
	}
}
func sliceFixture(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	for name, value := range entries {
		w, e := z.Create(name)
		if e != nil {
			t.Fatal(e)
		}
		w.Write([]byte(value))
	}
	if e := z.Close(); e != nil {
		t.Fatal(e)
	}
	return buf.Bytes()
}
func TestSliceMappingReservesEverySpoolAndDeductsOnce(t *testing.T) {
	s, u, _ := setup(t)
	p, sp := fixture(t, s, u)
	sp.Mount = &Mount{p.ID, "1"}
	if e := s.SaveSpool(u, sp); e != nil {
		t.Fatal(e)
	}
	blue := sp
	blue.ID = ""
	blue.Label = "蓝色 PLA"
	blue.Color = "#3159ed"
	blue.Mount = &Mount{p.ID, "2"}
	if e := s.SaveSpool(u, blue); e != nil {
		t.Fatal(e)
	}
	spools, _ := readSpools(s.DB)
	blue = spools[1]
	raw := sliceFixture(t, map[string]string{"Metadata/project_settings.config": `{"printer_model":"Bambu Lab P1S"}`, "Metadata/plate_2.gcode": "; sliced test only\nG28\n", "Metadata/slice_info.config": `<config><plate><metadata key="index" value="2"/><filament id="1" type="PLA" color="#eeeeee" used_g="12.1"/><filament id="3" type="PLA" color="#3159ed" used_g="6.2"/></plate></config>`})
	file := filepath.Join(s.Config.DataDir, "uploads", "sample.3mf")
	os.WriteFile(file, raw, 0600)
	hash := sha256.Sum256(raw)
	id, e := s.CreateJob(u, Schedule{Printer: p.ID, Estimate: 20, Grams: 20, SpoolID: sp.ID}, "sample.gcode.3mf", file, hex.EncodeToString(hash[:]), "", int64(len(raw)))
	if e != nil {
		t.Fatal(e)
	}
	opts := PrintOptions{Plate: 2, UseAMS: true, Mapping: []AMSTarget{{0, 0, 0, sp.ID, 13}, {2, 0, 1, blue.ID, 7}}, BedLeveling: true}
	if e = s.SavePrintOptions(u, id, opts); e != nil {
		t.Fatal(e)
	}
	jobs, _ := readJobs(s.DB)
	if reserve(jobs, sp.ID, 0) != 13 || reserve(jobs, blue.ID, 0) != 7 {
		t.Fatal("mapping reservation lost")
	}
	blue.Remaining = 6
	if s.SaveSpool(u, blue) == nil {
		t.Fatal("secondary reservation overspent")
	}
	opts.Mapping[1].SlotID = 0
	if s.SavePrintOptions(u, id, opts) == nil {
		t.Fatal("wrong physical mount accepted")
	}
	opts.Mapping[1].SlotID = 1
	opts.Plate = 1
	if s.SavePrintOptions(u, id, opts) == nil {
		t.Fatal("nonexistent plate accepted")
	}
	s.DB.Exec("UPDATE jobs SET state='stopped' WHERE id=?", id)
	if s.CompleteJob(u, id, 18) == nil {
		t.Fatal("multi-spool single total accepted")
	}
	if e = s.CompleteJobUses(u, id, 0, map[string]int{sp.ID: 11, blue.ID: 6}); e != nil {
		t.Fatal(e)
	}
	if s.CompleteJobUses(u, id, 0, map[string]int{sp.ID: 11, blue.ID: 6}) == nil {
		t.Fatal("duplicate multi-spool completion")
	}
	all, _ := readSpools(s.DB)
	if all[0].Remaining != 989 || all[1].Remaining != 994 {
		t.Fatal("incorrect multi-spool deduction")
	}
}
func TestSliceRejectsUnslicedUnsafeAndMismatched(t *testing.T) {
	for name, entries := range map[string]map[string]string{"unsliced": {"3D/3dmodel.model": "<model/>"}, "traversal": {"../escape": "x"}, "missing plate": {"Metadata/plate_1.gcode": "x", "Metadata/slice_info.config": `<config><plate><metadata key="index" value="2"/></plate></config>`}} {
		t.Run(name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "x.3mf")
			os.WriteFile(file, sliceFixture(t, entries), 0600)
			if _, e := inspectSlice(file); e == nil {
				t.Fatal("invalid 3mf accepted")
			}
		})
	}
}

func TestVersion2BackupRestoresWithControlMigration(t *testing.T) {
	s, u, _ := setup(t)
	p, sp := fixture(t, s, u)
	newJob(t, s, u, p, sp, 10)
	if _, e := s.DB.Exec("DROP TABLE job_consumption; DROP TABLE printer_commands; ALTER TABLE jobs DROP COLUMN print_options; PRAGMA user_version=2;"); e != nil {
		t.Fatal(e)
	}
	archive, cleanup, e := s.ExportBackup(u, "TestPassword123")
	if e != nil {
		t.Fatal(e)
	}
	defer cleanup()
	f, e := os.Open(archive)
	if e != nil {
		t.Fatal(e)
	}
	_, ticket, e := s.stageRestore(u, f)
	f.Close()
	if e != nil {
		t.Fatal(e)
	}
	if e = s.confirmRestore(u, ticket, "TestPassword123"); e != nil {
		t.Fatal(e)
	}
	cfg := s.Config
	s.Close()
	restored, e := Open(cfg, &fakeDevice{})
	if e != nil {
		t.Fatal(e)
	}
	defer restored.Close()
	jobs, e := readJobs(restored.DB)
	if e != nil || len(jobs) != 1 || jobs[0].PrintOptions != nil {
		t.Fatalf("legacy jobs lost: %+v %v", jobs, e)
	}
	var version int
	restored.DB.QueryRow("PRAGMA user_version").Scan(&version)
	if version != 5 {
		t.Fatal("legacy backup was not migrated")
	}
}
