package app

import (
	"database/sql"
	"encoding/json"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxProjectPlates = 8

type SlicerPlate struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	ModelID    int64      `json:"modelId"`
	Options    SliceSetup `json:"options"`
	ActiveCopy int        `json:"activeCopy"`
	TaskID     int64      `json:"taskId,omitempty"`
}

// The existing model_id remains a foreign-key anchor to the first nonempty
// board. A versioned envelope preserves legacy single-board saves and backups.
type savedPlateSet struct {
	Version     int           `json:"version"`
	Plates      []SlicerPlate `json:"plates"`
	ActivePlate int           `json:"activePlate"`
}
type savedSlicerOptions struct {
	SliceSetup
	Project *savedPlateSet `json:"_project,omitempty"`
}

func normalizeSlicerState(s *SlicerState) error {
	if len(s.Plates) == 0 {
		return nil
	}
	if len(s.Plates) > maxProjectPlates || s.ActivePlate < 0 || s.ActivePlate >= len(s.Plates) {
		return invalid("工程最多 8 块打印板，当前打印板无效")
	}
	found := false
	for _, p := range s.Plates {
		if p.ModelID > 0 && !found {
			s.ModelID = p.ModelID
			s.Options = p.Options
			s.ActiveCopy = p.ActiveCopy
			found = true
		}
	}
	if !found {
		return invalid("工程至少需要一个模型；可以保留其他空打印板")
	}
	return nil
}
func encodeSlicerOptions(s SlicerState) string {
	if len(s.Plates) == 0 && instanceStateVersion(s) >= 3 {
		s.Plates = []SlicerPlate{{ID: "main", Name: "打印板 01", ModelID: s.ModelID, Options: s.Options, ActiveCopy: s.ActiveCopy}}
		s.ActivePlate = 0
	}
	o := savedSlicerOptions{SliceSetup: s.Options}
	if len(s.Plates) > 0 {
		o.Project = &savedPlateSet{Version: instanceStateVersion(s), Plates: s.Plates, ActivePlate: s.ActivePlate}
	}
	b, _ := json.Marshal(o)
	return string(b)
}
func decodeSlicerOptions(raw string, s *SlicerState) error {
	var o savedSlicerOptions
	if e := json.Unmarshal([]byte(raw), &o); e != nil {
		return e
	}
	s.Options = o.SliceSetup
	if o.Project != nil {
		if o.Project.Version != 2 && o.Project.Version != 3 && o.Project.Version != 4 {
			return invalid("工程格式版本不受支持，请升级软件")
		}
		s.Plates = o.Project.Plates
		s.FormatVersion = o.Project.Version
		s.ActivePlate = o.Project.ActivePlate
		return normalizeSlicerState(s)
	}
	return nil
}
func validateSlicerState(tx *sql.Tx, u User, s SlicerState) error {
	if e := validateProjectInstances(s); e != nil {
		return e
	}
	if len(s.Plates) == 0 {
		return validateSingleSlicerState(tx, u, s)
	}
	if e := normalizeSlicerState(&s); e != nil {
		return e
	}
	seen := map[string]bool{}
	total := 0
	for _, p := range s.Plates {
		if !regexp.MustCompile(`^[a-zA-Z0-9-]{1,48}$`).MatchString(p.ID) || seen[p.ID] || utf8.RuneCountInString(strings.TrimSpace(p.Name)) < 1 || utf8.RuneCountInString(p.Name) > 40 || strings.IndexFunc(p.Name, unicode.IsControl) >= 0 {
			return invalid("打印板标识或名称无效，标识不能重复")
		}
		seen[p.ID] = true
		if p.ModelID == 0 {
			if len(p.Options.Copies) != 0 || p.ActiveCopy != 0 || p.TaskID != 0 {
				return invalid("空打印板不能包含对象、选中项或切片结果")
			}
			// Validate process and placement fields with the already owned anchor,
			// without treating that anchor as an object on the empty board.
			if e := validateSingleSlicerState(tx, u, SlicerState{ModelID: s.ModelID, Options: p.Options}); e != nil {
				return e
			}
			continue
		}
		if e := validateSingleSlicerState(tx, u, SlicerState{ModelID: p.ModelID, Options: p.Options, ActiveCopy: p.ActiveCopy}); e != nil {
			return e
		}
		total += len(platePlacements(p.ModelID, p.Options))
		if total > 256 {
			return invalid("一个工程最多 256 个对象")
		}
		if p.TaskID < 0 {
			return invalid("切片结果标识无效")
		}
		if p.TaskID > 0 {
			var primary int64
			var raw string
			if tx.QueryRow("SELECT model_id,options FROM slice_tasks WHERE id=? AND user_id=?", p.TaskID, u.ID).Scan(&primary, &raw) != nil {
				return invalid("打印板的切片结果不存在或无权访问")
			}
			var o SliceSetup
			if json.Unmarshal([]byte(raw), &o) != nil || primary != p.ModelID || sliceLayoutSignature(o) != sliceLayoutSignature(p.Options) {
				return invalid("切片结果与打印板摆放或工艺不一致，请重新切片")
			}
		}
	}
	return nil
}
func sliceLayoutSignature(o SliceSetup) string {
	if !hasObjectOverrides(o) {
		o.Copies = append([]ModelPlacement{}, o.Copies...)
		for i := range o.Copies {
			o.Copies[i].InstanceID = ""
			if o.Copies[i].Process != nil && o.Copies[i].Process.Walls == nil && o.Copies[i].Process.Infill == nil {
				o.Copies[i].Process = nil
			}
		}
	}
	if len(o.Copies) > 0 {
		o.X = 0
		o.Y = 0
		o.Scale = 0
		o.Rotation = Point3{}
	}
	b, _ := json.Marshal(o)
	return string(b)
}
func (s *Service) slicerTask(u User, id int64) (SliceTask, error) {
	var t SliceTask
	var options, info string
	e := s.DB.QueryRow("SELECT t.id,t.model_id,m.filename,t.state,t.options,t.info,t.error,t.created,t.updated,COALESCE(t.job_id,0) FROM slice_tasks t JOIN model_sources m ON m.id=t.model_id WHERE t.id=? AND t.user_id=?", id, u.ID).Scan(&t.ID, &t.ModelID, &t.Filename, &t.State, &options, &info, &t.Error, &t.Created, &t.Updated, &t.JobID)
	if e == sql.ErrNoRows {
		return t, missing("切片结果不存在或无权访问")
	}
	if e != nil {
		return t, e
	}
	if e = json.Unmarshal([]byte(options), &t.Options); e != nil {
		return t, e
	}
	if info != "" {
		e = json.Unmarshal([]byte(info), &t.Info)
	}
	return t, e
}

// Cached older clients must not overwrite a multi-board save with just its
// anchor board. The user can explicitly create a separate legacy project.
func preventPlateDowngrade(tx *sql.Tx, table, key string, id, user int64, s SlicerState) error {
	var raw string
	e := tx.QueryRow("SELECT options FROM "+table+" WHERE "+key+"=? AND user_id=?", id, user).Scan(&raw)
	if e == sql.ErrNoRows {
		return nil
	}
	if e != nil {
		return e
	}
	if raw == "" {
		return nil
	}
	var old savedSlicerOptions
	if e = json.Unmarshal([]byte(raw), &old); e != nil {
		return e
	}
	if e := preventInstanceDowngrade(raw, s); e != nil {
		return e
	}
	if len(s.Plates) > 0 {
		return nil
	}
	if old.Project != nil {
		return conflict("该工程已包含多块打印板，请刷新页面后再保存，避免丢失其他打印板")
	}
	return nil
}
