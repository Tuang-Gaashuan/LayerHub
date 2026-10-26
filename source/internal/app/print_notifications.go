package app

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

func printFinishedMatches(j Job, t Telemetry) bool {
	return (j.State == "printing" || j.State == "paused") && t.RawState == "FINISH" && t.State == "idle" && mailTaskMatches(j, t)
}

func (s *Service) observePrintFinished(tx *sql.Tx, j Job, t Telemetry, now int64) error {
	if !printFinishedMatches(j, t) {
		return nil
	}
	if err := recordPrintCompletion(tx, j, now); err != nil {
		return err
	}
	var previous string
	err := tx.QueryRow("SELECT value FROM mail_observations WHERE key=?", fmt.Sprintf("job:%d", j.ID)).Scan(&previous)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	return s.notifyPrintMilestone(tx, j, "device_finished", "打印已完成", "打印机已报告此任务打印完成。可以前往取件；稍后在 LayerHub 确认取件与实际耗材用量。完成通知自动发送，无需先人工确认，库存不会自动扣减。", now, previous == "device_finished")
}

// A plate's Metadata/plate_1.gcode is shared by many archives. It must never
// identify a job on its own. X2D reports the unique job name in subtask_name.
func mailTaskMatches(j Job, t Telemetry) bool {
	if j.Remote == "" || !t.Connected || !timeFresh(t.Updated) || !timeFresh(t.StateUpdated) || !timeFresh(t.TaskUpdated) {
		return false
	}
	name := fmt.Sprintf("job-%d", j.ID)
	if t.File != "" {
		return sameTaskName(t.File, name) || sameTaskName(t.File, j.Remote)
	}
	return sameTaskName(t.GcodeFile, j.Remote)
}

func (s *Service) notifyPrintMilestone(tx *sql.Tx, j Job, phase, label, detail string, now int64, alreadySent bool) error {
	event := fmt.Sprintf("job:%d:%s", j.ID, phase)
	r, err := tx.Exec("INSERT OR IGNORE INTO mail_observations(key,value,candidate,since) VALUES(?,'observed','observed',?)", event, now)
	if err != nil {
		return err
	}
	n, err := r.RowsAffected()
	if err != nil || n == 0 || alreadySent {
		return err
	}
	var printer string
	if err := tx.QueryRow("SELECT name FROM printers WHERE id=?", j.Printer).Scan(&printer); err != nil {
		return err
	}
	// Keep mail subjects single-line even if a previously registered device or
	// filename contains line breaks. Bodies retain the useful task context.
	singleLine := func(v string) string { return strings.Join(strings.Fields(v), " ") }
	subject := fmt.Sprintf("LayerHub · %s · %s", label, singleLine(j.Filename))
	body := fmt.Sprintf("%s\n设备：%s\n任务：%s\n任务编号：%d\n\n%s", label, printer, j.Filename, j.ID, detail)
	return s.notifyUser(tx, j.UserID, event, subject, body, now)
}
