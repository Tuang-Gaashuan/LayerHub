package app

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func notificationMember(t *testing.T, s *Service, admin User) User {
	t.Helper()
	if err := s.SaveUser(admin, "notice_member", "通知成员", "MemberPassword123", false); err != nil {
		t.Fatal(err)
	}
	var u User
	if err := s.DB.QueryRow("SELECT id,username,name,admin FROM users WHERE username='notice_member'").Scan(&u.ID, &u.Username, &u.Name, &u.Admin); err != nil {
		t.Fatal(err)
	}
	return u
}
func notice(t *testing.T, s *Service, u User, event, title string) {
	t.Helper()
	if err := s.transaction(func(tx *sql.Tx) error {
		return s.notifyUser(tx, u.ID, event, title, "仅本人可见的通知正文", time.Now().Unix())
	}); err != nil {
		t.Fatal(err)
	}
}
func TestStationNotificationsWithoutEmailAndPrivateReadBoundary(t *testing.T) {
	s, u, _ := setup(t)
	other := notificationMember(t, s, u)
	notice(t, s, u, "job:1:1", "LayerHub · 任务已确认完成")
	notice(t, s, other, "job:2:1", "另一成员的任务")
	page, err := s.notificationPage(u, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	result := page.(map[string]any)
	items := result["notifications"].([]Notification)
	if len(items) != 1 || items[0].Delivery != "none" {
		t.Fatal("unbound notification missing")
	}
	var count int
	s.DB.QueryRow("SELECT COUNT(*) FROM mail_outbox").Scan(&count)
	if count != 0 {
		t.Fatal("unbound user queued email")
	}
	boundary := items[0].ID
	notice(t, s, u, "job:3:1", "新到达的任务")
	if err = s.readNotifications(u, 0, boundary); err != nil {
		t.Fatal(err)
	}
	summary, _ := s.notificationSummary(u)
	if summary["unread"].(int64) != 1 {
		t.Fatal("marked newer unread notice")
	}
	var foreignID int64
	s.DB.QueryRow("SELECT id FROM user_notifications WHERE user_id=?", other.ID).Scan(&foreignID)
	if err = s.readNotifications(u, foreignID, 0); err != nil {
		t.Fatal(err)
	}
	summary, _ = s.notificationSummary(other)
	if summary["unread"].(int64) != 1 {
		t.Fatal("read another user's notification")
	}
	if err = s.readNotifications(u, 0, 0); err == nil {
		t.Fatal("unbounded mark all allowed")
	}
	for i := 0; i < 40; i++ {
		notice(t, s, u, fmt.Sprintf("job:100:%d", i), "分页通知")
	}
	page, err = s.notificationPage(u, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	result = page.(map[string]any)
	items = result["notifications"].([]Notification)
	if len(items) != 30 || result["nextBefore"].(int64) == 0 {
		t.Fatal("pagination limit lost")
	}
	next, err := s.notificationPage(u, result["nextBefore"].(int64), false)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range next.(map[string]any)["notifications"].([]Notification) {
		if n.ID >= items[29].ID {
			t.Fatal("cursor duplicated records")
		}
	}
}
func TestNotificationPreferencesCancelPendingButPreserveStationAndCodes(t *testing.T) {
	s, u, _ := setup(t)
	readyMail(t, s)
	bound(t, s, u, "one@example.com")
	notice(t, s, u, "job:1:1", "LayerHub · 任务已确认完成")
	notice(t, s, u, "printer:1:1", "LayerHub · 打印机持续离线")
	p, err := readNotificationPreferences(s.DB, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	p.Completed = false
	p.Devices = false
	if err = s.saveNotificationPreferences(u, p); err != nil {
		t.Fatal(err)
	}
	if err = s.saveNotificationPreferences(u, p); err == nil {
		t.Fatal("stale preferences replaced newer settings")
	}
	m, admins, err := s.claimMail(true)
	if err != nil || m != nil || len(admins) != 0 {
		t.Fatal("excluded categories or offline recipient cache still active", err)
	}
	notice(t, s, u, "job:2:1", "LayerHub · 任务已确认完成")
	summary, _ := s.notificationSummary(u)
	if summary["unread"].(int64) != 3 {
		t.Fatal("email preference removed station notices")
	}
	notice(t, s, u, "job:3:1", "LayerHub · 任务已暂停，请检查")
	m, _, err = s.claimMail(true)
	if err != nil || m == nil || !strings.Contains(m.Subject, "暂停") {
		t.Fatal("allowed category lost", err)
	}
	if err = s.requestEmail(u, "second@example.com"); err != nil {
		t.Fatal(err)
	}
	m, _, err = s.claimMail(true)
	if err != nil || m == nil || !strings.Contains(m.Subject, "验证码") {
		t.Fatal("OTP affected by event preferences", err)
	}
	page, err := s.notificationPage(u, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(page)
	if strings.Contains(string(payload), "example.com") || strings.Contains(string(payload), "校验验证码") {
		t.Fatal("mail log exposed recipient or code")
	}
}
func TestStockThresholdReservationsOwnershipDedupAndRevision(t *testing.T) {
	s, admin, _ := setup(t)
	p, _ := fixture(t, s, admin)
	owner := notificationMember(t, s, admin)
	input := Spool{Label: "成员共享蓝卷", Brand: "Test", Material: "PLA", ColorName: "蓝", Color: "#6688aa", Capacity: 1000, Remaining: 1000, Owner: owner.Username, Shared: true, State: "available"}
	if err := s.SaveSpool(owner, input); err != nil {
		t.Fatal(err)
	}
	all, _ := readSpools(s.DB)
	var sp Spool
	for _, v := range all {
		if v.Label == input.Label {
			sp = v
		}
	}
	if err := s.saveStockThreshold(admin, sp.ID, 100, 0); err == nil {
		t.Fatal("admin edited another member's roll")
	}
	if err := s.saveStockThreshold(owner, sp.ID, 100, 0); err != nil {
		t.Fatal(err)
	}
	if err := s.saveStockThreshold(owner, sp.ID, 100, 0); err == nil {
		t.Fatal("stale threshold accepted")
	}
	if err := s.saveStockThreshold(owner, sp.ID, 1001, 1); err == nil {
		t.Fatal("threshold exceeds roll capacity")
	}
	now := time.Now()
	if err := s.collectMail(now); err != nil {
		t.Fatal(err)
	}
	newJob(t, s, owner, p, sp, 950)
	if err := s.collectMail(now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := s.collectMail(now.Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	page, err := s.notificationPage(owner, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	items := page.(map[string]any)["notifications"].([]Notification)
	if len(items) != 1 || items[0].Category != "stock" || !strings.Contains(items[0].Body, "50 g") {
		t.Fatal("reserved stock not reflected or duplicate notice", items)
	}
	summary, _ := s.notificationSummary(admin)
	if summary["unread"].(int64) != 0 {
		t.Fatal("shared personal roll notified non-owner")
	}
	snapshot, err := s.Snapshot(owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range snapshot["spools"].([]Spool) {
		if v.ID == sp.ID && (v.Remaining != 1000 || v.LowThreshold != 100) {
			t.Fatal("alert modified actual inventory")
		}
	}
}
func TestConcurrentSpoolEditsAndStaleTaskSchedule(t *testing.T) {
	s, u, _ := setup(t)
	p, sp := fixture(t, s, u)
	copies := []Spool{sp, sp}
	copies[0].Label = "修改一"
	copies[1].Label = "修改二"
	results := make(chan error, 2)
	var wait sync.WaitGroup
	for _, copy := range copies {
		wait.Add(1)
		go func(in Spool) { defer wait.Done(); results <- s.SaveSpool(u, in) }(copy)
	}
	wait.Wait()
	close(results)
	accepted, rejected := 0, 0
	for err := range results {
		if err == nil {
			accepted++
		} else {
			rejected++
		}
	}
	if accepted != 1 || rejected != 1 {
		t.Fatal("lost-update protection failed", accepted, rejected)
	}
	all, _ := readSpools(s.DB)
	sp = all[0]
	id := newJob(t, s, u, p, sp, 10)
	jobs, _ := readJobs(s.DB)
	j := jobs[0]
	in := Schedule{JobID: id, Printer: p.ID, Estimate: 30, Grams: 10, SpoolID: sp.ID, Expected: &ScheduleExpectation{Printer: j.Printer, Estimate: j.Estimate, Grams: j.Grams, SpoolID: j.SpoolID, Order: j.Order}}
	if err := s.ScheduleJob(u, in); err != nil {
		t.Fatal(err)
	}
	in.Estimate = 35
	if err := s.ScheduleJob(u, in); err == nil {
		t.Fatal("stale task editor silently overwrote schedule")
	}
}
func TestNotificationRoutesNeedSessionCSRFAndStayUserScoped(t *testing.T) {
	s, u, _ := setup(t)
	other := notificationMember(t, s, u)
	notice(t, s, other, "job:2:1", "不可泄露的通知")
	r := httptest.NewRequest("GET", "/api/notifications", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("anonymous notifications access")
	}
	token := randomID(32)
	s.DB.Exec("INSERT INTO sessions(hash,user_id,expires) VALUES(?,?,?)", tokenHash(token), u.ID, time.Now().Add(time.Hour).Unix())
	r = httptest.NewRequest("GET", "/api/notifications", nil)
	r.AddCookie(&http.Cookie{Name: "layerhub_sid", Value: token})
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 200 || strings.Contains(w.Body.String(), "不可泄露") {
		t.Fatal("notification scope failure", w.Body.String())
	}
	r = httptest.NewRequest("POST", "/api/notifications/read", strings.NewReader(`{"through":999999}`))
	r.AddCookie(&http.Cookie{Name: "layerhub_sid", Value: token})
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("notification write CSRF bypass")
	}
}

func TestNotificationPersistenceAndConcurrentSnapshotReaders(t *testing.T) {
	s, u, _ := setup(t)
	fixture(t, s, u)
	notice(t, s, u, "job:8:1", "重启前的站内通知")
	var wg sync.WaitGroup
	failures := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; n < 35; n++ {
				if _, err := s.Snapshot(u); err != nil {
					failures <- err
					return
				}
			}
		}()
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
	cfg := s.Config
	s.Close()
	next, err := Open(cfg, &fakeDevice{})
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	summary, err := next.notificationSummary(u)
	if err != nil || summary["unread"].(int64) != 1 {
		t.Fatal("restart lost notification state", err)
	}
	if _, err = next.notificationPage(u, 0, false); err != nil {
		t.Fatal(err)
	}
}

func TestMakingSpoolPrivateHidesOldNoticeAndCancelsOtherAdminsMail(t *testing.T) {
	s, u, _ := setup(t)
	p, sp := fixture(t, s, u)
	readyMail(t, s)
	bound(t, s, u, "owner@example.com")
	if err := s.SaveUser(u, "notice_admin", "提醒管理员", "MemberPassword123", true); err != nil {
		t.Fatal(err)
	}
	var other User
	s.DB.QueryRow("SELECT id,username,name,admin FROM users WHERE username='notice_admin'").Scan(&other.ID, &other.Username, &other.Name, &other.Admin)
	bound(t, s, other, "other@example.com")
	if err := s.saveStockThreshold(u, sp.ID, 900, 0); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := s.collectMail(now); err != nil {
		t.Fatal(err)
	}
	newJob(t, s, u, p, sp, 200)
	if err := s.collectMail(now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	summary, _ := s.notificationSummary(other)
	if summary["unread"].(int64) != 1 {
		t.Fatal("public roll did not notify subscribed admin")
	}
	sp.Owner = u.Username
	sp.Shared = false
	if err := s.SaveSpool(u, sp); err != nil {
		t.Fatal(err)
	}
	summary, _ = s.notificationSummary(other)
	if summary["unread"].(int64) != 0 {
		t.Fatal("private roll notice leaked")
	}
	page, err := s.notificationPage(other, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.(map[string]any)["notifications"].([]Notification)) != 0 {
		t.Fatal("private notification history visible")
	}
	var state string
	s.DB.QueryRow("SELECT state FROM mail_outbox WHERE user_id=?", other.ID).Scan(&state)
	if state != "cancelled" {
		t.Fatal("private roll mail not revoked", state)
	}
}
