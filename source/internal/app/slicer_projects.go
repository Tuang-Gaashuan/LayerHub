package app

import (
	"database/sql"
	"math"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type SlicerState struct {
	FormatVersion int           `json:"formatVersion,omitempty"`
	ModelID       int64         `json:"modelId"`
	Options       SliceSetup    `json:"options"`
	ActiveCopy    int           `json:"activeCopy"`
	Plates        []SlicerPlate `json:"plates,omitempty"`
	ActivePlate   int           `json:"activePlate,omitempty"`
}
type SlicerProject struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Filename string `json:"filename"`
	Revision int64  `json:"revision"`
	Updated  int64  `json:"updated"`
	SlicerState
}
type ProjectWrite struct {
	ID       int64  `json:"id"`
	Revision int64  `json:"revision"`
	Name     string `json:"name"`
	SlicerState
}
type DraftWrite struct {
	Revision int64 `json:"revision"`
	SlicerState
}

// Incomplete layouts may be saved, but remain subject to all placement checks
// when slicing. Reject malformed/unbounded data without destroying the last save.
func validateSingleSlicerState(tx *sql.Tx, u User, state SlicerState) error {
	if e := validateObjectPlacements(state.Options); e != nil {
		return e
	}
	ids, e := plateSourceIDs(state.ModelID, state.Options)
	if e != nil {
		return e
	}
	counts := map[int64]int{}
	for _, id := range ids {
		var n int
		if tx.QueryRow("SELECT triangles FROM model_sources WHERE id=? AND user_id=?", id, u.ID).Scan(&n) != nil {
			return missing("盘内源模型不存在或不属于当前账号")
		}
		counts[id] = n
	}
	o := state.Options
	if _, e := sliceMaterial(o.Material); e != nil {
		return e
	}
	if e := validateSliceAdvanced(o.Advanced); e != nil {
		return e
	}
	if !regexp.MustCompile(`^#[0-9a-fA-F]{6}$`).MatchString(o.Color) || (o.Layer != .12 && o.Layer != .16 && o.Layer != .2) || o.Infill < 5 || o.Infill > 50 {
		return invalid("切片参数无效，未覆盖原有保存内容")
	}
	placements := platePlacements(state.ModelID, o)
	triangles := 0
	for _, p := range placements {
		triangles += counts[p.ModelID]
	}
	if len(placements) > 64 || triangles > maxModelTriangles || state.ActiveCopy < 0 || state.ActiveCopy >= len(placements) {
		return invalid("对象数量、选中项或整盘面数超出限制")
	}
	// Top-level legacy placement fields must remain sane as well.
	placements = append(append([]ModelPlacement{}, placements...), ModelPlacement{X: o.X, Y: o.Y, Scale: o.Scale, Rotation: o.Rotation})
	for _, p := range placements {
		if !finite(p.X) || !finite(p.Y) || math.Abs(p.X) > 1000 || math.Abs(p.Y) > 1000 || !finite(p.Scale) || p.Scale < .01 || p.Scale > 10 {
			return invalid("位置或缩放无效，未覆盖原有保存内容")
		}
		for _, angle := range p.Rotation {
			if !finite(angle) || math.Abs(angle) > 360 {
				return invalid("旋转角度无效")
			}
		}
	}
	return nil
}
func scanSlicerProject(row interface{ Scan(...any) error }) (SlicerProject, error) {
	var p SlicerProject
	var options string
	err := row.Scan(&p.ID, &p.Name, &p.ModelID, &p.Filename, &options, &p.ActiveCopy, &p.Revision, &p.Updated)
	if err == nil {
		err = decodeSlicerOptions(options, &p.SlicerState)
	}
	return p, err
}

const projectSelect = "SELECT p.id,p.name,p.model_id,m.filename,p.options,p.active_copy,p.revision,p.updated FROM slicer_projects p JOIN model_sources m ON m.id=p.model_id"

func (s *Service) slicerProject(u User, id int64) (SlicerProject, error) {
	p, e := scanSlicerProject(s.DB.QueryRow(projectSelect+" WHERE p.id=? AND p.user_id=?", id, u.ID))
	if e == sql.ErrNoRows {
		return p, missing("工程不存在或无权访问")
	}
	return p, e
}
func (s *Service) slicerStorage(u User) (any, error) {
	rows, e := s.DB.Query(projectSelect+" WHERE p.user_id=? ORDER BY p.updated DESC,p.id DESC", u.ID)
	if e != nil {
		return nil, e
	}
	projects := []SlicerProject{}
	for rows.Next() {
		p, err := scanSlicerProject(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		projects = append(projects, p)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	var model sql.NullInt64
	var options string
	var active int
	var revision, updated int64
	e = s.DB.QueryRow("SELECT model_id,options,active_copy,revision,updated FROM slicer_drafts WHERE user_id=?", u.ID).Scan(&model, &options, &active, &revision, &updated)
	if e != nil && e != sql.ErrNoRows {
		return nil, e
	}
	var draft *SlicerProject
	if model.Valid {
		p := SlicerProject{Revision: revision, Updated: updated, SlicerState: SlicerState{ModelID: model.Int64, ActiveCopy: active}}
		if e = decodeSlicerOptions(options, &p.SlicerState); e != nil {
			return nil, e
		}
		draft = &p
	}
	return map[string]any{"ok": true, "projects": projects, "draft": draft, "draftRevision": revision}, nil
}
func (s *Service) saveSlicerProject(u User, in ProjectWrite) (SlicerProject, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.ID < 0 || in.Revision < 0 || utf8.RuneCountInString(in.Name) < 1 || utf8.RuneCountInString(in.Name) > 80 || strings.IndexFunc(in.Name, unicode.IsControl) >= 0 {
		return SlicerProject{}, invalid("工程名称须为 1–80 个字符，不能包含控制字符")
	}
	if e := normalizeSlicerState(&in.SlicerState); e != nil {
		return SlicerProject{}, e
	}
	id := in.ID
	updated := time.Now().UnixMilli()
	filename := ""
	e := s.transaction(func(tx *sql.Tx) error {
		if e := validateSlicerState(tx, u, in.SlicerState); e != nil {
			return e
		}
		if e := tx.QueryRow("SELECT filename FROM model_sources WHERE id=? AND user_id=?", in.ModelID, u.ID).Scan(&filename); e != nil {
			return e
		}
		var duplicate int
		if e := tx.QueryRow("SELECT COUNT(*) FROM slicer_projects WHERE user_id=? AND name=? COLLATE NOCASE AND id<>?", u.ID, in.Name, id).Scan(&duplicate); e != nil {
			return e
		}
		if duplicate > 0 {
			return conflict("已有同名工程，请换个名称，或打开原工程后保存")
		}
		if id > 0 {
			if e := preventPlateDowngrade(tx, "slicer_projects", "id", id, u.ID, in.SlicerState); e != nil {
				return e
			}
		}
		options := encodeSlicerOptions(in.SlicerState)
		if id == 0 {
			if in.Revision != 0 {
				return conflict("新工程版本无效")
			}
			var count int
			if e := tx.QueryRow("SELECT COUNT(*) FROM slicer_projects WHERE user_id=?", u.ID).Scan(&count); e != nil {
				return e
			}
			if count >= 20 {
				return conflict("最多保存 20 个工程，请整理后再保存")
			}
			result, e := tx.Exec("INSERT INTO slicer_projects(user_id,model_id,name,options,active_copy,revision,updated) VALUES(?,?,?,?,?,1,?)", u.ID, in.ModelID, in.Name, string(options), in.ActiveCopy, updated)
			if e != nil {
				return e
			}
			id, e = result.LastInsertId()
			return e
		}
		result, e := tx.Exec("UPDATE slicer_projects SET model_id=?,name=?,options=?,active_copy=?,revision=revision+1,updated=? WHERE id=? AND user_id=? AND revision=?", in.ModelID, in.Name, string(options), in.ActiveCopy, updated, id, u.ID, in.Revision)
		if e != nil {
			return e
		}
		n, _ := result.RowsAffected()
		if n != 1 {
			return conflict("工程已被其他页面修改或删除，本页内容未覆盖；可另存为新工程")
		}
		return nil
	})
	if e != nil {
		return SlicerProject{}, e
	}
	revision := in.Revision + 1
	return SlicerProject{ID: id, Name: in.Name, Filename: filename, Revision: revision, Updated: updated, SlicerState: in.SlicerState}, nil
}
func (s *Service) deleteSlicerProject(u User, id, revision int64) error {
	return s.transaction(func(tx *sql.Tx) error {
		r, e := tx.Exec("DELETE FROM slicer_projects WHERE id=? AND user_id=? AND revision=?", id, u.ID, revision)
		if e != nil {
			return e
		}
		n, _ := r.RowsAffected()
		if n != 1 {
			return conflict("工程已变化或无权操作，请刷新工程列表")
		}
		return nil
	})
}
func (s *Service) saveSlicerDraft(u User, in DraftWrite, clear bool) (int64, error) {
	if in.Revision < 0 {
		return 0, invalid("草稿版本无效")
	}
	if !clear {
		if e := normalizeSlicerState(&in.SlicerState); e != nil {
			return 0, e
		}
	}
	next := in.Revision + 1
	e := s.transaction(func(tx *sql.Tx) error {
		if !clear {
			if e := validateSlicerState(tx, u, in.SlicerState); e != nil {
				return e
			}
		}
		var current int64
		e := tx.QueryRow("SELECT revision FROM slicer_drafts WHERE user_id=?", u.ID).Scan(&current)
		if e != nil && e != sql.ErrNoRows {
			return e
		}
		if current != in.Revision {
			return conflict("草稿已被其他页面更新，本页内容尚未保存；请重新读取或保存为工程")
		}
		var model any
		options := ""
		active := 0
		if !clear {
			model = in.ModelID
			if e := preventPlateDowngrade(tx, "slicer_drafts", "user_id", u.ID, u.ID, in.SlicerState); e != nil {
				return e
			}
			options = encodeSlicerOptions(in.SlicerState)
			active = in.ActiveCopy
		}
		_, e = tx.Exec("INSERT INTO slicer_drafts(user_id,model_id,options,active_copy,revision,updated) VALUES(?,?,?,?,?,?) ON CONFLICT(user_id) DO UPDATE SET model_id=excluded.model_id,options=excluded.options,active_copy=excluded.active_copy,revision=excluded.revision,updated=excluded.updated", u.ID, model, options, active, next, time.Now().UnixMilli())
		return e
	})
	return next, e
}
