package app

import (
	"database/sql"
	"math"
	"time"
)

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// Additive tables leave the version-3 integer inventory ledger readable by
// older releases. Reported precision and the source are retained separately.
const settlementSchema = `
CREATE TABLE IF NOT EXISTS job_completion(job_id INTEGER PRIMARY KEY REFERENCES jobs(id) ON DELETE CASCADE,finished_at INTEGER NOT NULL DEFAULT 0,collected_at INTEGER NOT NULL DEFAULT 0,settled_at INTEGER NOT NULL DEFAULT 0,settled_by INTEGER REFERENCES users(id));
CREATE TABLE IF NOT EXISTS job_usage_notes(job_id INTEGER NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,spool_id TEXT NOT NULL REFERENCES spools(id),source TEXT NOT NULL,reported_mg INTEGER NOT NULL CHECK(reported_mg>=0),PRIMARY KEY(job_id,spool_id));
INSERT OR IGNORE INTO job_completion(job_id,finished_at)
 SELECT j.id,o.since FROM jobs j JOIN mail_observations o ON o.key='job:'||j.id||':device_finished';
`

type SettlementRoll struct {
	SpoolID   string   `json:"spoolId"`
	Label     string   `json:"label"`
	Color     string   `json:"color"`
	Reserved  int      `json:"reserved"`
	Available int      `json:"available"`
	Estimate  *float64 `json:"estimate"`
	Source    string   `json:"source,omitempty"`
	Reported  *float64 `json:"reported,omitempty"`
	Booked    *int     `json:"booked,omitempty"`
}
type Settlement struct {
	Job   Job              `json:"job"`
	Rolls []SettlementRoll `json:"rolls"`
}
type SettlementUse struct {
	Source string  `json:"source"`
	Grams  float64 `json:"grams"`
}
type SettlementWrite struct {
	JobID     int64                    `json:"jobId"`
	Collected bool                     `json:"collected"`
	Uses      map[string]SettlementUse `json:"uses"`
}

func settlementJob(q queryer, u User, id int64) (Job, []Job, error) {
	jobs, err := readJobs(q)
	if err != nil {
		return Job{}, nil, err
	}
	for _, j := range jobs {
		if j.ID == id {
			if j.UserID != u.ID && !u.Admin {
				return Job{}, nil, forbidden("无权查看或结算其他用户的任务")
			}
			return j, jobs, nil
		}
	}
	return Job{}, nil, missing("任务不存在")
}

func settlementEstimates(j Job) map[string]float64 {
	result := map[string]float64{}
	info, err := inspectSlice(j.Path)
	if err != nil {
		return result
	}
	plate := 1
	if j.PrintOptions != nil {
		plate = j.PrintOptions.Plate
	}
	for _, p := range info.Plates {
		if p.ID != plate {
			continue
		}
		for _, f := range p.Filaments {
			id := j.SpoolID
			if j.PrintOptions != nil {
				id = ""
				for _, m := range j.PrintOptions.Mapping {
					if m.Filament == f.Index {
						id = m.SpoolID
					}
				}
			}
			if id != "" && finite(f.UsedGrams) && f.UsedGrams > 0 {
				result[id] += f.UsedGrams
			}
		}
	}
	return result
}

func (s *Service) JobSettlement(u User, id int64) (Settlement, error) {
	var out Settlement
	err := s.transaction(func(tx *sql.Tx) error {
		j, jobs, err := settlementJob(tx, u, id)
		if err != nil {
			return err
		}
		j, err = s.observeDeviceStopped(tx, j, s.Device.Status(j.Printer), time.Now().Unix())
		if err != nil {
			return err
		}
		out.Job, out.Rolls = j, []SettlementRoll{}
		spools, err := readSpools(tx)
		if err != nil {
			return err
		}
		estimates := settlementEstimates(j)
		for _, sp := range spools {
			reserved, ok := jobUses(j)[sp.ID]
			if !ok {
				continue
			}
			r := SettlementRoll{SpoolID: sp.ID, Label: sp.Label, Color: sp.Color, Reserved: reserved, Available: sp.Remaining - reserve(jobs, sp.ID, j.ID)}
			if g, ok := estimates[sp.ID]; ok && j.State != "stopped" {
				r.Estimate = &g
			}
			var mg, booked int
			err = tx.QueryRow("SELECT COALESCE(n.source,'legacy'),COALESCE(n.reported_mg,c.grams*1000),c.grams FROM job_consumption c LEFT JOIN job_usage_notes n USING(job_id,spool_id) WHERE c.job_id=? AND c.spool_id=?", id, sp.ID).Scan(&r.Source, &mg, &booked)
			if err == nil {
				g := float64(mg) / 1000
				r.Reported = &g
				r.Booked = &booked
			} else if err != sql.ErrNoRows {
				return err
			}
			out.Rolls = append(out.Rolls, r)
		}
		return nil
	})
	return out, err
}

func (s *Service) SettleJob(u User, in SettlementWrite) error {
	if !in.Collected {
		return invalid("请先确认已取件并清空打印板")
	}
	return s.transaction(func(tx *sql.Tx) error {
		j, _, err := settlementJob(tx, u, in.JobID)
		if err != nil {
			return err
		}
		if j.State != "stopped" && j.FinishedAt == 0 && !printFinishedMatches(j, s.Device.Status(j.Printer)) {
			j, err = s.observeDeviceStopped(tx, j, s.Device.Status(j.Printer), time.Now().Unix())
			if err != nil {
				return err
			}
		}
		if j.State != "stopped" && j.FinishedAt == 0 && !printFinishedMatches(j, s.Device.Status(j.Printer)) {
			return conflict("尚无此任务的设备完成记录，请先核对设备")
		}
		estimates, actual := settlementEstimates(j), map[string]int{}
		normalized := map[string]SettlementUse{}
		if len(in.Uses) != len(jobUses(j)) {
			return invalid("请登记本任务每卷耗材")
		}
		for id := range jobUses(j) {
			entry, ok := in.Uses[id]
			if !ok {
				return invalid("请登记本任务每卷耗材")
			}
			g := entry.Grams
			switch entry.Source {
			case "estimated":
				var ok bool
				g, ok = estimates[id]
				if !ok || j.State == "stopped" {
					return invalid("此任务没有可用于结算的切片估算，请填写实测用量")
				}
			case "measured":
			default:
				return invalid("请选择估算或实测用量")
			}
			if !finite(g) || g < 0 || g > 100000 || math.Abs(g*1000-math.Round(g*1000)) > .00001 {
				return invalid("用量须为非负数，最多保留三位小数")
			}
			entry.Grams = g
			normalized[id] = entry
			actual[id] = int(math.Ceil(float64(math.Round(g*1000)) / 1000))
		}
		if err = s.completeJobUsesTx(tx, u, in.JobID, 0, actual); err != nil {
			return err
		}
		now := time.Now().Unix()
		if _, err = tx.Exec("INSERT INTO job_completion(job_id,collected_at,settled_at,settled_by) VALUES(?,?,?,?) ON CONFLICT(job_id) DO UPDATE SET collected_at=excluded.collected_at,settled_at=excluded.settled_at,settled_by=excluded.settled_by", j.ID, now, now, u.ID); err != nil {
			return err
		}
		for id, e := range normalized {
			if _, err = tx.Exec("INSERT INTO job_usage_notes(job_id,spool_id,source,reported_mg) VALUES(?,?,?,?)", j.ID, id, e.Source, int64(math.Round(e.Grams*1000))); err != nil {
				return err
			}
		}
		return nil
	})
}

func recordPrintCompletion(tx *sql.Tx, j Job, now int64) error {
	_, err := tx.Exec("INSERT INTO job_completion(job_id,finished_at) VALUES(?,?) ON CONFLICT(job_id) DO UPDATE SET finished_at=CASE WHEN finished_at=0 THEN excluded.finished_at ELSE finished_at END", j.ID, now)
	return err
}
