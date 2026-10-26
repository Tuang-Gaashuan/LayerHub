package app

import (
	"database/sql"
	"encoding/json"
	"math"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const processPresetSchema = `
CREATE TABLE IF NOT EXISTS slicer_process_presets(id TEXT PRIMARY KEY,user_id INTEGER NOT NULL REFERENCES users(id),name TEXT NOT NULL COLLATE NOCASE,settings TEXT NOT NULL,revision INTEGER NOT NULL,updated INTEGER NOT NULL,archived INTEGER NOT NULL DEFAULT 0);
CREATE UNIQUE INDEX IF NOT EXISTS slicer_preset_name ON slicer_process_presets(user_id,name COLLATE NOCASE) WHERE archived=0;
`

// A recipe has no model geometry, color, printer address or executable settings.
type ProcessSettings struct {
	Material string         `json:"material"`
	Layer    float64        `json:"layer"`
	Infill   int            `json:"infill"`
	Support  bool           `json:"support"`
	Brim     bool           `json:"brim"`
	Advanced *SliceAdvanced `json:"advanced"`
}
type ProcessPreset struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Settings ProcessSettings `json:"settings"`
	Revision int64           `json:"revision"`
	Updated  int64           `json:"updated"`
	Archived bool            `json:"archived"`
}
type PresetWrite struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Settings ProcessSettings `json:"settings"`
	Revision int64           `json:"revision"`
}
type PresetArchive struct {
	ID       string `json:"id"`
	Revision int64  `json:"revision"`
	Archived bool   `json:"archived"`
}

func normalizeProcessSettings(p *ProcessSettings) error {
	m, err := sliceMaterial(p.Material)
	if err != nil {
		return err
	}
	p.Material = m.Key
	if p.Layer != .12 && p.Layer != .16 && p.Layer != .2 {
		return invalid("请选择已支持的层高")
	}
	if p.Infill < 5 || p.Infill > 50 {
		return invalid("填充密度须为 5%–50%")
	}
	if p.Advanced == nil {
		p.Advanced = &SliceAdvanced{Walls: 2, TopThickness: 1, BottomThickness: math.Round(p.Layer*300) / 100, Pattern: "grid", SupportType: "tree", SupportAngle: 30}
	} else {
		a := *p.Advanced
		p.Advanced = &a
	}
	return validateSliceAdvanced(p.Advanced)
}
func validPresetID(id string) bool {
	return regexp.MustCompile(`^[a-zA-Z0-9-]{12,64}$`).MatchString(id)
}
func scanProcessPreset(row interface{ Scan(...any) error }) (ProcessPreset, error) {
	var p ProcessPreset
	var raw string
	err := row.Scan(&p.ID, &p.Name, &raw, &p.Revision, &p.Updated, &p.Archived)
	if err == nil {
		err = json.Unmarshal([]byte(raw), &p.Settings)
	}
	return p, err
}

const presetSelect = "SELECT id,name,settings,revision,updated,archived FROM slicer_process_presets"

func (s *Service) processPresets(u User) ([]ProcessPreset, error) {
	rows, err := s.DB.Query(presetSelect+" WHERE user_id=? ORDER BY archived,updated DESC,id", u.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProcessPreset{}
	for rows.Next() {
		p, e := scanProcessPreset(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
func checkPresetName(tx *sql.Tx, u User, id, name string) error {
	var count int
	if err := tx.QueryRow("SELECT COUNT(*) FROM slicer_process_presets WHERE user_id=? AND name=? COLLATE NOCASE AND id<>? AND archived=0", u.ID, name, id).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return conflict("已有同名预设，请另起名称或明确更新原预设")
	}
	return nil
}
func (s *Service) saveProcessPreset(u User, in PresetWrite) (ProcessPreset, error) {
	in.Name = strings.TrimSpace(in.Name)
	if !validPresetID(in.ID) || utf8.RuneCountInString(in.Name) < 1 || utf8.RuneCountInString(in.Name) > 40 || strings.IndexFunc(in.Name, unicode.IsControl) >= 0 {
		return ProcessPreset{}, invalid("预设名称须为 1–40 个字符，标识无效或名称包含控制字符")
	}
	if err := normalizeProcessSettings(&in.Settings); err != nil {
		return ProcessPreset{}, err
	}
	raw, _ := json.Marshal(in.Settings)
	var out ProcessPreset
	err := s.transaction(func(tx *sql.Tx) error {
		p, e := scanProcessPreset(tx.QueryRow(presetSelect+" WHERE id=? AND user_id=?", in.ID, u.ID))
		if e != nil && e != sql.ErrNoRows {
			return e
		}
		if e == nil {
			if in.Revision == 0 {
				stored, _ := json.Marshal(p.Settings)
				if !p.Archived && p.Name == in.Name && string(stored) == string(raw) {
					out = p
					return nil
				}
				return conflict("该保存请求已有记录，请重新读取预设")
			}
			if p.Archived || p.Revision != in.Revision {
				return conflict("预设已被其他页面修改或收起，请重新读取；当前参数未被覆盖")
			}
		} else {
			if in.Revision != 0 {
				return conflict("预设不存在或无权修改")
			}
			var count int
			if e = tx.QueryRow("SELECT COUNT(*) FROM slicer_process_presets WHERE user_id=?", u.ID).Scan(&count); e != nil {
				return e
			}
			if count >= 100 {
				return conflict("每位成员最多保留 100 份预设，收起的预设可在管理中恢复复用")
			}
		}
		if e = checkPresetName(tx, u, in.ID, in.Name); e != nil {
			return e
		}
		now := time.Now().UnixMilli()
		if in.Revision == 0 {
			if _, e = tx.Exec("INSERT INTO slicer_process_presets(id,user_id,name,settings,revision,updated) VALUES(?,?,?,?,1,?)", in.ID, u.ID, in.Name, string(raw), now); e != nil {
				if strings.Contains(e.Error(), "UNIQUE constraint failed") {
					return conflict("保存请求标识已存在，请重新读取")
				}
				return e
			}
		} else {
			if _, e = tx.Exec("UPDATE slicer_process_presets SET name=?,settings=?,revision=revision+1,updated=? WHERE id=? AND user_id=?", in.Name, string(raw), now, in.ID, u.ID); e != nil {
				return e
			}
		}
		var readErr error
		out, readErr = scanProcessPreset(tx.QueryRow(presetSelect+" WHERE id=? AND user_id=?", in.ID, u.ID))
		return readErr
	})
	return out, err
}
func (s *Service) archiveProcessPreset(u User, in PresetArchive) error {
	return s.transaction(func(tx *sql.Tx) error {
		p, e := scanProcessPreset(tx.QueryRow(presetSelect+" WHERE id=? AND user_id=?", in.ID, u.ID))
		if e != nil {
			return missing("预设不存在或无权操作")
		}
		// Re-reading after a lost acknowledgement is safe; an old revision must
		// never overwrite somebody else's newer change, even to the same flag.
		if p.Revision != in.Revision {
			return conflict("预设已变更，请重新读取后操作")
		}
		if p.Archived == in.Archived {
			return nil
		}
		if !in.Archived {
			if e = checkPresetName(tx, u, p.ID, p.Name); e != nil {
				return e
			}
		}
		_, e = tx.Exec("UPDATE slicer_process_presets SET archived=?,revision=revision+1,updated=? WHERE id=? AND user_id=?", in.Archived, time.Now().UnixMilli(), in.ID, u.ID)
		return e
	})
}
