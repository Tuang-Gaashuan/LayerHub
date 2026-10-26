package app

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

const usabilitySchema = `
CREATE TABLE IF NOT EXISTS notification_preferences(user_id INTEGER PRIMARY KEY REFERENCES users(id),started INTEGER NOT NULL DEFAULT 1,completed INTEGER NOT NULL DEFAULT 1,attention INTEGER NOT NULL DEFAULT 1,devices INTEGER NOT NULL DEFAULT 1,stock INTEGER NOT NULL DEFAULT 1,revision INTEGER NOT NULL DEFAULT 1);
CREATE TABLE IF NOT EXISTS user_notifications(id INTEGER PRIMARY KEY AUTOINCREMENT,user_id INTEGER NOT NULL REFERENCES users(id),event_key TEXT NOT NULL,category TEXT NOT NULL,title TEXT NOT NULL,body TEXT NOT NULL,entity_type TEXT NOT NULL DEFAULT '',entity_id TEXT NOT NULL DEFAULT '',created INTEGER NOT NULL,read_at INTEGER NOT NULL DEFAULT 0,UNIQUE(user_id,event_key));
CREATE INDEX IF NOT EXISTS notifications_user_page ON user_notifications(user_id,id DESC);
CREATE TABLE IF NOT EXISTS spool_alert_settings(spool_id TEXT PRIMARY KEY REFERENCES spools(id) ON DELETE CASCADE,threshold INTEGER NOT NULL CHECK(threshold>=0 AND threshold<=10000),revision INTEGER NOT NULL DEFAULT 1);
CREATE TABLE IF NOT EXISTS record_versions(kind TEXT NOT NULL,record_id TEXT NOT NULL,revision INTEGER NOT NULL,PRIMARY KEY(kind,record_id));
`

type NotificationPreferences struct {
	Started   bool  `json:"started"`
	Completed bool  `json:"completed"`
	Attention bool  `json:"attention"`
	Devices   bool  `json:"devices"`
	Stock     bool  `json:"stock"`
	Revision  int64 `json:"revision"`
}

func readNotificationPreferences(q queryer, uid int64) (NotificationPreferences, error) {
	p := NotificationPreferences{Started: true, Completed: true, Attention: true, Devices: true, Stock: true}
	err := q.QueryRow("SELECT started,completed,attention,devices,stock,revision FROM notification_preferences WHERE user_id=?", uid).Scan(&p.Started, &p.Completed, &p.Attention, &p.Devices, &p.Stock, &p.Revision)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	return p, err
}
func (p NotificationPreferences) allows(category string) bool {
	switch category {
	case "started":
		return p.Started
	case "completed":
		return p.Completed
	case "devices":
		return p.Devices
	case "stock":
		return p.Stock
	default:
		return p.Attention
	}
}
func (s *Service) saveNotificationPreferences(u User, p NotificationPreferences) error {
	return s.transaction(func(tx *sql.Tx) error {
		old, err := readNotificationPreferences(tx, u.ID)
		if err != nil {
			return err
		}
		if p.Revision != old.Revision {
			return conflict("通知偏好已在其他窗口修改，请重新读取后再保存")
		}
		if _, err = tx.Exec(`INSERT INTO notification_preferences(user_id,started,completed,attention,devices,stock,revision) VALUES(?,?,?,?,?,?,?) ON CONFLICT(user_id) DO UPDATE SET started=excluded.started,completed=excluded.completed,attention=excluded.attention,devices=excluded.devices,stock=excluded.stock,revision=excluded.revision`, u.ID, p.Started, p.Completed, p.Attention, p.Devices, p.Stock, old.Revision+1); err != nil {
			return err
		}
		_, err = tx.Exec(`UPDATE mail_outbox SET state='cancelled',body=X'' WHERE user_id=? AND kind='notification' AND state IN ('queued','leased') AND event_key IN (SELECT event_key||':user:'||user_id FROM user_notifications WHERE user_id=? AND ((category='started' AND ?=0) OR (category='completed' AND ?=0) OR (category='attention' AND ?=0) OR (category='devices' AND ?=0) OR (category='stock' AND ?=0)))`, u.ID, u.ID, p.Started, p.Completed, p.Attention, p.Devices, p.Stock)
		return err
	})
}

// Re-check current spool visibility even for past notices if a shared roll was
// later made private. Public snapshots and mail status never contain addresses.
const visibleNotification = `(n.entity_type<>'spool' OR EXISTS(SELECT 1 FROM spools s WHERE s.id=n.entity_id AND (s.owner_id IS NULL OR s.owner_id=n.user_id OR s.shared=1)))`

type Notification struct {
	ID         int64  `json:"id"`
	Category   string `json:"category"`
	Title      string `json:"title"`
	Body       string `json:"body"`
	EntityType string `json:"entityType"`
	EntityID   string `json:"entityId"`
	Created    int64  `json:"created"`
	Read       bool   `json:"read"`
	Delivery   string `json:"delivery"`
}
type MailDelivery struct {
	Kind     string `json:"kind"`
	State    string `json:"state"`
	Created  int64  `json:"created"`
	Attempts int    `json:"attempts"`
	NextAt   int64  `json:"nextAt"`
}

func (s *Service) notificationSummary(u User) (map[string]any, error) {
	var unread, latest int64
	err := s.DB.QueryRow("SELECT COALESCE(SUM(CASE WHEN n.read_at=0 THEN 1 ELSE 0 END),0),COALESCE(MAX(n.id),0) FROM user_notifications n WHERE n.user_id=? AND "+visibleNotification, u.ID).Scan(&unread, &latest)
	return map[string]any{"unread": unread, "latestId": latest}, err
}
func (s *Service) notificationPage(u User, before int64, unreadOnly bool) (any, error) {
	rows, err := s.DB.Query(`SELECT n.id,n.category,n.title,n.body,n.entity_type,n.entity_id,n.created,n.read_at<>0,COALESCE(m.state,'none') FROM user_notifications n LEFT JOIN mail_outbox m ON m.event_key=n.event_key||':user:'||n.user_id AND m.user_id=n.user_id WHERE n.user_id=? AND (?=0 OR n.id<?) AND (?=0 OR n.read_at=0) AND `+visibleNotification+` ORDER BY n.id DESC LIMIT 31`, u.ID, before, before, unreadOnly)
	if err != nil {
		return nil, err
	}
	notices := []Notification{}
	for rows.Next() {
		var n Notification
		if err = rows.Scan(&n.ID, &n.Category, &n.Title, &n.Body, &n.EntityType, &n.EntityID, &n.Created, &n.Read, &n.Delivery); err != nil {
			rows.Close()
			return nil, err
		}
		notices = append(notices, n)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	next := int64(0)
	if len(notices) > 30 {
		notices = notices[:30]
		next = notices[29].ID
	}
	deliveries := []MailDelivery{}
	rows, err = s.DB.Query("SELECT kind,state,created,attempts,next_at FROM mail_outbox WHERE user_id=? ORDER BY created DESC,id DESC LIMIT 30", u.ID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var d MailDelivery
		if err = rows.Scan(&d.Kind, &d.State, &d.Created, &d.Attempts, &d.NextAt); err != nil {
			rows.Close()
			return nil, err
		}
		deliveries = append(deliveries, d)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	summary, err := s.notificationSummary(u)
	if err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "notifications": notices, "deliveries": deliveries, "nextBefore": next, "summary": summary}, nil
}
func (s *Service) readNotifications(u User, id, through int64) error {
	if id <= 0 && through <= 0 {
		return invalid("请选择通知或先读取通知列表")
	}
	return s.transaction(func(tx *sql.Tx) error {
		if id > 0 {
			_, err := tx.Exec("UPDATE user_notifications SET read_at=? WHERE user_id=? AND id=? AND read_at=0", time.Now().Unix(), u.ID, id)
			return err
		}
		_, err := tx.Exec("UPDATE user_notifications SET read_at=? WHERE user_id=? AND id<=? AND read_at=0", time.Now().Unix(), u.ID, through)
		return err
	})
}
func notificationCategory(event, title string) string {
	if strings.HasSuffix(event, ":started") && strings.HasPrefix(event, "job:") {
		return "started"
	}
	if strings.HasPrefix(event, "printer:") {
		return "devices"
	}
	if strings.HasPrefix(event, "stock:") {
		return "stock"
	}
	if strings.HasSuffix(event, ":device_finished") || strings.Contains(title, "任务已确认完成") || strings.Contains(title, "设备报告打印结束") {
		return "completed"
	}
	return "attention"
}
func (s *Service) saveStockThreshold(u User, id string, threshold int, revision int64) error {
	return s.transaction(func(tx *sql.Tx) error {
		spools, err := readSpools(tx)
		if err != nil {
			return err
		}
		var spool *Spool
		for i := range spools {
			if spools[i].ID == id {
				spool = &spools[i]
				break
			}
		}
		if spool == nil {
			return missing("耗材不存在")
		}
		if !spoolEditable(*spool, u) {
			return forbidden("只能为本人耗材或管理员维护的公共耗材设置提醒")
		}
		if threshold < 0 || threshold > spool.Capacity {
			return invalid("提醒阈值应在 0 与标称净重之间，0 表示关闭")
		}
		if revision != spool.AlertRevision {
			return conflict("提醒设置已在其他窗口修改，请刷新后重试")
		}
		if _, err = tx.Exec("INSERT INTO spool_alert_settings(spool_id,threshold,revision) VALUES(?,?,?) ON CONFLICT(spool_id) DO UPDATE SET threshold=excluded.threshold,revision=excluded.revision", id, threshold, revision+1); err != nil {
			return err
		}
		if threshold == 0 {
			if _, err = tx.Exec(`UPDATE mail_outbox SET state='cancelled',body=X'' WHERE kind='notification' AND state IN ('queued','leased') AND event_key IN (SELECT event_key||':user:'||user_id FROM user_notifications WHERE category='stock' AND entity_type='spool' AND entity_id=?)`, id); err != nil {
				return err
			}
		}
		// Explicit configuration may produce one alert when the current roll is low.
		_, err = tx.Exec("INSERT OR IGNORE INTO mail_observations(key,value,candidate,since) VALUES(?,'ok','ok',?)", "stock:"+id, time.Now().Unix())
		return err
	})
}
func (s *Service) observeStock(tx *sql.Tx, now int64, jobs []Job) error {
	spools, err := readSpools(tx)
	if err != nil {
		return err
	}
	for _, sp := range spools {
		reserved := reserve(jobs, sp.ID, 0)
		available := max(0, sp.Remaining-reserved)
		state := "ok"
		if sp.LowThreshold <= 0 {
			state = "off"
		} else if available <= sp.LowThreshold {
			state = "low"
		}
		seq, changed, err := observeMail(tx, "stock:"+sp.ID, state, now, 0)
		if err != nil {
			return err
		}
		if !changed || state != "low" {
			continue
		}
		ids := []int64{}
		if sp.OwnerID != 0 {
			ids = append(ids, sp.OwnerID)
		} else {
			rows, err := tx.Query("SELECT id FROM users WHERE admin=1 AND disabled=0")
			if err != nil {
				return err
			}
			for rows.Next() {
				var uid int64
				if err = rows.Scan(&uid); err != nil {
					rows.Close()
					return err
				}
				ids = append(ids, uid)
			}
			err = rows.Err()
			rows.Close()
			if err != nil {
				return err
			}
		}
		for _, uid := range ids {
			if err = s.notifyUser(tx, uid, fmt.Sprintf("stock:%s:%d", sp.ID, seq), "LayerHub · 耗材可分配余量偏低", fmt.Sprintf("耗材：%s\n人工登记净余量：%d g\n任务预留：%d g\n当前可分配：%d g\n提醒阈值：%d g\n请核对库存，需要时补充或调整任务安排。", sp.Label, sp.Remaining, reserved, available, sp.LowThreshold), now); err != nil {
				return err
			}
		}
	}
	return nil
}
