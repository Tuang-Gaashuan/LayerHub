package app

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

type motionFake struct {
	*fakeDevice
	calls  atomic.Int32
	hold   chan struct{}
	reject bool
}

func (f *motionFake) Move(ctx context.Context, p Printer, in MotionInput) error {
	f.calls.Add(1)
	if f.hold != nil {
		select {
		case <-f.hold:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if f.reject {
		return ErrDeviceRejected
	}
	return nil
}
func motionFixture(d *fakeDevice, pid int64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	mask := 7
	now := time.Now()
	d.statuses[pid] = Telemetry{Connected: true, RawState: "IDLE", State: "idle", Updated: now, StateUpdated: now, Equipment: Equipment{Motion: MotionReading{NativeHome: true, NativeJog: true, HomeMask: &mask, HomeAt: now, CapabilitiesAt: now}}}
}
func TestMotionAdminFreshnessReplayAndSharedLock(t *testing.T) {
	s, u, d := setup(t)
	p, _ := fixture(t, s, u)
	s.DB.Exec("UPDATE printers SET model_id='x2d' WHERE id=?", p.ID)
	motionFixture(d, p.ID)
	f := &motionFake{fakeDevice: d, hold: make(chan struct{})}
	s.Device = f
	in := MotionInput{PrinterID: p.ID, Action: "jog", Axis: "X", Direction: 1, Step: 1, RequestID: "motion-request-001", Clear: true}
	if s.MovePrinter(User{ID: 888}, in) == nil {
		t.Fatal("non-admin motion")
	}
	bad := in
	bad.Step = 10
	if s.MovePrinter(u, bad) == nil {
		t.Fatal("untested 10mm")
	}
	bad = in
	bad.Axis = "E"
	if s.MovePrinter(u, bad) == nil {
		t.Fatal("extrusion permitted")
	}
	if e := s.MovePrinter(u, in); e != nil {
		t.Fatal(e)
	}
	if e := s.MovePrinter(u, in); e != nil {
		t.Fatal("replay rejected", e)
	}
	bad = in
	bad.RequestID = "motion-request-002"
	if s.MovePrinter(u, bad) == nil {
		t.Fatal("concurrent motion")
	}
	bad = in
	bad.Direction = -1
	if s.MovePrinter(u, bad) == nil {
		t.Fatal("altered receipt replay")
	}
	close(f.hold)
	waitCommand(t, s, "uncertain")
	if f.calls.Load() != 1 {
		t.Fatal("duplicate send")
	}
	var id int64
	s.DB.QueryRow("SELECT id FROM printer_commands WHERE action='motion:jog'").Scan(&id)
	review := MotionInput{PrinterID: p.ID, Action: "review", CommandID: id, Clear: true, RequestID: "motion-review-001"}
	if e := s.MovePrinter(u, review); e != nil {
		t.Fatal(e)
	}
	waitCommand(t, s, "reviewed")
	if e := s.MovePrinter(u, in); e != nil {
		t.Fatal(e)
	}
	if f.calls.Load() != 1 {
		t.Fatal("review caused resend")
	}
	// Every dispatch path also checks this same table (see executable/validateStart).
}
func TestMotionStateGatesAndNoBusinessMutation(t *testing.T) {
	s, u, d := setup(t)
	p, _ := fixture(t, s, u)
	s.DB.Exec("UPDATE printers SET model_id='x2d' WHERE id=?", p.ID)
	f := &motionFake{fakeDevice: d}
	s.Device = f
	in := MotionInput{PrinterID: p.ID, Action: "home", RequestID: "motion-home-0001", Clear: true}
	for _, raw := range []string{"RUNNING", "PAUSE", "FAILED", "UNKNOWN"} {
		motionFixture(d, p.ID)
		d.mu.Lock()
		v := d.statuses[p.ID]
		v.RawState = raw
		d.statuses[p.ID] = v
		d.mu.Unlock()
		if s.MovePrinter(u, in) == nil {
			t.Fatal("unsafe state", raw)
		}
	}
	motionFixture(d, p.ID)
	d.mu.Lock()
	v := d.statuses[p.ID]
	v.StateUpdated = time.Now().Add(-time.Minute)
	d.statuses[p.ID] = v
	d.mu.Unlock()
	if s.MovePrinter(u, in) == nil {
		t.Fatal("stale state")
	}
	motionFixture(d, p.ID)
	f.reject = true
	if e := s.MovePrinter(u, in); e != nil {
		t.Fatal(e)
	}
	waitCommand(t, s, "rejected")
	var jobs, consumption int
	s.DB.QueryRow("SELECT count(*) FROM jobs").Scan(&jobs)
	s.DB.QueryRow("SELECT count(*) FROM job_consumption").Scan(&consumption)
	if jobs != 0 || consumption != 0 {
		t.Fatal("motion touched business data")
	}
}
