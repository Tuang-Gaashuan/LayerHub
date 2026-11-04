package app

import "database/sql"

// DeleteSpool removes only an unused, unmounted record. History is never rewritten.
func (s *Service) DeleteSpool(actor User, id string, revision int64) error {
	return s.transaction(func(tx *sql.Tx) error {
		fresh, err := currentUser(tx, actor.ID)
		if err != nil {
			return err
		}
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
		if !spoolEditable(*spool, fresh) {
			return forbidden("只能删除本人耗材，公共耗材由管理员维护")
		}
		if revision < 1 || revision != spool.Revision {
			return conflict("耗材记录已变化，请重新打开后核对")
		}
		if spool.Mount != nil {
			return conflict("耗材仍有挂载登记，请先确认实物并解除挂载")
		}
		jobs, err := readJobs(tx)
		if err != nil {
			return err
		}
		for _, j := range jobs {
			_, used := jobUses(j)[id]
			if used || j.SpoolID == id {
				return conflict("耗材已有任务关联，不能删除；历史用量记录需要保留")
			}
		}
		var count int
		err = tx.QueryRow(`SELECT (SELECT COUNT(*) FROM job_consumption WHERE spool_id=?) + (SELECT COUNT(*) FROM job_usage_notes WHERE spool_id=?) + (SELECT COUNT(*) FROM orphan_completions o WHERE EXISTS (SELECT 1 FROM json_each(o.uses_json) WHERE key=?))`, id, id, id).Scan(&count)
		if err != nil {
			return err
		}
		if count > 0 {
			return conflict("耗材已有用量记录，不能删除")
		}
		// Unsent reminders for a removed record are no longer relevant.
		if _, err = tx.Exec(`UPDATE mail_outbox SET state='cancelled',body=X'' WHERE kind='notification' AND state IN ('queued','leased') AND event_key IN (SELECT event_key||':user:'||user_id FROM user_notifications WHERE entity_type='spool' AND entity_id=?)`, id); err != nil {
			return err
		}
		_, err = tx.Exec("DELETE FROM spools WHERE id=?", id)
		return err
	})
}
