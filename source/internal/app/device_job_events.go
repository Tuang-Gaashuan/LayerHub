package app

import (
	"database/sql"
	"fmt"
)

// A physical stop is an end of execution, not a successful print or a stock
// settlement. A cleared/foreign task name requires explicit human review.
func deviceEndNeedsReview(j Job, t Telemetry) bool {
	return (j.State == "printing" || j.State == "paused") && j.FinishedAt == 0 &&
		t.Connected && timeFresh(t.Updated) && timeFresh(t.StateUpdated) &&
		(t.RawState == "FAILED" && t.State == "failed" || t.RawState == "IDLE" && t.State == "idle")
}

// FAILED may be a latched cancellation result. It is ready for another task
// only after the matching previous job's collection and usage were confirmed,
// with fresh telemetry and no current alarm or execution. Preserve RawState.
func settledStopReady(jobs []Job, pid int64, t Telemetry) bool {
	if t.RawState != "FAILED" || t.State != "failed" || t.PrintError != 0 || t.Error != "" || len(t.Equipment.HMS) != 0 || t.Equipment.ControlIssue != "" || t.Equipment.Switching {
		return false
	}
	matched := false
	for _, j := range jobs {
		if j.Printer != pid {
			continue
		}
		switch j.State {
		case "uploading", "starting", "printing", "paused", "stopped", "checking":
			return false
		}
		if j.State == "cancelled" && j.CollectedAt > 0 && j.SettledAt > 0 && mailTaskMatches(j, t) {
			matched = true
		}
	}
	return matched
}

func (s *Service) observeDeviceStopped(tx *sql.Tx, j Job, t Telemetry, now int64) (Job, error) {
	if !deviceEndNeedsReview(j, t) || !mailTaskMatches(j, t) {
		return j, nil
	}
	var pending int
	if err := tx.QueryRow("SELECT COUNT(*) FROM printer_commands WHERE printer_id=? AND state IN ('pending','uncertain')", j.Printer).Scan(&pending); err != nil {
		return j, err
	}
	if pending != 0 {
		return j, nil
	}
	r, err := tx.Exec("UPDATE jobs SET state='stopped',error='' WHERE id=? AND state IN ('printing','paused')", j.ID)
	if err != nil {
		return j, err
	}
	n, err := r.RowsAffected()
	if err != nil || n == 0 {
		return j, err
	}
	_, err = tx.Exec("INSERT OR IGNORE INTO mail_observations(key,value,candidate,since) VALUES(?,?,?,?)", fmt.Sprintf("job:%d:device_stopped", j.ID), t.RawState, t.RawState, now)
	if err != nil {
		return j, err
	}
	j.State, j.DisplayState, j.Error = "stopped", "stopped", ""
	return j, nil
}
