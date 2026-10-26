package app

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDispatchTransfersAndStartsExactlyOnce(t *testing.T) {
	s, u, d := setup(t)
	p, sp := fixture(t, s, u)
	id := newJob(t, s, u, p, sp, 20)
	next := newJob(t, s, u, p, sp, 20)
	if err := s.DispatchJob(u, next); err == nil {
		t.Fatal("bypassed queue")
	}
	if err := s.DispatchJob(User{ID: 999}, id); err == nil {
		t.Fatal("bypassed owner")
	}
	d.hold = make(chan struct{})
	var accepted atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if s.DispatchJob(u, id) == nil {
				accepted.Add(1)
			}
		}()
	}
	wg.Wait()
	close(d.hold)
	waitState(t, s, id, "printing")
	var starts int
	s.DB.QueryRow("SELECT COUNT(*) FROM printer_commands WHERE action='start'").Scan(&starts)
	if accepted.Load() != 1 || d.uploads != 1 || starts != 1 {
		t.Fatal("duplicate dispatch", accepted.Load(), d.uploads, starts)
	}
	if err := s.DispatchJob(u, id); err == nil {
		t.Fatal("replayed dispatch")
	}
	var state string
	s.DB.QueryRow("SELECT state FROM jobs WHERE id=?", next).Scan(&state)
	if state != "queued" {
		t.Fatal("automatically advanced queue")
	}
}

func TestDispatchRechecksAfterTransfer(t *testing.T) {
	for _, which := range []string{"offline", "busy", "disabled user", "changed preparation", "pending command", "cancelled service"} {
		t.Run(which, func(t *testing.T) {
			s, u, d := setup(t)
			p, sp := fixture(t, s, u)
			id := newJob(t, s, u, p, sp, 20)
			d.hold = make(chan struct{})
			if err := s.DispatchJob(u, id); err != nil {
				t.Fatal(err)
			}
			switch which {
			case "offline":
				d.mu.Lock()
				d.statuses[p.ID] = Telemetry{}
				d.mu.Unlock()
			case "busy":
				d.mu.Lock()
				d.statuses[p.ID] = Telemetry{Connected: true, Updated: time.Now(), State: "printing"}
				d.mu.Unlock()
			case "disabled user":
				s.DB.Exec("UPDATE users SET disabled=1 WHERE id=?", u.ID)
			case "changed preparation":
				s.DB.Exec("UPDATE jobs SET estimate=estimate+1 WHERE id=?", id)
			case "pending command":
				s.DB.Exec("INSERT INTO printer_commands(printer_id,job_id,user_id,action,state,created_at) VALUES(?,?,?,'start','uncertain',0)", p.ID, id, u.ID)
			case "cancelled service":
				s.cancel()
			}
			close(d.hold)
			s.workers.Wait()
			var starts int
			s.DB.QueryRow("SELECT COUNT(*) FROM printer_commands WHERE state<>'uncertain'").Scan(&starts)
			var state string
			s.DB.QueryRow("SELECT state FROM jobs WHERE id=?", id).Scan(&state)
			if starts != 0 || state != "assigned" && !(which == "cancelled service" && state == "uploading") {
				t.Fatal("unsafe continuation", starts, state)
			}
		})
	}
}

func TestDispatchPreflightAndUncertainStart(t *testing.T) {
	s, u, d := setup(t)
	p, sp := fixture(t, s, u)
	id := newJob(t, s, u, p, sp, 20)
	s.Config.EnablePrintStart = false
	if s.DispatchJob(u, id) == nil || d.uploads != 0 {
		t.Fatal("disabled start uploaded")
	}
	s.Config.EnablePrintStart = true
	s.DB.Exec("UPDATE jobs SET filename='project.3mf' WHERE id=?", id)
	if s.DispatchJob(u, id) == nil || d.uploads != 0 {
		t.Fatal("unprepared project uploaded")
	}
	s.DB.Exec("UPDATE jobs SET filename='file.gcode' WHERE id=?", id)
	d.startErr = true
	if err := s.DispatchJob(u, id); err != nil {
		t.Fatal(err)
	}
	s.workers.Wait()
	var state string
	s.DB.QueryRow("SELECT state FROM printer_commands WHERE job_id=?", id).Scan(&state)
	if state != "uncertain" || s.DispatchJob(u, id) == nil {
		t.Fatal("uncertain start repeated", state)
	}
}

type failedUpload struct{ *fakeDevice }

func (d failedUpload) Upload(context.Context, Printer, string, string, func(int64)) error {
	return errors.New("upload failed")
}
func TestDispatchFailedTransferNeverStarts(t *testing.T) {
	s, u, d := setup(t)
	p, sp := fixture(t, s, u)
	id := newJob(t, s, u, p, sp, 20)
	s.Device = failedUpload{d}
	if err := s.DispatchJob(u, id); err != nil {
		t.Fatal(err)
	}
	s.workers.Wait()
	var n int
	s.DB.QueryRow("SELECT COUNT(*) FROM printer_commands").Scan(&n)
	if n != 0 {
		t.Fatal("start followed failed upload")
	}
	waitState(t, s, id, "queued")
}

func TestManualCollectionCannotDelayOrDuplicateFinishedMail(t *testing.T) {
	for _, collectFirst := range []bool{false, true} {
		t.Run(fmt.Sprint(collectFirst), func(t *testing.T) {
			s, u, d := setup(t)
			p, sp := fixture(t, s, u)
			id := newJob(t, s, u, p, sp, 20)
			bound(t, s, u, "owner@example.com")
			s.DB.Exec("UPDATE jobs SET state='printing',remote_path='unique.3mf' WHERE id=?", id)
			now := time.Now()
			d.statuses[p.ID] = Telemetry{Connected: true, Updated: now, StateUpdated: now, TaskUpdated: now, State: "idle", RawState: "FINISH", File: fmt.Sprintf("job-%d", id)}
			if collectFirst {
				if err := s.collectMail(now); err != nil {
					t.Fatal(err)
				}
			}
			if err := s.CompleteJob(u, id, 18); err != nil {
				t.Fatal(err)
			}
			if err := s.collectMail(now); err != nil {
				t.Fatal(err)
			}
			if err := s.collectMail(now); err != nil {
				t.Fatal(err)
			}
			var n int
			s.DB.QueryRow("SELECT COUNT(*) FROM mail_outbox WHERE kind='notification'").Scan(&n)
			if n != 1 {
				t.Fatal("completion mail count", n)
			}
			var subject string
			s.DB.QueryRow("SELECT title FROM user_notifications WHERE event_key=?", fmt.Sprintf("job:%d:device_finished", id)).Scan(&subject)
			if subject != "LayerHub · 打印已完成 · same-name.gcode" {
				t.Fatal(subject)
			}
		})
	}
}
