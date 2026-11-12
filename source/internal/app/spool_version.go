package app

import "database/sql"

// Inventory and physical replacement writes invalidate open editors, including
// older workspaces whose implicit first revision has no record_versions row.
func bumpSpoolRevision(tx *sql.Tx, id string) error {
	_, err := tx.Exec("INSERT INTO record_versions(kind,record_id,revision) VALUES('spool',?,2) ON CONFLICT(kind,record_id) DO UPDATE SET revision=record_versions.revision+1", id)
	return err
}
