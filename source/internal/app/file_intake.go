package app

// File inspection is advisory and ephemeral. It creates no project/job, passes
// no embedded configuration to an engine and never contacts a printer.
import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

type ImportReference struct {
	Printer   string   `json:"printer,omitempty"`
	Bed       string   `json:"bed,omitempty"`
	Nozzles   []string `json:"nozzles,omitempty"`
	Materials []string `json:"materials,omitempty"`
	Colors    []string `json:"colors,omitempty"`
	Layer     string   `json:"layer,omitempty"`
	Infill    string   `json:"infill,omitempty"`
	Pattern   string   `json:"pattern,omitempty"`
}

// Only these display fields leave the parser. Machine macros, custom G-code,
// paths, vendor account details and arbitrary metadata are never returned.
func importReference(raw []byte) *ImportReference {
	var values map[string]json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &values) != nil {
		return nil
	}
	list := func(key string) []string {
		var ss []string
		if json.Unmarshal(values[key], &ss) != nil {
			var s string
			if json.Unmarshal(values[key], &s) != nil {
				return nil
			}
			ss = []string{s}
		}
		out := []string{}
		for _, s := range ss {
			if len(out) == 32 {
				break
			}
			if s = referenceLabel(s); s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	one := func(key string) string {
		ss := list(key)
		if len(ss) > 0 {
			return ss[0]
		}
		return ""
	}
	r := &ImportReference{Printer: one("printer_model"), Bed: one("curr_bed_type"), Nozzles: list("nozzle_diameter"), Materials: list("filament_type"), Colors: list("filament_colour"), Layer: one("layer_height"), Infill: one("sparse_infill_density"), Pattern: one("sparse_infill_pattern")}
	if r.Printer == "" && r.Bed == "" && len(r.Nozzles) == 0 && len(r.Materials) == 0 && len(r.Colors) == 0 && r.Layer == "" && r.Infill == "" && r.Pattern == "" {
		return nil
	}
	return r
}

type fileSliceReview struct {
	Available bool       `json:"available"`
	Info      *SliceInfo `json:"info"`
}
type fileReview struct {
	Schema    int              `json:"schema"`
	Kind      string           `json:"kind"`
	SHA256    string           `json:"sha256"`
	Bytes     int64            `json:"bytes"`
	Sliced    fileSliceReview  `json:"sliced"`
	Reference *ImportReference `json:"reference,omitempty"`
}

func referenceLabel(value string) string {
	value = strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
			return -1
		}
		return r
	}, value))
	chars := []rune(value)
	if len(chars) > 100 {
		value = string(chars[:100])
	}
	return value
}
func inspectPrintFile(ctx context.Context, filename, digest string, size int64) (fileReview, error) {
	out := fileReview{Schema: 2, Kind: "sliced-3mf", SHA256: digest, Bytes: size}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	info, err := inspectSlice(filename)
	if err != nil {
		return out, invalid("Beta 仅接受已切片的 3MF 文件，请先在 Bambu Studio 完成切片并导出切片盘文件")
	}
	out.Sliced = fileSliceReview{Available: true, Info: &info}
	z, err := zip.OpenReader(filename)
	if err != nil {
		return out, err
	}
	defer z.Close()
	for _, f := range z.File {
		if err = ctx.Err(); err != nil {
			return out, err
		}
		if f.Name != "Metadata/project_settings.config" {
			continue
		}
		if f.UncompressedSize64 > 4<<20 {
			return out, invalid("3MF 元数据过大")
		}
		reader, e := f.Open()
		if e != nil {
			return out, invalid("3MF 元数据无法读取")
		}
		raw, e := io.ReadAll(io.LimitReader(reader, (4<<20)+1))
		reader.Close()
		if e != nil || len(raw) > 4<<20 {
			return out, invalid("3MF 元数据损坏")
		}
		out.Reference = importReference(raw)
	}
	return out, nil
}

func (s *Service) inspectFile(w http.ResponseWriter, r *http.Request) {
	if !s.fileInspectBusy.CompareAndSwap(false, true) {
		fail(w, conflict("正在检查另一个文件，请稍后重试"))
		return
	}
	defer s.fileInspectBusy.Store(false)
	limit := s.Config.MaxUpload
	if limit <= 0 || limit > 500<<20 {
		limit = 500 << 20
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit+(64<<10))
	defer r.Body.Close()
	mr, e := r.MultipartReader()
	if e != nil {
		fail(w, invalid("请选择 3MF 文件进行检查"))
		return
	}
	f, e := os.CreateTemp(filepath.Join(s.Config.DataDir, "uploads"), "inspect-*")
	if e != nil {
		fail(w, e)
		return
	}
	defer func() { f.Close(); os.Remove(f.Name()) }()
	h := sha256.New()
	var size int64
	found := false
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			fail(w, invalid("文件为空、上传中断或超过大小限制"))
			return
		}
		if found || part.FormName() != "file" || !strings.EqualFold(filepath.Ext(part.FileName()), ".3mf") || len([]rune(part.FileName())) > 160 {
			part.Close()
			fail(w, invalid("一次只检查一个 3MF 文件"))
			return
		}
		found = true
		size, e = io.Copy(io.MultiWriter(f, h), io.LimitReader(part, limit+1))
		part.Close()
		if e != nil || size == 0 || size > limit {
			fail(w, invalid("文件为空、上传中断或超过大小限制"))
			return
		}
	}
	if !found {
		fail(w, invalid("请选择 3MF 文件进行检查"))
		return
	}
	if e = f.Close(); e != nil {
		fail(w, e)
		return
	}
	result, e := inspectPrintFile(r.Context(), f.Name(), hex.EncodeToString(h.Sum(nil)), size)
	if e != nil {
		fail(w, e)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	reply(w, 200, map[string]any{"ok": true, "review": result})
}
