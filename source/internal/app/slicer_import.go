package app

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

func importedSliceDefaults() SliceSetup {
	return SliceSetup{X: 138.25, Y: 128, Scale: 1, Layer: .2, Infill: 15, Color: "#FFFFFF"}
}

// Analysis is read-only. Commit requires an explicit geometry-only agreement
// and the same digest. A receipt makes a retried uncertain commit idempotent.
func (s *Service) importModel3MF(w http.ResponseWriter, r *http.Request, u User) {
	if !s.modelImportBusy.CompareAndSwap(false, true) {
		fail(w, conflict("正在解析另一个工程，请稍后重试"))
		return
	}
	defer s.modelImportBusy.Store(false)
	r.Body = http.MaxBytesReader(w, r.Body, maxProjectImportBytes+(64<<10))
	defer r.Body.Close()
	mr, e := r.MultipartReader()
	if e != nil {
		fail(w, invalid("请选择工程 3MF 文件"))
		return
	}
	var raw []byte
	name := ""
	fields := map[string]string{}
	for {
		part, e := mr.NextPart()
		if e == io.EOF {
			break
		}
		if e != nil {
			fail(w, invalid("工程上传不完整或超过 32 MB"))
			return
		}
		if part.FormName() == "file" && part.FileName() != "" && name == "" {
			name = filepath.Base(strings.ReplaceAll(part.FileName(), "\\", "/"))
			raw, e = io.ReadAll(io.LimitReader(part, maxProjectImportBytes+1))
		} else if part.FileName() == "" && (part.FormName() == "mode" || part.FormName() == "sha256" || part.FormName() == "requestKey" || part.FormName() == "acknowledge") {
			if _, exists := fields[part.FormName()]; exists {
				part.Close()
				fail(w, invalid("导入字段重复"))
				return
			}
			b, err := io.ReadAll(io.LimitReader(part, 257))
			e = err
			if len(b) > 256 {
				e = invalid("导入字段过长")
			}
			fields[part.FormName()] = string(b)
		} else {
			part.Close()
			fail(w, invalid("一次只接受一个工程文件及导入确认字段"))
			return
		}
		part.Close()
		if e != nil {
			fail(w, invalid("工程读取失败"))
			return
		}
	}
	if !strings.EqualFold(filepath.Ext(name), ".3mf") || len([]rune(name)) > 120 {
		fail(w, invalid("请选择文件名不超过 120 个字符的 3MF"))
		return
	}
	mode := fields["mode"]
	if mode != "review" && mode != "commit" {
		fail(w, invalid("请先检查工程，再确认导入"))
		return
	}
	if mode == "commit" && (fields["acknowledge"] != "geometry-only" || !regexp.MustCompile(`^[a-zA-Z0-9-]{16,64}$`).MatchString(fields["requestKey"]) || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(fields["sha256"])) {
		fail(w, invalid("导入前须确认仅导入几何、重新排版并使用 LayerHub 工艺"))
		return
	}
	p, e := parseModel3MF(r.Context(), raw)
	if e != nil {
		fail(w, e)
		return
	}
	if mode == "review" {
		reply(w, 200, map[string]any{"ok": true, "review": p.summary()})
		return
	}
	if fields["sha256"] != p.Digest {
		fail(w, conflict("工程文件已变化，请重新检查并确认"))
		return
	}
	if e = r.Context().Err(); e != nil {
		return
	}
	project, replayed, e := s.commitModelImport(u, name, p, fields["requestKey"])
	if e != nil {
		fail(w, e)
		return
	}
	reply(w, 201, map[string]any{"ok": true, "project": project, "replayed": replayed})
}

func (s *Service) commitModelImport(u User, filename string, in modelImport, key string) (SlicerProject, bool, error) {
	project := SlicerProject{}
	replayed := false
	e := s.transaction(func(tx *sql.Tx) error {
		var prior int64
		var digest string
		err := tx.QueryRow("SELECT project_id,digest FROM slicer_import_receipts WHERE user_id=? AND request_key=?", u.ID, key).Scan(&prior, &digest)
		if err == nil {
			if digest != in.Digest {
				return conflict("此导入标识已用于其他文件，请重新选择文件")
			}
			project, err = scanSlicerProject(tx.QueryRow(projectSelect+" WHERE p.id=? AND p.user_id=?", prior, u.ID))
			replayed = true
			return err
		}
		if err != sql.ErrNoRows {
			return err
		}
		var models, used, projects int64
		if err = tx.QueryRow("SELECT COUNT(*),COALESCE(SUM(length(stl)),0) FROM model_sources WHERE user_id=?", u.ID).Scan(&models, &used); err != nil {
			return err
		}
		if err = tx.QueryRow("SELECT COUNT(*) FROM slicer_projects WHERE user_id=?", u.ID).Scan(&projects); err != nil {
			return err
		}
		if projects >= 20 {
			return conflict("工程空间已满（20 个），请先整理我的工程；当前内容未改变")
		}
		bytes := int64(0)
		for _, source := range in.Sources {
			bytes += int64(len(source.Raw))
		}
		if models+int64(len(in.Sources)) > 30 || used+bytes > 128<<20 {
			return conflict("模型库空间不足：最多 30 种模型、128 MB；未导入任何对象，请先整理模型库")
		}
		now := time.Now()
		ids := make([]int64, len(in.Sources))
		for i, source := range in.Sources {
			bounds, _ := json.Marshal(source.Mesh.Bounds)
			name := importLabel(source.Name, "导入模型", 100) + ".stl"
			result, err := tx.Exec("INSERT INTO model_sources(user_id,filename,stl,triangles,bounds,created) VALUES(?,?,?,?,?,?)", u.ID, name, source.Raw, len(source.Mesh.Vertices)/3, string(bounds), now.Unix())
			if err != nil {
				return err
			}
			ids[i], err = result.LastInsertId()
			if err != nil {
				return err
			}
		}
		state := SlicerState{Plates: []SlicerPlate{}}
		for _, board := range in.Boards {
			plate := SlicerPlate{ID: "import-" + randomID(12), Name: board.Name, Options: importedSliceDefaults()}
			for i, source := range board.Objects {
				placement := board.Placements[i]
				placement.ModelID = ids[source]
				plate.Options.Copies = append(plate.Options.Copies, placement)
			}
			if len(plate.Options.Copies) > 0 {
				placement := plate.Options.Copies[0]
				plate.ModelID = placement.ModelID
				plate.Options.X = placement.X
				plate.Options.Y = placement.Y
			}
			state.Plates = append(state.Plates, plate)
		}
		if err = normalizeSlicerState(&state); err != nil {
			return err
		}
		if err = validateSlicerState(tx, u, state); err != nil {
			return err
		}
		for _, plate := range state.Plates {
			if plate.ModelID > 0 {
				if _, err = loadPlate(tx, u, plate.ModelID, plate.Options); err != nil {
					return err
				}
			}
		}
		stem := importLabel(strings.TrimSuffix(filename, filepath.Ext(filename)), "3MF 工程", 60)
		base := "导入 · " + stem
		name := base
		for n := 1; n <= 21; n++ {
			var count int
			if err = tx.QueryRow("SELECT COUNT(*) FROM slicer_projects WHERE user_id=? AND name=? COLLATE NOCASE", u.ID, name).Scan(&count); err != nil {
				return err
			}
			if count == 0 {
				break
			}
			name = fmt.Sprintf("%s (%d)", base, n+1)
		}
		result, err := tx.Exec("INSERT INTO slicer_projects(user_id,model_id,name,options,active_copy,revision,updated) VALUES(?,?,?,?,?,1,?)", u.ID, state.ModelID, name, encodeSlicerOptions(state), state.ActiveCopy, now.UnixMilli())
		if err != nil {
			return err
		}
		id, err := result.LastInsertId()
		if err != nil {
			return err
		}
		if _, err = tx.Exec("INSERT INTO slicer_import_receipts(user_id,request_key,digest,project_id) VALUES(?,?,?,?)", u.ID, key, in.Digest, id); err != nil {
			return err
		}
		project = SlicerProject{ID: id, Name: name, Revision: 1, Updated: now.UnixMilli(), SlicerState: state}
		return tx.QueryRow("SELECT filename FROM model_sources WHERE id=?", state.ModelID).Scan(&project.Filename)
	})
	return project, replayed, e
}
