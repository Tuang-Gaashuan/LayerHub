package app

import "database/sql"

// CancelJob only archives a task which has not started. It never sends a device
// command, deletes a file, settles material, or advances the queue.
func (s *Service) CancelJob(u User, id int64) error {
	return s.transaction(func(tx *sql.Tx) error {
		jobs, err := readJobs(tx)
		if err != nil {
			return err
		}
		for _, j := range jobs {
			if j.ID != id {
				continue
			}
			if j.UserID != u.ID && !u.Admin {
				return forbidden("无权取消其他用户的任务")
			}
			if j.State == "cancelled" {
				return nil
			}
			if j.State != "queued" && j.State != "assigned" && j.State != "failed" {
				return conflict("任务已经执行，请在设备控制台核对或停止打印")
			}
			var unresolved int
			if err = tx.QueryRow("SELECT count(*) FROM printer_commands WHERE job_id=? AND state IN ('pending','uncertain')", id).Scan(&unresolved); err != nil {
				return err
			}
			// Any historical start means a physical result must be reconciled first.
			var starts int
			if err = tx.QueryRow("SELECT count(*) FROM printer_commands WHERE job_id=? AND action='start'", id).Scan(&starts); err != nil {
				return err
			}
			if unresolved > 0 || starts > 0 || j.Error != "" {
				return conflict("任务仍有待核对的执行结果，不能取消")
			}
			_, err = tx.Exec("UPDATE jobs SET state='cancelled' WHERE id=?", id)
			return err
		}
		return missing("任务不存在")
	})
}
