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
			if s = importLabel(s, "", 100); s != "" {
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

type fileGeometryReview struct {
	Available bool                `json:"available"`
	Reason    string              `json:"reason,omitempty"`
	Review    *modelImportSummary `json:"review,omitempty"`
}
type fileSliceReview struct {
	Available bool       `json:"available"`
	Reason    string     `json:"reason,omitempty"`
	Info      *SliceInfo `json:"info,omitempty"`
}
type fileReview struct {
	Schema    int                `json:"schema"`
	SHA256    string             `json:"sha256"`
	Bytes     int64              `json:"bytes"`
	Geometry  fileGeometryReview `json:"geometry"`
	Sliced    fileSliceReview    `json:"sliced"`
	Reference *ImportReference   `json:"reference,omitempty"`
}

func inspectPrintFile(ctx context.Context, filename, digest string, size int64) (fileReview, error) {
	out := fileReview{Schema: 1, SHA256: digest, Bytes: size}
	z, e := zip.OpenReader(filename)
	if e != nil {
		return out, invalid("无法识别 3MF 压缩包")
	}
	defer z.Close()
	if len(z.File) > 10000 {
		return out, invalid("3MF 文件条目过多")
	}
	seen := map[string]bool{}
	for _, f := range z.File {
		if e = ctx.Err(); e != nil {
			return out, e
		}
		name := strings.TrimSuffix(f.Name, "/")
		key := strings.ToLower(name)
		if !mfPartName(name) || seen[key] || f.Mode()&os.ModeSymlink != 0 || !f.Mode().IsRegular() && !f.FileInfo().IsDir() {
			return out, invalid("3MF 含不安全或重复路径")
		}
		seen[key] = true
		if f.Name == "Metadata/project_settings.config" {
			b, err := mfRead(ctx, f, 4<<20)
			if err != nil {
				return out, err
			}
			out.Reference = importReference(b)
		}
	}
	if info, err := inspectSlice(filename); err == nil {
		out.Sliced = fileSliceReview{Available: true, Info: &info}
	} else {
		out.Sliced.Reason = err.Error()
	}
	if size > maxProjectImportBytes {
		out.Geometry.Reason = "工程 3MF 须在 32 MB 以内"
	} else {
		raw, err := os.ReadFile(filename)
		if err != nil {
			return out, err
		}
		if p, err := parseModel3MF(ctx, raw); err == nil {
			review := p.summary()
			out.Geometry = fileGeometryReview{Available: true, Review: &review}
		} else {
			out.Geometry.Reason = err.Error()
		}
	}
	if e = ctx.Err(); e != nil {
		return out, e
	}
	return out, nil
}

func (s *Service) inspectFile(w http.ResponseWriter, r *http.Request) {
	if !s.modelImportBusy.CompareAndSwap(false, true) {
		fail(w, conflict("正在解析另一个工程，请稍后重试"))
		return
	}
	defer s.modelImportBusy.Store(false)
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
	filename := ""
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
		filename = part.FileName()
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
	if len([]rune(filename)) > 120 {
		result.Geometry = fileGeometryReview{Reason: "请选择文件名不超过 120 个字符的 3MF"}
	}
	w.Header().Set("Cache-Control", "no-store")
	reply(w, 200, map[string]any{"ok": true, "review": result})
}
