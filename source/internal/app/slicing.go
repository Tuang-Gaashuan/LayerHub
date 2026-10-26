package app

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const slicingSchema = `
CREATE TABLE IF NOT EXISTS model_sources(id INTEGER PRIMARY KEY,user_id INTEGER NOT NULL REFERENCES users(id),filename TEXT NOT NULL,stl BLOB NOT NULL,triangles INTEGER NOT NULL,bounds TEXT NOT NULL,created INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS slice_tasks(id INTEGER PRIMARY KEY,user_id INTEGER NOT NULL REFERENCES users(id),model_id INTEGER NOT NULL REFERENCES model_sources(id),state TEXT NOT NULL,options TEXT NOT NULL,info TEXT NOT NULL DEFAULT '',output BLOB NOT NULL DEFAULT X'',sha256 TEXT NOT NULL DEFAULT '',error TEXT NOT NULL DEFAULT '',created INTEGER NOT NULL,updated INTEGER NOT NULL,job_id INTEGER REFERENCES jobs(id) ON DELETE SET NULL);
CREATE INDEX IF NOT EXISTS slice_owner ON slice_tasks(user_id,id DESC);
CREATE TABLE IF NOT EXISTS slicer_projects(id INTEGER PRIMARY KEY,user_id INTEGER NOT NULL REFERENCES users(id),model_id INTEGER NOT NULL REFERENCES model_sources(id),name TEXT NOT NULL COLLATE NOCASE,options TEXT NOT NULL,active_copy INTEGER NOT NULL,revision INTEGER NOT NULL,updated INTEGER NOT NULL,UNIQUE(user_id,name));
CREATE TABLE IF NOT EXISTS slicer_drafts(user_id INTEGER PRIMARY KEY REFERENCES users(id),model_id INTEGER REFERENCES model_sources(id),options TEXT NOT NULL DEFAULT '',active_copy INTEGER NOT NULL DEFAULT 0,revision INTEGER NOT NULL,updated INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS slicer_import_receipts(user_id INTEGER NOT NULL REFERENCES users(id),request_key TEXT NOT NULL,digest TEXT NOT NULL,project_id INTEGER NOT NULL REFERENCES slicer_projects(id) ON DELETE CASCADE,PRIMARY KEY(user_id,request_key));
`

type ModelSource struct {
	ID        int64       `json:"id"`
	Filename  string      `json:"filename"`
	Triangles int         `json:"triangles"`
	Bounds    ModelBounds `json:"bounds"`
	Created   int64       `json:"created"`
}
type SliceTask struct {
	ID       int64      `json:"id"`
	ModelID  int64      `json:"modelId"`
	Filename string     `json:"filename"`
	State    string     `json:"state"`
	Options  SliceSetup `json:"options"`
	Info     *SliceInfo `json:"info,omitempty"`
	Error    string     `json:"error,omitempty"`
	Created  int64      `json:"created"`
	Updated  int64      `json:"updated"`
	JobID    int64      `json:"jobId"`
}

func (s *Service) slicingReady() bool {
	if s.Config.SlicerPath == "" || s.Config.SlicerProfiles == "" {
		return false
	}
	for _, p := range []string{s.Config.SlicerPath, filepath.Join(s.Config.SlicerProfiles, "machine.json"), filepath.Join(s.Config.SlicerProfiles, "process.json"), filepath.Join(s.Config.SlicerProfiles, "filament.json")} {
		if st, e := os.Stat(p); e != nil || st.IsDir() {
			return false
		}
	}
	return true
}
func (s *Service) initSlicing() error {
	if _, err := s.DB.Exec(slicingSchema); err != nil {
		return err
	}
	if _, err := s.DB.Exec(processPresetSchema); err != nil {
		return err
	}
	if _, err := s.DB.Exec(teamProjectSchema); err != nil {
		return err
	}
	if _, err := s.DB.Exec("UPDATE slice_tasks SET state='failed',error='切片进程已中断，请重新切片',updated=? WHERE state='running'", time.Now().Unix()); err != nil {
		return err
	}
	s.sliceWake = make(chan struct{}, 1)
	s.sliceCancel = map[int64]context.CancelFunc{}
	if !s.slicingReady() || s.Config.DisableBackgroundEvents {
		return nil
	}
	s.workers.Add(1)
	go func() {
		defer s.workers.Done()
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			if s.ctx.Err() != nil {
				return
			}
			if s.pendingRestore.Load() {
				select {
				case <-s.ctx.Done():
					return
				case <-ticker.C:
				}
				continue
			}
			var id int64
			if s.DB.QueryRow("SELECT t.id FROM slice_tasks t JOIN users u ON u.id=t.user_id WHERE t.state='queued' AND u.disabled=0 ORDER BY t.id LIMIT 1").Scan(&id) == nil {
				s.runSlice(id)
				continue
			}
			select {
			case <-s.ctx.Done():
				return
			case <-s.sliceWake:
			case <-ticker.C:
			}
		}
	}()
	return nil
}
func (s *Service) sliceWorkspace(u User) (any, error) {
	models := []ModelSource{}
	rows, err := s.DB.Query("SELECT id,filename,triangles,bounds,created FROM model_sources WHERE user_id=? ORDER BY id DESC LIMIT 30", u.ID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var m ModelSource
		var b string
		if err = rows.Scan(&m.ID, &m.Filename, &m.Triangles, &b, &m.Created); err != nil {
			break
		}
		if err = json.Unmarshal([]byte(b), &m.Bounds); err != nil {
			break
		}
		models = append(models, m)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return nil, err
	}
	tasks := []SliceTask{}
	rows, err = s.DB.Query("SELECT t.id,t.model_id,m.filename,t.state,t.options,t.info,t.error,t.created,t.updated,COALESCE(t.job_id,0) FROM slice_tasks t JOIN model_sources m ON m.id=t.model_id WHERE t.user_id=? ORDER BY t.id DESC LIMIT 30", u.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var t SliceTask
		var o, info string
		if err = rows.Scan(&t.ID, &t.ModelID, &t.Filename, &t.State, &o, &info, &t.Error, &t.Created, &t.Updated, &t.JobID); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(o), &t.Options)
		if info != "" {
			json.Unmarshal([]byte(info), &t.Info)
		}
		tasks = append(tasks, t)
	}
	return map[string]any{"ok": true, "models": models, "tasks": tasks, "ready": s.slicingReady(), "machine": "X2D", "material": "Bambu PETG Basic", "materials": slicerMaterials, "maxBytes": maxModelBytes, "maxTriangles": maxModelTriangles, "objectProcessVersion": 2}, rows.Err()
}
func (s *Service) uploadModel(w http.ResponseWriter, r *http.Request, u User) {
	r.Body = http.MaxBytesReader(w, r.Body, maxModelBytes+(64<<10))
	defer r.Body.Close()
	mr, err := r.MultipartReader()
	if err != nil {
		fail(w, invalid("请选择 STL 模型"))
		return
	}
	var raw []byte
	name := ""
	for {
		part, e := mr.NextPart()
		if e == io.EOF {
			break
		}
		if e != nil {
			fail(w, invalid("模型上传中断或超过 8 MB"))
			return
		}
		if part.FormName() != "file" || part.FileName() == "" || name != "" {
			part.Close()
			fail(w, invalid("一次只接受一个 STL 模型"))
			return
		}
		name = filepath.Base(strings.ReplaceAll(part.FileName(), "\\", "/"))
		raw, e = io.ReadAll(io.LimitReader(part, maxModelBytes+1))
		part.Close()
		if e != nil {
			fail(w, invalid("模型读取失败"))
			return
		}
	}
	if !strings.EqualFold(filepath.Ext(name), ".stl") || len([]rune(name)) > 120 {
		fail(w, invalid("请选择文件名不超过 120 个字符的 STL"))
		return
	}
	mesh, err := parseModelSTL(raw)
	if err != nil {
		fail(w, err)
		return
	}
	normalized := encodeModelSTL(mesh)
	bounds, _ := json.Marshal(mesh.Bounds)
	var id int64
	err = s.transaction(func(tx *sql.Tx) error {
		var count, total int64
		if e := tx.QueryRow("SELECT COUNT(*),COALESCE(SUM(length(stl)),0) FROM model_sources WHERE user_id=?", u.ID).Scan(&count, &total); e != nil {
			return e
		}
		if count >= 30 || total+int64(len(normalized)) > 128<<20 {
			return conflict("模型空间已满，请删除不再使用的模型后重试")
		}
		res, e := tx.Exec("INSERT INTO model_sources(user_id,filename,stl,triangles,bounds,created) VALUES(?,?,?,?,?,?)", u.ID, name, normalized, len(mesh.Vertices)/3, string(bounds), time.Now().Unix())
		if e == nil {
			id, e = res.LastInsertId()
		}
		return e
	})
	if err != nil {
		fail(w, err)
		return
	}
	reply(w, 201, map[string]any{"ok": true, "model": ModelSource{ID: id, Filename: name, Triangles: len(mesh.Vertices) / 3, Bounds: mesh.Bounds, Created: time.Now().Unix()}})
}
func (s *Service) queueSlice(u User, modelID int64, o SliceSetup) (int64, error) {
	if _, e := sliceMaterial(o.Material); e != nil {
		return 0, e
	}
	if !s.slicingReady() {
		return 0, conflict("切片服务暂未就绪，可以先导入和预览模型")
	}
	if !regexp.MustCompile(`^#[0-9a-fA-F]{6}$`).MatchString(o.Color) {
		return 0, invalid("请选择有效的耗材显示颜色")
	}
	o.Color = strings.ToUpper(o.Color)
	var id int64
	err := s.transaction(func(tx *sql.Tx) error {
		if _, e := loadPlate(tx, u, modelID, o); e != nil {
			return e
		}
		var e error
		var all, mine int
		if e = tx.QueryRow("SELECT COUNT(*),COALESCE(SUM(CASE WHEN user_id=? THEN 1 ELSE 0 END),0) FROM slice_tasks WHERE state IN ('queued','running')", u.ID).Scan(&all, &mine); e != nil {
			return e
		}
		if all >= 8 || mine >= 2 {
			return conflict("切片任务较多，请等待当前任务完成")
		}
		var used int64
		if e = tx.QueryRow("SELECT COALESCE(SUM(length(output)),0) FROM slice_tasks WHERE user_id=?", u.ID).Scan(&used); e != nil {
			return e
		}
		if used > 256<<20 {
			return conflict("切片结果空间已满，请删除旧记录后重试")
		}
		options, _ := json.Marshal(o)
		now := time.Now().Unix()
		res, e := tx.Exec("INSERT INTO slice_tasks(user_id,model_id,state,options,created,updated) VALUES(?,?,'queued',?,?,?)", u.ID, modelID, string(options), now, now)
		if e == nil {
			id, e = res.LastInsertId()
		}
		return e
	})
	if err == nil {
		select {
		case s.sliceWake <- struct{}{}:
		default:
		}
	}
	return id, err
}
func (s *Service) cancelSlice(u User, id int64) error {
	err := s.transaction(func(tx *sql.Tx) error {
		res, e := tx.Exec("UPDATE slice_tasks SET state='cancelled',updated=? WHERE id=? AND user_id=? AND state IN ('queued','running')", time.Now().Unix(), id, u.ID)
		if e != nil {
			return e
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return conflict("任务已结束或不属于当前账户")
		}
		return nil
	})
	if err == nil {
		s.sliceMu.Lock()
		if cancel := s.sliceCancel[id]; cancel != nil {
			cancel()
		}
		s.sliceMu.Unlock()
	}
	return err
}
func (s *Service) deleteSliceModel(u User, id int64) error {
	return s.transaction(func(tx *sql.Tx) error {
		var owner int64
		if tx.QueryRow("SELECT user_id FROM model_sources WHERE id=?", id).Scan(&owner) != nil || owner != u.ID {
			return missing("模型不存在或无权访问")
		}
		projects, e := modelReferences(tx, u, id, "project", false)
		if e != nil {
			return e
		}
		if len(projects) > 0 {
			return conflict("模型仍被保存的工程使用，请先删除相关工程；模型与工程会一同保留")
		}
		active, e := modelReferences(tx, u, id, "task", true)
		if e != nil {
			return e
		}
		if len(active) > 0 {
			return conflict("请先取消或等待使用此模型的整盘切片任务")
		}
		drafts, e := modelReferences(tx, u, id, "draft", false)
		if e != nil {
			return e
		}
		for _, uid := range drafts {
			if _, e = tx.Exec("UPDATE slicer_drafts SET model_id=NULL,options='',active_copy=0,revision=revision+1,updated=? WHERE user_id=?", time.Now().UnixMilli(), uid); e != nil {
				return e
			}
		}
		tasks, e := modelReferences(tx, u, id, "task", false)
		if e != nil {
			return e
		}
		for _, task := range tasks {
			if _, e = tx.Exec("DELETE FROM slice_tasks WHERE id=? AND user_id=?", task, u.ID); e != nil {
				return e
			}
		}
		_, err := tx.Exec("DELETE FROM model_sources WHERE id=? AND user_id=?", id, u.ID)
		return err
	})
}
func (s *Service) sliceFile(w http.ResponseWriter, r *http.Request, u User, kind string, id int64) {
	var raw []byte
	var name string
	var err error
	switch kind {
	case "model":
		err = s.DB.QueryRow("SELECT stl,filename FROM model_sources WHERE id=? AND user_id=?", id, u.ID).Scan(&raw, &name)
	case "output", "preview":
		err = s.DB.QueryRow("SELECT t.output,m.filename FROM slice_tasks t JOIN model_sources m ON m.id=t.model_id WHERE t.id=? AND t.user_id=? AND t.state='succeeded'", id, u.ID).Scan(&raw, &name)
	default:
		err = sql.ErrNoRows
	}
	if err != nil {
		fail(w, missing("文件不存在、尚未生成或无权访问"))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if kind == "preview" {
		archive, e := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
		if e == nil {
			for _, f := range archive.File {
				if f.Name == "Metadata/plate_1.png" && f.UncompressedSize64 <= 4<<20 {
					stream, e := f.Open()
					if e != nil {
						break
					}
					image, e := io.ReadAll(io.LimitReader(stream, 4<<20))
					stream.Close()
					if e == nil {
						w.Header().Set("Content-Type", "image/png")
						w.Write(image)
						return
					}
				}
			}
		}
		var thumb []byte
		_ = s.transaction(func(tx *sql.Tx) error {
			var primary int64
			var raw string
			if e := tx.QueryRow("SELECT model_id,options FROM slice_tasks WHERE id=? AND user_id=? AND state='succeeded'", id, u.ID).Scan(&primary, &raw); e != nil {
				return e
			}
			var o SliceSetup
			if e := json.Unmarshal([]byte(raw), &o); e != nil {
				return e
			}
			mesh, e := loadPlate(tx, u, primary, o)
			if e != nil {
				return e
			}
			thumb, e = meshThumbnail(mesh, o)
			return e
		})
		if len(thumb) > 0 {
			w.Header().Set("Content-Type", "image/png")
			w.Write(thumb)
			return
		}
		fail(w, missing("此结果没有预览图"))
		return
	}
	if kind == "model" {
		w.Header().Set("Content-Type", "model/stl")
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="slice-%d.gcode.3mf"`, id))
	}
	w.Write(raw)
}

func (s *Service) promoteSlice(u User, id, pid int64, spoolID string) (int64, error) {
	var jobID int64
	var createdPath string
	err := s.transaction(func(tx *sql.Tx) error {
		var raw []byte
		var infoRaw, filename, hash string
		var previous sql.NullInt64
		if e := tx.QueryRow("SELECT t.output,t.info,m.filename,t.sha256,t.job_id FROM slice_tasks t JOIN model_sources m ON m.id=t.model_id WHERE t.id=? AND t.user_id=? AND t.state='succeeded'", id, u.ID).Scan(&raw, &infoRaw, &filename, &hash, &previous); e != nil {
			return missing("请选择自己的已完成切片结果")
		}
		if previous.Valid {
			return conflict("此结果已加入队列，请在我的任务中查看")
		}
		var info SliceInfo
		if json.Unmarshal([]byte(infoRaw), &info) != nil || len(info.Plates) != 1 || len(info.Plates[0].Filaments) != 1 {
			return conflict("切片结果无法确认")
		}
		plate := info.Plates[0]
		// Only the PETG Basic route has completed a physical print validation.
		// New profiles can be sliced and downloaded until separately accepted.
		if plate.Filaments[0].ProfileID != "GFG00" {
			return conflict("此材料预设支持切片预览与下载，实机验证后才开放在线打印")
		}
		grams := plate.Filaments[0].Grams
		estimate := int(math.Ceil(float64(plate.Seconds) / 60))
		if pid == 0 {
			return invalid("请确认一台 X2D 打印机")
		}
		var model string
		var enabled bool
		if tx.QueryRow("SELECT model_id,enabled FROM printers WHERE id=?", pid).Scan(&model, &enabled) != nil || model != "x2d" || !enabled {
			return invalid("请选择已启用的 X2D 打印机")
		}
		in := Schedule{Printer: pid, Estimate: estimate, Grams: grams, SpoolID: spoolID}
		if e := validateSchedule(tx, u, in, 0); e != nil {
			return e
		}
		spools, e := readSpools(tx)
		if e != nil {
			return e
		}
		slot := -1
		for _, sp := range spools {
			if sp.ID == spoolID {
				if !strings.EqualFold(sp.Material, "PETG") || sp.Mount == nil || sp.Mount.PrinterID != pid {
					return conflict("请选择目标设备已装载的 PETG 料卷")
				}
				n, e := strconv.Atoi(sp.Mount.Slot)
				if e != nil || n < 1 || n > 4 {
					return conflict("当前仅支持第一组 AMS 四槽")
				}
				slot = n - 1
			}
		}
		if slot < 0 {
			return invalid("请选择可用料卷")
		}
		file, e := os.CreateTemp(filepath.Join(s.Config.DataDir, "uploads"), "sliced-*.3mf")
		if e != nil {
			return e
		}
		createdPath = file.Name()
		if _, e = file.Write(raw); e != nil {
			file.Close()
			return e
		}
		if e = file.Close(); e != nil {
			return e
		}
		digest := sha256.Sum256(raw)
		if hex.EncodeToString(digest[:]) != hash {
			return conflict("结果文件校验失败，请重新切片")
		}
		options := PrintOptions{Plate: 1, UseAMS: true, BedLeveling: true, Mapping: []AMSTarget{{Filament: 0, AMSID: 0, SlotID: slot, SpoolID: spoolID, Grams: grams}}}
		if _, _, e = x2dPrintSlice(Job{Path: createdPath}, options); e != nil {
			return e
		}
		o, _ := json.Marshal(options)
		var order int64
		if e = tx.QueryRow("SELECT COALESCE(MAX(queue_order),0)+1 FROM jobs").Scan(&order); e != nil {
			return e
		}
		filename = strings.TrimSuffix(filename, filepath.Ext(filename)) + ".gcode.3mf"
		res, e := tx.Exec("INSERT INTO jobs(user_id,filename,path,size,sha256,printer_id,state,note,created_at,queue_order,estimate,grams,spool_id,print_options) VALUES(?,?,?,?,?,?,'queued',?,?,?,?,?,?,?)", u.ID, filename, createdPath, len(raw), hash, pid, "在线切片；供料与设备状态将在执行前再次检查", time.Now().UnixMilli(), order, estimate, grams, spoolID, string(o))
		if e != nil {
			return e
		}
		jobID, e = res.LastInsertId()
		if e != nil {
			return e
		}
		_, e = tx.Exec("UPDATE slice_tasks SET job_id=? WHERE id=?", jobID, id)
		return e
	})
	if err != nil && createdPath != "" {
		os.Remove(createdPath)
	}
	return jobID, err
}

func (s *Service) runSlice(id int64) {
	var mesh ModelMesh
	var objects []sliceObject
	var options string
	ready := false
	err := s.transaction(func(tx *sql.Tx) error {
		var primary, uid int64
		if e := tx.QueryRow("SELECT t.model_id,t.user_id,t.options FROM slice_tasks t JOIN users u ON u.id=t.user_id WHERE t.id=? AND t.state='queued' AND u.disabled=0", id).Scan(&primary, &uid, &options); e != nil {
			return e
		}
		var setup SliceSetup
		e := json.Unmarshal([]byte(options), &setup)
		if e == nil {
			mesh, e = loadPlate(tx, User{ID: uid}, primary, setup)
			if e == nil && hasObjectOverrides(setup) {
				objects, e = loadSliceObjects(tx, User{ID: uid}, primary, setup)
			}
		}
		if e != nil {
			_, writeErr := tx.Exec("UPDATE slice_tasks SET state='failed',error=?,updated=? WHERE id=? AND state='queued'", trim("盘内模型或参数无法恢复："+e.Error(), 1000), time.Now().Unix(), id)
			return writeErr
		}
		_, e = tx.Exec("UPDATE slice_tasks SET state='running',updated=? WHERE id=?", time.Now().Unix(), id)
		ready = e == nil
		return e
	})
	if err != nil || !ready {
		return
	}
	ctx, cancel := context.WithTimeout(s.ctx, 3*time.Minute)
	defer cancel()
	s.sliceMu.Lock()
	s.sliceCancel[id] = cancel
	s.sliceMu.Unlock()
	defer func() { s.sliceMu.Lock(); delete(s.sliceCancel, id); s.sliceMu.Unlock() }()
	var state string
	s.DB.QueryRow("SELECT state FROM slice_tasks WHERE id=?", id).Scan(&state)
	if state != "running" {
		return
	}
	var o SliceSetup
	json.Unmarshal([]byte(options), &o)
	output, info, e := s.executePreparedPlate(ctx, mesh, o, objects)
	state = "succeeded"
	message := ""
	if e != nil {
		state = "failed"
		message = trim(e.Error(), 1000)
		if ctx.Err() != nil {
			message = "切片已取消或超过 3 分钟，请简化模型后重试"
		}
	}
	encoded := ""
	hash := ""
	if e == nil {
		b, _ := json.Marshal(info)
		encoded = string(b)
		h := sha256.Sum256(output)
		hash = hex.EncodeToString(h[:])
	}
	if output == nil {
		output = []byte{}
	}
	_ = s.transaction(func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE slice_tasks SET state=?,error=?,output=?,info=?,sha256=?,updated=? WHERE id=? AND state='running'", state, message, output, encoded, hash, time.Now().Unix(), id)
		return err
	})
}
func (s *Service) executeSlice(ctx context.Context, raw []byte, o SliceSetup) ([]byte, SliceInfo, error) {
	empty := SliceInfo{}
	mesh, err := parseModelSTL(raw)
	if err != nil {
		return nil, empty, err
	}
	mesh, err = transformModel(mesh, o)
	if err != nil {
		return nil, empty, err
	}
	return s.executePlate(ctx, mesh, o)
}
func (s *Service) executePlate(ctx context.Context, mesh ModelMesh, o SliceSetup) ([]byte, SliceInfo, error) {
	return s.executePreparedPlate(ctx, mesh, o, nil)
}
func (s *Service) executePreparedPlate(ctx context.Context, mesh ModelMesh, o SliceSetup, objects []sliceObject) ([]byte, SliceInfo, error) {
	empty := SliceInfo{}
	material, e := sliceMaterial(o.Material)
	if e != nil {
		return nil, empty, e
	}
	if e := validateSliceAdvanced(o.Advanced); e != nil {
		return nil, empty, e
	}
	if o.Layer != .12 && o.Layer != .16 && o.Layer != .2 {
		return nil, empty, invalid("请选择已支持的层高")
	}
	dir, err := os.MkdirTemp(s.Config.DataDir, "slice-work-")
	if err != nil {
		return nil, empty, err
	}
	defer func() {
		rel, e := filepath.Rel(s.Config.DataDir, dir)
		if e == nil && filepath.Base(rel) == rel && strings.HasPrefix(rel, "slice-work-") {
			os.RemoveAll(dir)
		}
	}()
	if err = os.WriteFile(filepath.Join(dir, "model.stl"), encodeModelSTL(mesh), 0600); err != nil {
		return nil, empty, err
	}
	for _, kind := range []string{"machine", "process", "filament"} {
		b, e := os.ReadFile(filepath.Join(s.Config.SlicerProfiles, kind+".json"))
		if kind == "filament" {
			b, e = s.filamentProfile(o.Material)
		}
		if e != nil {
			return nil, empty, conflict("切片配置暂不可用")
		}
		if kind == "process" {
			var settings map[string]any
			if json.Unmarshal(b, &settings) != nil {
				return nil, empty, invalid("切片预设无效")
			}
			settings["layer_height"] = strconv.FormatFloat(o.Layer, 'f', 2, 64)
			settings["sparse_infill_density"] = fmt.Sprintf("%d%%", o.Infill)
			settings["enable_support"] = "0"
			if o.Support {
				settings["enable_support"] = "1"
			}
			settings["brim_type"] = "no_brim"
			if o.Brim {
				settings["brim_type"] = "outer_only"
			}
			for key, value := range advancedProcessSettings(o) {
				settings[key] = value
			}
			b, _ = json.Marshal(settings)
		}
		if e = os.WriteFile(filepath.Join(dir, kind+".json"), b, 0600); e != nil {
			return nil, empty, e
		}
	}
	inputPath := filepath.Join(dir, "model.stl")
	if len(objects) > 0 {
		inputPath, err = writeObjectProject(dir, objects, o)
		if err != nil {
			return nil, empty, err
		}
	}
	args := []string{"--datadir", filepath.Join(dir, "studio-data"), "--debug", "2", "--curr-bed-type", "Textured PEI Plate", "--filament-colour", o.Color, "--load-settings", filepath.Join(dir, "machine.json") + ";" + filepath.Join(dir, "process.json"), "--load-filaments", filepath.Join(dir, "filament.json"), "--arrange", "0", "--slice", "0", "--outputdir", dir, "--export-3mf", "output.gcode.3mf", inputPath}
	cmd := exec.CommandContext(ctx, s.Config.SlicerPath, args...)
	cmd.WaitDelay = 5 * time.Second
	prepareSlicerProcess(cmd)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err = cmd.Run(); err != nil {
		var report struct {
			Error string `json:"error_string"`
		}
		if b, e := os.ReadFile(filepath.Join(dir, "result.json")); e == nil {
			json.Unmarshal(b, &report)
		}
		if report.Error != "" {
			return nil, empty, conflict("切片失败：" + trim(report.Error, 600))
		}
		return nil, empty, conflict("切片进程未能完成，请检查模型或稍后重试")
	}
	path := filepath.Join(dir, "output.gcode.3mf")
	stat, err := os.Stat(path)
	if err != nil || stat.Size() > 32<<20 {
		return nil, empty, conflict("切片结果缺失或超过 32 MB")
	}
	info, plate, err := x2dPrintSlice(Job{Path: path}, PrintOptions{Plate: 1, UseAMS: true, Mapping: []AMSTarget{{AMSID: 0, SlotID: 0}}})
	if err != nil {
		return nil, empty, err
	}
	if modelKey(info.Model) != modelKey("X2D") || len(info.Plates) != 1 || plate.Nozzle.Physical != 1 || plate.Filaments[0].ProfileID != material.ProfileID || plate.Filaments[0].Color != o.Color || plate.Filaments[0].Material != material.Material {
		return nil, empty, conflict("切片结果与所选机器、喷嘴或耗材不一致")
	}
	if err = verifyAdvancedProcess(path, o); err != nil {
		return nil, empty, err
	}
	if len(objects) > 0 {
		if err = verifyObjectOutput(path, objects, o); err != nil {
			return nil, empty, err
		}
		output, e := os.ReadFile(path)
		return output, info, e
	}
	actual, err := outputModelBounds(path)
	if err != nil {
		return nil, empty, conflict("无法核对切片结果的模型摆放")
	}
	for k := 0; k < 3; k++ {
		if math.Abs(actual.Min[k]-mesh.Bounds.Min[k]) > .1 || math.Abs(actual.Max[k]-mesh.Bounds.Max[k]) > .1 {
			return nil, empty, conflict("切片引擎改变了模型位置或尺寸，请重新检查摆放")
		}
	}

	output, err := os.ReadFile(path)
	return output, info, err
}
