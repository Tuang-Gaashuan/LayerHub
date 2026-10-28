package app

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"strings"
	"time"
)

const importBatchSchema = `
CREATE TABLE IF NOT EXISTS job_plate_selection(job_id INTEGER PRIMARY KEY REFERENCES jobs(id) ON DELETE CASCADE,plate INTEGER NOT NULL CHECK(plate>0));
CREATE TABLE IF NOT EXISTS job_import_batches(user_id INTEGER NOT NULL REFERENCES users(id),request_id TEXT NOT NULL,input_hash TEXT NOT NULL,job_ids TEXT NOT NULL,created INTEGER NOT NULL,PRIMARY KEY(user_id,request_id));
`

type ImportBatch struct {
	RequestID string     `json:"requestId"`
	Items     []Schedule `json:"items"`
}

type ImportReceipt struct {
	RequestID string  `json:"requestId"`
	JobIDs    []int64 `json:"jobIds"`
	Replayed  bool    `json:"replayed"`
}

var importRequestID = regexp.MustCompile(`^[a-zA-Z0-9-]{16,80}$`)

func recordSelectedPlate(tx *sql.Tx, id int64, plate int) error {
	_, err := tx.Exec("INSERT INTO job_plate_selection(job_id,plate) VALUES(?,?) ON CONFLICT(job_id) DO UPDATE SET plate=excluded.plate", id, plate)
	return err
}

func selectedImportPlate(info SliceInfo, requested int) (SlicePlate, error) {
	if requested == 0 && len(info.Plates) == 1 {
		return info.Plates[0], nil
	}
	for _, p := range info.Plates {
		if p.ID == requested {
			return p, nil
		}
	}
	return SlicePlate{}, invalid("请明确选择文件中已切片的打印盘")
}

func readImportReceipt(q queryer, uid int64, requestID string) (ImportReceipt, string, error) {
	out := ImportReceipt{RequestID: requestID}
	var fingerprint, ids string
	err := q.QueryRow("SELECT input_hash,job_ids FROM job_import_batches WHERE user_id=? AND request_id=?", uid, requestID).Scan(&fingerprint, &ids)
	if err != nil {
		return out, "", err
	}
	if err = json.Unmarshal([]byte(ids), &out.JobIDs); err != nil {
		return out, "", err
	}
	return out, fingerprint, nil
}

func (s *Service) ImportReceipt(u User, requestID string) (map[string]any, error) {
	if !importRequestID.MatchString(requestID) {
		return nil, invalid("导入核对编号无效")
	}
	r, _, err := readImportReceipt(s.DB, u.ID, requestID)
	if err == sql.ErrNoRows {
		return map[string]any{"ok": true, "found": false}, nil
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "found": true, "receipt": r}, nil
}

// Each selected single-filament plate gets its own reservation. Nothing here
// uploads to a printer or starts it; the original archive is shared immutably.
func (s *Service) CreateImportBatch(u User, in ImportBatch, filename, path, digest, note string, size int64) (ImportReceipt, error) {
	out := ImportReceipt{RequestID: in.RequestID}
	if !importRequestID.MatchString(in.RequestID) || len(in.Items) < 1 || len(in.Items) > 16 || len(digest) != 64 || size < 1 {
		return out, invalid("一次请选择 1 至 16 个打印盘，并使用有效导入编号")
	}
	info, err := inspectSlice(path)
	if err != nil {
		return out, err
	}
	seen := map[int]bool{}
	for _, item := range in.Items {
		plate, e := selectedImportPlate(info, item.Plate)
		if e != nil || item.Plate <= 0 || seen[item.Plate] {
			return out, invalid("打印盘缺失、重复或不在此文件中")
		}
		seen[item.Plate] = true
		if plate.MultiNozzle || len(plate.Filaments) != 1 {
			return out, invalid("批量入队仅接受单耗材盘；多耗材盘请单独创建并完成供料准备")
		}
		if item.Grams < plate.Filaments[0].Grams {
			return out, invalid("预留用量不能低于所选盘的切片用量")
		}
	}
	input, _ := json.Marshal(struct {
		Items          []Schedule
		Filename, Hash string
		Note           string
	}{in.Items, filename, digest, trim(note, 200)})
	h := sha256.Sum256(input)
	fingerprint := hex.EncodeToString(h[:])
	err = s.transaction(func(tx *sql.Tx) error {
		prior, oldHash, e := readImportReceipt(tx, u.ID, in.RequestID)
		if e == nil {
			if oldHash != fingerprint {
				return conflict("该导入编号已用于其他安排，请先核对原批次")
			}
			out = prior
			out.Replayed = true
			return nil
		}
		if e != sql.ErrNoRows {
			return e
		}
		spools, e := readSpools(tx)
		if e != nil {
			return e
		}
		printers, e := readPrinters(tx)
		if e != nil {
			return e
		}
		var order int64
		if e = tx.QueryRow("SELECT COALESCE(MAX(queue_order),0) FROM jobs").Scan(&order); e != nil {
			return e
		}
		for _, item := range in.Items {
			if e = validateSchedule(tx, u, item, 0); e != nil {
				return e
			}
			plate, _ := selectedImportPlate(info, item.Plate)
			for _, sp := range spools {
				if sp.ID == item.SpoolID && !strings.EqualFold(sp.Material, plate.Filaments[0].Material) {
					return conflict("所选料卷材质与打印盘不同，请重新选择或重新切片")
				}
			}
			for _, p := range printers {
				if p.ID == item.Printer && (info.Model == "" || modelKey(info.Model) != modelKey(models[p.ModelID])) {
					return conflict("切片机型无法确认或与目标设备不同，请为目标机型重新切片")
				}
			}
			order++
			var pid any
			if item.Printer != 0 {
				pid = item.Printer
			}
			newID, e := nextJobID(tx)
			if e != nil {
				return e
			}
			r, e := tx.Exec("INSERT INTO jobs(id,user_id,filename,path,size,sha256,printer_id,state,note,created_at,queue_order,estimate,grams,spool_id) VALUES(?,?,?,?,?,?,?,'queued',?,?,?,?,?,?)", newID, u.ID, filename, path, size, digest, pid, trim(note, 200), time.Now().UnixMilli(), order, item.Estimate, item.Grams, item.SpoolID)
			if e != nil {
				return e
			}
			id, e := r.LastInsertId()
			if e != nil {
				return e
			}
			if e = recordSelectedPlate(tx, id, item.Plate); e != nil {
				return e
			}
			out.JobIDs = append(out.JobIDs, id)
		}
		ids, _ := json.Marshal(out.JobIDs)
		_, e = tx.Exec("INSERT INTO job_import_batches(user_id,request_id,input_hash,job_ids,created) VALUES(?,?,?,?,?)", u.ID, in.RequestID, fingerprint, string(ids), time.Now().Unix())
		return e
	})
	if err != nil {
		out.JobIDs = nil
	}
	return out, err
}
