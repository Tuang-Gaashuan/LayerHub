package app

import (
	"database/sql"
	"encoding/json"
)

func platePlacements(primary int64, o SliceSetup) []ModelPlacement {
	result := append([]ModelPlacement{}, o.Copies...)
	if len(result) == 0 {
		result = []ModelPlacement{{X: o.X, Y: o.Y, Scale: o.Scale, Rotation: o.Rotation}}
	}
	for i := range result {
		if result[i].ModelID == 0 {
			result[i].ModelID = primary
		}
	}
	return result
}

func plateSourceIDs(primary int64, o SliceSetup) ([]int64, error) {
	if primary <= 0 || len(o.Copies) > 64 {
		return nil, invalid("整盘源模型或对象数量无效")
	}
	ids := []int64{primary}
	seen := map[int64]bool{primary: true}
	found := false
	for _, p := range platePlacements(primary, o) {
		if p.ModelID <= 0 {
			return nil, invalid("盘内源模型无效")
		}
		if p.ModelID == primary {
			found = true
		}
		if !seen[p.ModelID] {
			seen[p.ModelID] = true
			ids = append(ids, p.ModelID)
		}
	}
	if !found {
		return nil, invalid("主模型必须属于当前打印盘")
	}
	return ids, nil
}

func loadPlate(tx *sql.Tx, u User, primary int64, o SliceSetup) (ModelMesh, error) {
	if e := validateObjectPlacements(o); e != nil {
		return ModelMesh{}, e
	}
	ids, e := plateSourceIDs(primary, o)
	if e != nil {
		return ModelMesh{}, e
	}
	sources := map[int64]ModelMesh{}
	for _, id := range ids {
		var raw []byte
		if tx.QueryRow("SELECT stl FROM model_sources WHERE id=? AND user_id=?", id, u.ID).Scan(&raw) != nil {
			return ModelMesh{}, missing("盘内模型不存在或不属于当前账户")
		}
		mesh, e := parseModelSTL(raw)
		if e != nil {
			return ModelMesh{}, e
		}
		sources[id] = mesh
	}
	return transformModels(sources, primary, o)
}

// References are read from the canonical saved JSON inside the same write
// transaction. This also handles existing projects without a schema migration.
func modelReferences(tx *sql.Tx, u User, id int64, kind string, activeOnly bool) ([]int64, error) {
	table, key := "", "id"
	switch kind {
	case "project":
		table = "slicer_projects"
	case "task":
		table = "slice_tasks"
	case "draft":
		table = "slicer_drafts"
		key = "user_id"
	default:
		return nil, invalid("源模型引用类型无效")
	}
	query := "SELECT " + key + ",model_id,options FROM " + table + " WHERE user_id=? AND model_id IS NOT NULL"
	if activeOnly {
		query += " AND state IN ('queued','running')"
	}
	rows, e := tx.Query(query, u.ID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	result := []int64{}
	for rows.Next() {
		var record, primary int64
		var raw string
		if e = rows.Scan(&record, &primary, &raw); e != nil {
			return nil, e
		}
		var o SliceSetup
		if e = json.Unmarshal([]byte(raw), &o); e != nil {
			return nil, e
		}
		used := primary == id
		for _, p := range platePlacements(primary, o) {
			used = used || p.ModelID == id
		}
		if kind != "task" {
			state := SlicerState{ModelID: primary}
			if e = decodeSlicerOptions(raw, &state); e != nil {
				return nil, e
			}
			for _, board := range state.Plates {
				if board.ModelID == 0 {
					continue
				}
				for _, p := range platePlacements(board.ModelID, board.Options) {
					used = used || p.ModelID == id
				}
			}
		}
		if used {
			result = append(result, record)
		}
	}
	return result, rows.Err()
}
