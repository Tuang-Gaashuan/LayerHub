package app

import (
	"testing"
	"time"
)

func TestDeleteUnconfirmedJobAndNoIdentityReuse(t *testing.T) {
	s, u, d := setup(t)
	p, sp := fixture(t, s, u)
	id := newJob(t, s, u, p, sp, 10)
	s.DB.Exec("UPDATE jobs SET state='assigned' WHERE id=?", id)
	s.DB.Exec("INSERT INTO printer_commands(printer_id,job_id,user_id,action,state,created_at) VALUES(?,?,?,'start','uncertain',?)", p.ID, id, u.ID, time.Now().UnixMilli())
	if s.DeleteJob(u, id) == nil {
		t.Fatal("unconfirmed job deleted")
	}
	// Historical corruption fixture; production DeleteJob must prevent this.
	s.DB.Exec("DELETE FROM jobs WHERE id=?", id)
	d.mu.Lock()
	d.statuses[p.ID] = Telemetry{Connected: true, Updated: time.Now(), StateUpdated: time.Now(), TaskUpdated: time.Now(), RawState: "RUNNING", State: "printing", File: "job-1"}
	d.mu.Unlock()
	var cid int64
	s.DB.QueryRow("SELECT id FROM printer_commands WHERE job_id=?", id).Scan(&cid)
	in := OrphanReview{PrinterID: p.ID, CommandID: cid, ConfirmTask: "job-1", Confirmed: true}
	if s.ReviewOrphan(User{ID: 888}, in) == nil {
		t.Fatal("nonadmin orphan review")
	}
	bad := in
	bad.ConfirmTask = "job-2"
	if s.ReviewOrphan(u, bad) == nil {
		t.Fatal("wrong device identity")
	}
	if e := s.ReviewOrphan(u, in); e != nil {
		t.Fatal(e)
	}
	if e := s.ReviewOrphan(u, in); e != nil {
		t.Fatal("review replay", e)
	}
	var n int
	s.DB.QueryRow("SELECT count(*) FROM jobs").Scan(&n)
	if n != 0 {
		t.Fatal("invented job")
	}
	s.DB.QueryRow("SELECT count(*) FROM job_consumption").Scan(&n)
	if n != 0 {
		t.Fatal("invented usage")
	}
	d.mu.Lock()
	d.statuses[p.ID] = Telemetry{Connected: true, Updated: time.Now(), State: "idle"}
	d.mu.Unlock()
	next := newJob(t, s, u, p, sp, 10)
	if next <= id {
		t.Fatal("orphan identity reused", next)
	}
}
