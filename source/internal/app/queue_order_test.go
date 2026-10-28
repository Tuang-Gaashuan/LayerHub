package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestQueueNewOrderControlsTransferButNeverStartsNextAutomatically(t *testing.T) {
	s, u, d := setup(t)
	p, sp := fixture(t, s, u)
	a := newJob(t, s, u, p, sp, 10)
	b := newJob(t, s, u, p, sp, 10)
	if s.SendJob(u, b) == nil {
		t.Fatal("non-head transferred")
	}
	if err := s.MoveQueueJob(u, QueueMove{p.ID, b, "first", queueExpectation(t, s, p.ID)}); err != nil {
		t.Fatal(err)
	}
	if s.SendJob(u, a) == nil {
		t.Fatal("old head transferred after reorder")
	}
	if d.uploads != 0 {
		t.Fatal("reordering triggered transfer")
	}
	if err := s.SendJob(u, b); err != nil {
		t.Fatal(err)
	}
	waitState(t, s, b, "assigned")
	if s.SendJob(u, a) == nil {
		t.Fatal("prepared task was bypassed")
	}
	if d.uploads != 1 || d.Status(p.ID).State != "idle" {
		t.Fatal("transfer started a print")
	}
}

func TestQueueReorderHTTPRequiresSessionCSRFAndCurrentSnapshot(t *testing.T) {
	s, u, _ := setup(t)
	p, sp := fixture(t, s, u)
	newJob(t, s, u, p, sp, 10)
	b := newJob(t, s, u, p, sp, 10)
	token := securityLogin(t, s, "admin", "TestPassword123", "Queue test")
	body, _ := json.Marshal(QueueMove{p.ID, b, "first", queueExpectation(t, s, p.ID)})
	call := func(auth, csrf bool) int {
		r := httptest.NewRequest(http.MethodPost, "/api/jobs/reorder", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if auth {
			r.AddCookie(&http.Cookie{Name: "layerhub_sid", Value: token})
		}
		if csrf {
			r.Header.Set("X-CSRF-Token", s.csrf(token))
		}
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		return w.Code
	}
	if got := call(false, false); got != 401 {
		t.Fatal(got)
	}
	if got := call(true, false); got != 403 {
		t.Fatal(got)
	}
	if got := call(true, true); got != 200 {
		t.Fatal(got)
	}
	if got := call(true, true); got != 409 {
		t.Fatal("stale replay", got)
	}
}

func queueExpectation(t *testing.T, s *Service, pid int64) []QueuePosition {
	t.Helper()
	jobs, err := readJobs(s.DB)
	if err != nil {
		t.Fatal(err)
	}
	var positions []QueuePosition
	for _, j := range jobs {
		if j.Printer == pid && activeStates[j.State] {
			positions = append(positions, QueuePosition{j.ID, j.Order, j.State})
		}
	}
	return positions
}

func queuedIDs(t *testing.T, s *Service, pid int64) []int64 {
	t.Helper()
	var ids []int64
	for _, p := range queueExpectation(t, s, pid) {
		if p.State == "queued" {
			ids = append(ids, p.ID)
		}
	}
	return ids
}

func TestQueueMovesPreserveRunningJobsReservationsAndPreparation(t *testing.T) {
	s, u, d := setup(t)
	p, sp := fixture(t, s, u)
	running := newJob(t, s, u, p, sp, 10)
	a := newJob(t, s, u, p, sp, 11)
	b := newJob(t, s, u, p, sp, 12)
	c := newJob(t, s, u, p, sp, 13)
	if err := s.SavePrinter(u, PrinterInput{Name: "A1", IP: "192.0.2.2", Serial: "TEST-SECOND", ModelID: "a1", Configuration: "bare", Code: "test-device-code"}); err != nil {
		t.Fatal(err)
	}
	printers, _ := s.printers()
	other := printers[len(printers)-1]
	newJob(t, s, u, other, sp, 5)
	otherBefore := fmt.Sprint(queueExpectation(t, s, other.ID))
	s.DB.Exec("UPDATE jobs SET state='printing' WHERE id=?", running)
	s.DB.Exec("UPDATE jobs SET print_options=? WHERE id=?", `{"plate":7,"mapping":[]}`, c)
	before, _ := readJobs(s.DB)
	original := queueExpectation(t, s, p.ID)[0]
	for _, action := range []struct {
		id   int64
		move string
		want []int64
	}{{c, "first", []int64{c, a, b}}, {c, "down", []int64{a, c, b}}, {b, "up", []int64{a, b, c}}} {
		if err := s.MoveQueueJob(u, QueueMove{p.ID, action.id, action.move, queueExpectation(t, s, p.ID)}); err != nil {
			t.Fatal(err)
		}
		if got := queuedIDs(t, s, p.ID); fmt.Sprint(got) != fmt.Sprint(action.want) {
			t.Fatal(got, action.want)
		}
	}
	if queueExpectation(t, s, p.ID)[0] != original {
		t.Fatal("executing task moved")
	}
	if fmt.Sprint(queueExpectation(t, s, other.ID)) != otherBefore {
		t.Fatal("another printer's order changed")
	}
	after, _ := readJobs(s.DB)
	if reserve(before, sp.ID, 0) != reserve(after, sp.ID, 0) {
		t.Fatal("reservation changed")
	}
	for _, j := range after {
		if j.ID == c && (j.PrintOptions == nil || j.PrintOptions.Plate != 7) {
			t.Fatal("preparation lost")
		}
	}
	for _, table := range []string{"printer_commands", "job_consumption", "mail_outbox"} {
		var n int
		s.DB.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n)
		if n != 0 {
			t.Fatal(table, n)
		}
	}
	if d.uploads != 0 {
		t.Fatal("reorder sent a file")
	}
}

func TestQueueMovesRejectStaleReplayAndConcurrentChanges(t *testing.T) {
	s, u, _ := setup(t)
	p, sp := fixture(t, s, u)
	newJob(t, s, u, p, sp, 10)
	b := newJob(t, s, u, p, sp, 10)
	move := QueueMove{p.ID, b, "first", queueExpectation(t, s, p.ID)}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); results <- s.MoveQueueJob(u, move) }()
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatal("duplicate/stale move accepted", success)
	}
	current := QueueMove{p.ID, b, "down", queueExpectation(t, s, p.ID)}
	s.DB.Exec("UPDATE jobs SET state='assigned' WHERE id=?", b)
	if s.MoveQueueJob(u, current) == nil {
		t.Fatal("concurrent transfer accepted")
	}
	if s.MoveQueueJob(u, QueueMove{p.ID, b, "down", queueExpectation(t, s, p.ID)}) == nil {
		t.Fatal("prepared task moved")
	}
}

func TestQueueMembersCannotOvertakeOthersOrAnotherPrinter(t *testing.T) {
	s, u, _ := setup(t)
	p, sp := fixture(t, s, u)
	if err := s.SaveUser(u, "member", "Member", "MemberPassword123", false); err != nil {
		t.Fatal(err)
	}
	var member User
	if err := s.DB.QueryRow("SELECT id,username,name,admin FROM users WHERE username='member'").Scan(&member.ID, &member.Username, &member.Name, &member.Admin); err != nil {
		t.Fatal(err)
	}
	a := newJob(t, s, u, p, sp, 10)
	b := newJob(t, s, member, p, sp, 10)
	c := newJob(t, s, member, p, sp, 10)
	if err := s.MoveQueueJob(member, QueueMove{p.ID, c, "up", queueExpectation(t, s, p.ID)}); err != nil {
		t.Fatal(err)
	}
	if s.MoveQueueJob(member, QueueMove{p.ID, c, "first", queueExpectation(t, s, p.ID)}) == nil {
		t.Fatal("member overtook another owner")
	}
	if s.MoveQueueJob(member, QueueMove{p.ID, a, "down", queueExpectation(t, s, p.ID)}) == nil {
		t.Fatal("member moved another owner's task")
	}
	if s.MoveQueueJob(u, QueueMove{p.ID + 1, b, "first", queueExpectation(t, s, p.ID)}) == nil {
		t.Fatal("wrong printer accepted")
	}
	if s.MoveQueueJob(u, QueueMove{0, b, "first", queueExpectation(t, s, p.ID)}) == nil {
		t.Fatal("unassigned queue accepted")
	}
	if s.MoveQueueJob(u, QueueMove{p.ID, b, "sideways", queueExpectation(t, s, p.ID)}) == nil {
		t.Fatal("invalid direction accepted")
	}
}
