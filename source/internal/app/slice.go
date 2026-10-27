package app

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"image/png"
	"io"
	"math"
	"os"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type SliceFilament struct {
	Index     int     `json:"index"`
	Material  string  `json:"material"`
	Color     string  `json:"color"`
	Grams     int     `json:"grams"`
	UsedGrams float64 `json:"usedGrams,omitempty"`
	ProfileID string  `json:"profileId,omitempty"`
}
type SliceNozzle struct {
	Logical  int    `json:"logical"`
	Physical int    `json:"physical"`
	Diameter string `json:"diameter"`
	Volume   string `json:"volume"`
}
type SlicePlate struct {
	ID          int             `json:"id"`
	Filaments   []SliceFilament `json:"filaments"`
	MultiNozzle bool            `json:"multiNozzle"`
	Seconds     int             `json:"seconds"`
	Thumbnail   bool            `json:"thumbnail"`
	Warnings    []string        `json:"warnings,omitempty"`
	Nozzle      *SliceNozzle    `json:"nozzle,omitempty"`
}
type SliceInfo struct {
	Model           string       `json:"model"`
	Plates          []SlicePlate `json:"plates"`
	BedType         string       `json:"bedType,omitempty"`
	NozzleDiameters []string     `json:"nozzleDiameters,omitempty"`
}
type xmlMeta struct {
	Key   string `xml:"key,attr"`
	Value string `xml:"value,attr"`
}
type xmlPlate struct {
	Metadata  []xmlMeta `xml:"metadata"`
	Filaments []struct {
		ID        int     `xml:"id,attr"`
		Type      string  `xml:"type,attr"`
		Color     string  `xml:"color,attr"`
		Used      float64 `xml:"used_g,attr"`
		ProfileID string  `xml:"tray_info_idx,attr"`
		Group     int     `xml:"group_id,attr"`
	} `xml:"filament"`
	Nozzles []struct {
		Extruder int    `xml:"extruder_id,attr"`
		Group    int    `xml:"id,attr"`
		Diameter string `xml:"nozzle_diameter,attr"`
		Volume   string `xml:"volume_type,attr"`
	} `xml:"nozzle"`
	Mixed    []struct{} `xml:"mixed_filament"`
	Warnings []struct {
		Message string `xml:"msg,attr"`
	} `xml:"warning"`
}

func inspectSlice(filename string) (SliceInfo, error) {
	out := SliceInfo{Plates: []SlicePlate{}}
	z, err := zip.OpenReader(filename)
	if err != nil {
		return out, invalid("无法读取 3MF 压缩包")
	}
	defer z.Close()
	if len(z.File) > 10000 {
		return out, invalid("3MF 文件条目过多")
	}
	seen := map[string]bool{}
	folded := map[string]bool{}
	gcodes := map[int]bool{}
	var sliceRaw, settings []byte
	re := regexp.MustCompile(`^Metadata/plate_([1-9][0-9]*)\.gcode$`)
	for _, f := range z.File {
		name := strings.TrimSuffix(f.Name, "/")
		if name == "" || name == ".." || strings.HasPrefix(name, "../") || strings.ContainsAny(name, "\\:\x00?#") || strings.HasPrefix(name, "/") || path.Clean(name) != name || f.Mode()&os.ModeSymlink != 0 || !f.Mode().IsRegular() && !f.FileInfo().IsDir() || seen[f.Name] || folded[strings.ToLower(name)] {
			return out, invalid("3MF 含不安全或重复路径")
		}
		seen[f.Name] = true
		folded[strings.ToLower(strings.TrimSuffix(f.Name, "/"))] = true
		if m := re.FindStringSubmatch(f.Name); m != nil {
			id, _ := strconv.Atoi(m[1])
			if id > 1000 || f.UncompressedSize64 == 0 || f.UncompressedSize64 > 1<<30 {
				return out, invalid("打印盘 G-code 大小或编号无效")
			}
			gcodes[id] = true
		}
		if f.Name != "Metadata/slice_info.config" && f.Name != "Metadata/project_settings.config" {
			continue
		}
		if f.UncompressedSize64 > 4<<20 {
			return out, invalid("3MF 元数据过大")
		}
		r, e := f.Open()
		if e != nil {
			return out, invalid("3MF 元数据无法读取")
		}
		raw, e := io.ReadAll(io.LimitReader(r, (4<<20)+1))
		r.Close()
		if e != nil || len(raw) > 4<<20 {
			return out, invalid("3MF 元数据损坏")
		}
		if f.Name == "Metadata/slice_info.config" {
			sliceRaw = raw
		} else {
			settings = raw
		}
	}
	if len(gcodes) == 0 || len(sliceRaw) == 0 {
		return out, invalid("请选择已切片的打印 3MF；模型项目需先在切片软件中切片")
	}
	var physical []int
	if len(settings) > 0 {
		var cfg struct {
			Model     string          `json:"printer_model"`
			BedType   string          `json:"curr_bed_type"`
			Diameters json.RawMessage `json:"nozzle_diameter"`
			Physical  json.RawMessage `json:"physical_extruder_map"`
		}
		if json.Unmarshal(settings, &cfg) != nil {
			return out, invalid("3MF 项目配置无效")
		}
		out.Model = cfg.Model
		out.BedType = trim(cfg.BedType, 80)
		var values []string
		if json.Unmarshal(cfg.Physical, &values) == nil {
			for _, value := range values {
				n, e := strconv.Atoi(value)
				if e != nil {
					physical = nil
					break
				}
				physical = append(physical, n)
			}
		} else {
			_ = json.Unmarshal(cfg.Physical, &physical)
		}
		if len(physical) != 2 || physical[0] < 0 || physical[0] > 1 || physical[1] < 0 || physical[1] > 1 || physical[0] == physical[1] {
			physical = nil
		}
		var diameters []string
		if json.Unmarshal(cfg.Diameters, &diameters) != nil {
			var single string
			if json.Unmarshal(cfg.Diameters, &single) == nil {
				diameters = []string{single}
			}
		}
		for _, diameter := range diameters {
			if value, err := strconv.ParseFloat(diameter, 64); err == nil && value > 0 && value <= 2 && len(out.NozzleDiameters) < 16 {
				out.NozzleDiameters = append(out.NozzleDiameters, strconv.FormatFloat(value, 'f', -1, 64))
			}
		}
	}
	var cfg struct {
		Plates []xmlPlate `xml:"plate"`
	}
	if xml.Unmarshal(sliceRaw, &cfg) != nil {
		return out, invalid("3MF 切片信息无效")
	}
	ids := map[int]bool{}
	for _, p := range cfg.Plates {
		id := 0
		for _, m := range p.Metadata {
			if m.Key == "index" || m.Key == "plater_id" {
				id, _ = strconv.Atoi(m.Value)
			}
			if out.Model == "" && (m.Key == "printer_model" || m.Key == "printer_model_id") {
				out.Model = m.Value
			}
		}
		if !gcodes[id] {
			continue
		}
		if ids[id] {
			return out, invalid("3MF 打印盘编号重复")
		}
		ids[id] = true
		plate := SlicePlate{ID: id, Filaments: []SliceFilament{}, MultiNozzle: len(p.Mixed) > 0}
		plate.Thumbnail = seen[fmt.Sprintf("Metadata/plate_%d.png", id)]
		for _, warning := range p.Warnings {
			if warning.Message == "not_support_traditional_timelapse" && len(plate.Warnings) == 0 {
				plate.Warnings = append(plate.Warnings, "文件不支持传统延时摄影")
			}
		}
		for _, m := range p.Metadata {
			if m.Key == "prediction" {
				if n, e := strconv.Atoi(m.Value); e == nil && n >= 0 && n <= 366*86400 {
					plate.Seconds = n
				}
			}
		}
		nozzles := map[int]bool{}
		for _, n := range p.Nozzles {
			nozzles[n.Extruder] = true
		}
		if len(nozzles) > 1 {
			plate.MultiNozzle = true
		}
		if len(p.Nozzles) == 1 && len(physical) == 2 {
			n := p.Nozzles[0]
			logical := n.Extruder - 1
			diameter, e := strconv.ParseFloat(n.Diameter, 64)
			if logical >= 0 && logical < 2 && e == nil && diameter > 0 && diameter <= 2 {
				valid := true
				for _, f := range p.Filaments {
					if f.Used > 0 && f.Group != n.Group {
						valid = false
					}
				}
				if valid {
					plate.Nozzle = &SliceNozzle{Logical: logical, Physical: physical[logical], Diameter: strconv.FormatFloat(diameter, 'f', -1, 64), Volume: n.Volume}
				}
			}
		}
		used := map[int]bool{}
		for _, f := range p.Filaments {
			if f.ID < 1 || f.ID > 32 || used[f.ID] || math.IsNaN(f.Used) || math.IsInf(f.Used, 0) || f.Used < 0 || f.Used > 10000 {
				return out, invalid("3MF 耗材信息无效")
			}
			used[f.ID] = true
			if f.Used == 0 {
				continue
			}
			plate.Filaments = append(plate.Filaments, SliceFilament{Index: f.ID - 1, Material: f.Type, Color: f.Color, Grams: int(math.Ceil(f.Used)), UsedGrams: f.Used, ProfileID: trim(f.ProfileID, 40)})
		}
		if len(plate.Filaments) == 0 {
			return out, invalid("打印盘缺少可验证的耗材用量")
		}
		out.Plates = append(out.Plates, plate)
	}
	if len(out.Plates) == 0 {
		return out, invalid("3MF 盘号与 G-code 文件不匹配")
	}
	sort.Slice(out.Plates, func(i, j int) bool { return out.Plates[i].ID < out.Plates[j].ID })
	return out, nil
}

func (s *Service) SliceThumbnail(u User, id int64, plate int) ([]byte, error) {
	info, e := s.JobSlice(u, id)
	if e != nil {
		return nil, e
	}
	valid := false
	for _, p := range info.Plates {
		if p.ID == plate && p.Thumbnail {
			valid = true
		}
	}
	if !valid {
		return nil, missing("此盘没有缩略图")
	}
	jobs, e := readJobs(s.DB)
	if e != nil {
		return nil, e
	}
	for _, j := range jobs {
		if j.ID != id {
			continue
		}
		z, e := zip.OpenReader(j.Path)
		if e != nil {
			return nil, invalid("文件无法读取")
		}
		defer z.Close()
		for _, f := range z.File {
			if f.Name != fmt.Sprintf("Metadata/plate_%d.png", plate) {
				continue
			}
			if f.UncompressedSize64 > 4<<20 {
				return nil, invalid("缩略图过大")
			}
			r, e := f.Open()
			if e != nil {
				return nil, invalid("缩略图无法读取")
			}
			raw, e := io.ReadAll(io.LimitReader(r, (4<<20)+1))
			r.Close()
			if e != nil || len(raw) > 4<<20 {
				return nil, invalid("缩略图损坏")
			}
			cfg, e := png.DecodeConfig(bytes.NewReader(raw))
			if e != nil || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 2048 || cfg.Height > 2048 {
				return nil, invalid("缩略图格式或尺寸无效")
			}
			img, e := png.Decode(bytes.NewReader(raw))
			if e != nil {
				return nil, invalid("缩略图损坏")
			}
			var out bytes.Buffer
			if e = png.Encode(&out, img); e != nil {
				return nil, e
			}
			return out.Bytes(), nil
		}
	}
	return nil, missing("缩略图不存在")
}
func jobUses(j Job) map[string]int {
	uses := map[string]int{}
	if j.PrintOptions != nil && len(j.PrintOptions.Mapping) > 0 {
		for _, v := range j.PrintOptions.Mapping {
			uses[v.SpoolID] += v.Grams
		}
	} else {
		uses[j.SpoolID] = j.Grams
	}
	return uses
}
func (s *Service) JobSlice(u User, id int64) (SliceInfo, error) {
	jobs, err := readJobs(s.DB)
	if err != nil {
		return SliceInfo{}, err
	}
	for _, j := range jobs {
		if j.ID == id {
			if j.UserID != u.ID && !u.Admin {
				return SliceInfo{}, forbidden("无权读取该任务文件")
			}
			if !strings.EqualFold(path.Ext(j.Filename), ".3mf") {
				return SliceInfo{}, invalid("此任务不是 3MF")
			}
			return inspectSlice(j.Path)
		}
	}
	return SliceInfo{}, missing("任务不存在")
}
func modelKey(v string) string {
	key := strings.NewReplacer(" ", "", "-", "", "_", "").Replace(strings.ToLower(v))
	return strings.TrimPrefix(strings.TrimPrefix(key, "bambulab"), "bambu")
}
func validatePrintOptions(tx *sql.Tx, u User, j Job, p Printer, o PrintOptions) error {
	// Saving a preparation is not authorization to dispatch it. Keep the
	// unverified device mapping gate on both transfer and start paths.
	if strings.HasPrefix(p.ModelID, "h2") && p.ModelID != "h2s" {
		return conflict("此机型的多喷嘴启动映射尚未接入；准备可保存，暂不能传输或启动")
	}
	if err := validatePreparationOptions(tx, u, j, p, o); err != nil {
		return err
	}
	if p.ModelID == "x2d" {
		_, _, err := x2dPrintSlice(j, o)
		return err
	}
	return nil
}

func validatePreparationOptions(tx *sql.Tx, u User, j Job, p Printer, o PrintOptions) error {
	if !strings.EqualFold(path.Ext(j.Filename), ".3mf") {
		return invalid("盘号配置只用于切片 3MF")
	}
	info, err := inspectSlice(j.Path)
	if err != nil {
		return err
	}
	if info.Model == "" || modelKey(info.Model) != modelKey(models[p.ModelID]) {
		return conflict("切片机型无法确认或与目标设备不同，请为目标机型重新切片")
	}
	var plate *SlicePlate
	for i := range info.Plates {
		if info.Plates[i].ID == o.Plate {
			plate = &info.Plates[i]
		}
	}
	if plate == nil {
		return invalid("请选择文件中存在的打印盘")
	}
	if plate.MultiNozzle {
		return conflict("多喷嘴或混料切片暂不能直接启动")
	}
	if len(o.Mapping) != len(plate.Filaments) {
		return invalid("请为当前打印盘的每种耗材确认供料位置")
	}
	if !o.UseAMS && len(o.Mapping) != 1 {
		return invalid("外置料架模式只支持单耗材打印盘")
	}
	if o.UseAMS && p.Configuration != "ams" {
		return conflict("目标设备未配置 AMS")
	}
	spools, err := readSpools(tx)
	if err != nil {
		return err
	}
	jobs, err := readJobs(tx)
	if err != nil {
		return err
	}
	seen := map[int]bool{}
	uses := map[string]int{}
	slots := map[string]string{}
	for _, m := range o.Mapping {
		var f *SliceFilament
		for i := range plate.Filaments {
			if plate.Filaments[i].Index == m.Filament {
				f = &plate.Filaments[i]
			}
		}
		if f == nil || seen[m.Filament] || m.Grams < f.Grams || m.Grams > 10000 {
			return invalid("耗材索引重复、用量过低或映射无效")
		}
		seen[m.Filament] = true
		if m.AMSID < 0 || m.AMSID > 15 || m.SlotID < 0 || m.SlotID > 3 {
			return invalid("AMS 单元或槽位编号无效")
		}
		if o.UseAMS && m.AMSID != 0 {
			return conflict("多 AMS 的启动映射仍需实机适配；当前只允许第一组四槽")
		}
		slot := "external"
		if o.UseAMS {
			slot = strconv.Itoa(m.AMSID*4 + m.SlotID + 1)
			if !validSlot(p, slot) {
				return conflict("该供料位置尚未登记，请先在耗材库装载对应耗材")
			}
		}
		if other, ok := slots[slot]; ok && other != m.SpoolID {
			return invalid("同一槽位不能对应两卷耗材")
		}
		slots[slot] = m.SpoolID
		found := false
		for _, sp := range spools {
			if sp.ID != m.SpoolID {
				continue
			}
			found = true
			if !spoolVisible(sp, u) {
				return forbidden("无权使用映射耗材")
			}
			if sp.State != "available" || !strings.EqualFold(sp.Material, f.Material) {
				return conflict("映射耗材不可用或材质与切片不同")
			}
			if sp.Mount == nil || sp.Mount.PrinterID != p.ID || sp.Mount.Slot != slot {
				return conflict("请先在耗材库确认每卷耗材装在目标供料位置")
			}
		}
		if !found {
			return missing("映射耗材不存在")
		}
		uses[m.SpoolID] += m.Grams
	}
	for id, g := range uses {
		for _, sp := range spools {
			if sp.ID == id && sp.Remaining-reserve(jobs, id, j.ID) < g {
				return conflict("映射耗材可分配余量不足")
			}
		}
	}
	return nil
}
func (s *Service) SavePrintOptions(u User, id int64, o PrintOptions) error {
	return s.transaction(func(tx *sql.Tx) error {
		jobs, err := readJobs(tx)
		if err != nil {
			return err
		}
		for _, j := range jobs {
			if j.ID != id {
				continue
			}
			if j.UserID != u.ID && !u.Admin {
				return forbidden("无权配置该任务")
			}
			if j.State != "queued" && j.State != "assigned" {
				return conflict("只能配置等待或已传输的任务")
			}
			ps, err := readPrinters(tx)
			if err != nil {
				return err
			}
			for _, p := range ps {
				if p.ID != j.Printer {
					continue
				}
				if err = validatePreparationOptions(tx, u, j, p, o); err != nil {
					return err
				}
				raw, _ := json.Marshal(o)
				total := 0
				for _, v := range o.Mapping {
					total += v.Grams
				}
				_, err = tx.Exec("UPDATE jobs SET print_options=?,spool_id=?,grams=?,error='' WHERE id=?", string(raw), o.Mapping[0].SpoolID, total, id)
				return err
			}
			return invalid("请先确认目标设备")
		}
		return missing("任务不存在")
	})
}
