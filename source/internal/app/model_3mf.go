package app

// Geometry-only 3MF reader. Archive data is never extracted, executed, or
// handed to the slicer as configuration. Component/build transforms are baked
// into normalized meshes; plate membership is retained and XY is repacked.
import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"unicode"
)

const maxProjectImportBytes = 32 << 20
const mfCore = "http://schemas.microsoft.com/3dmanufacturing/core/2015/02"
const mfProduction = "http://schemas.microsoft.com/3dmanufacturing/production/2015/06"

type mfMeta struct {
	Key   string `xml:"key,attr"`
	Value string `xml:"value,attr"`
}
type mfRef struct {
	ObjectID  string `xml:"objectid,attr"`
	Path      string `xml:"path,attr"`
	Transform string `xml:"transform,attr"`
	Printable string `xml:"printable,attr"`
}
type mfVertex struct {
	X string `xml:"x,attr"`
	Y string `xml:"y,attr"`
	Z string `xml:"z,attr"`
}
type mfTriangle struct {
	V1    string     `xml:"v1,attr"`
	V2    string     `xml:"v2,attr"`
	V3    string     `xml:"v3,attr"`
	Attrs []xml.Attr `xml:",any,attr"`
}
type mfObject struct {
	ID   string `xml:"id,attr"`
	Name string `xml:"name,attr"`
	Type string `xml:"type,attr"`
	Mesh *struct {
		Vertices  []mfVertex   `xml:"vertices>vertex"`
		Triangles []mfTriangle `xml:"triangles>triangle"`
	} `xml:"mesh"`
	Components []mfRef `xml:"components>component"`
}
type mfDocument struct {
	XMLName  xml.Name   `xml:"model"`
	Unit     string     `xml:"unit,attr"`
	Required string     `xml:"requiredextensions,attr"`
	Attrs    []xml.Attr `xml:",any,attr"`
	Objects  []mfObject `xml:"resources>object"`
	Items    []mfRef    `xml:"build>item"`
}
type mfConfig struct {
	Objects []struct {
		ID       string   `xml:"id,attr"`
		Metadata []mfMeta `xml:"metadata"`
		Parts    []struct {
			Subtype  string   `xml:"subtype,attr"`
			Metadata []mfMeta `xml:"metadata"`
		} `xml:"part"`
	} `xml:"object"`
	Plates []struct {
		Metadata  []mfMeta `xml:"metadata"`
		Instances []struct {
			Metadata []mfMeta `xml:"metadata"`
		} `xml:"model_instance"`
	} `xml:"plate"`
}
type importMesh struct {
	Name string
	Mesh ModelMesh
	Raw  []byte
}
type importBoard struct {
	Name       string
	Objects    []int
	Placements []ModelPlacement
}
type modelImport struct {
	Digest   string
	Sources  []importMesh
	Boards   []importBoard
	Skipped  int
	HasGcode bool
}
type importBoardSummary struct {
	Name      string `json:"name"`
	Objects   int    `json:"objects"`
	Triangles int    `json:"triangles"`
}
type importSourceSummary struct {
	Name      string `json:"name"`
	Triangles int    `json:"triangles"`
	Size      Point3 `json:"size"`
}
type modelImportSummary struct {
	SHA256   string                `json:"sha256"`
	Plates   []importBoardSummary  `json:"plates"`
	Sources  []importSourceSummary `json:"sources"`
	Objects  int                   `json:"objects"`
	Skipped  int                   `json:"skipped"`
	HasGcode bool                  `json:"hasGcode"`
}

func (p modelImport) summary() modelImportSummary {
	r := modelImportSummary{SHA256: p.Digest, Skipped: p.Skipped, HasGcode: p.HasGcode, Sources: []importSourceSummary{}, Plates: []importBoardSummary{}}
	for _, s := range p.Sources {
		r.Sources = append(r.Sources, importSourceSummary{Name: s.Name, Triangles: len(s.Mesh.Vertices) / 3, Size: s.Mesh.Bounds.Size})
	}
	for _, b := range p.Boards {
		n := 0
		for _, i := range b.Objects {
			n += len(p.Sources[i].Mesh.Vertices) / 3
		}
		r.Objects += len(b.Objects)
		r.Plates = append(r.Plates, importBoardSummary{Name: b.Name, Objects: len(b.Objects), Triangles: n})
	}
	return r
}
func importLabel(s, fallback string, limit int) string {
	s = strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
			return -1
		}
		return r
	}, s))
	if s == "" {
		s = fallback
	}
	r := []rune(s)
	if len(r) > limit {
		s = string(r[:limit])
	}
	return s
}
func mfPartName(s string) bool {
	if s == "" || len(s) > 1024 || strings.ContainsAny(s, "\\:\x00?#") || strings.HasPrefix(s, "/") || path.Clean(s) != s {
		return false
	}
	for _, segment := range strings.Split(s, "/") {
		if segment == ".." || segment == "." || segment == "" {
			return false
		}
	}
	return true
}
func mfResolve(part, ref string) (string, error) {
	s, e := url.PathUnescape(ref)
	if e != nil || strings.ContainsAny(s, "\\:\x00?#") {
		return "", invalid("3MF 组件路径无效；不读取外部文件或网络资源")
	}
	for _, segment := range strings.Split(s, "/") {
		if segment == ".." || segment == "." {
			return "", invalid("3MF 组件路径不能越级")
		}
	}
	if strings.HasPrefix(s, "/") {
		s = strings.TrimPrefix(s, "/")
	} else {
		s = path.Join(path.Dir(part), s)
	}
	// Reject traversal before cleaning, including encoded traversal.
	for _, v := range strings.Split(strings.TrimPrefix(ref, "/"), "/") {
		decoded, _ := url.PathUnescape(v)
		if decoded == ".." || decoded == "." {
			return "", invalid("3MF 组件路径不能越级")
		}
	}
	if !mfPartName(s) {
		return "", invalid("3MF 组件路径无效")
	}
	return s, nil
}
func mfRead(ctx context.Context, entry *zip.File, limit uint64) ([]byte, error) {
	if entry == nil || entry.UncompressedSize64 > limit {
		return nil, invalid("3MF 缺少必要文件或解压内容过大")
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	f, e := entry.Open()
	if e != nil {
		return nil, invalid("3MF 压缩内容无法读取")
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, int64(limit)+1))
	if e != nil || uint64(len(b)) > limit {
		return nil, invalid("3MF 内容损坏或超过读取限制")
	}
	return b, nil
}
func mfXML(ctx context.Context, raw []byte, out any) error {
	// Bound expansion and reject directives; Go's decoder never resolves external
	// entities. A token preflight also bounds allocations before struct decoding.
	d := xml.NewDecoder(bytes.NewReader(raw))
	_, isModel := out.(*mfDocument)
	type frame struct {
		name string
		seen uint8
	}
	stack := []frame{}
	parents := map[string]string{"model": "", "resources": "model", "build": "model", "object": "resources", "mesh": "object", "components": "object", "vertices": "mesh", "triangles": "mesh", "vertex": "vertices", "triangle": "triangles", "component": "components", "item": "build"}
	roots := 0
	vertices, triangles, objects, nodes, depth := 0, 0, 0, 0, 0
	for {
		t, e := d.Token()
		if e == io.EOF {
			break
		}
		if e != nil {
			return invalid("3MF XML 不完整或格式无效")
		}
		nodes++
		if nodes%4096 == 0 {
			if e = ctx.Err(); e != nil {
				return e
			}
		}
		switch v := t.(type) {
		case xml.Directive:
			return invalid("3MF 不接受 XML 指令或外部实体")
		case xml.ProcInst:
			if v.Target != "xml" {
				return invalid("3MF 不接受处理指令")
			}
		case xml.StartElement:
			if depth == 0 {
				roots++
				if roots > 1 {
					return invalid("3MF XML 不能包含多个根节点")
				}
			}
			depth++
			seenAttrs := map[xml.Name]bool{}
			for _, a := range v.Attr {
				if seenAttrs[a.Name] {
					return invalid("3MF XML 属性重复")
				}
				seenAttrs[a.Name] = true
			}
			if isModel {
				if parent, known := parents[v.Name.Local]; known {
					actual := ""
					if len(stack) > 0 {
						actual = stack[len(stack)-1].name
					}
					if actual != parent {
						return invalid("3MF 网格层级结构无效")
					}
					bit := uint8(0)
					if actual == "model" || actual == "object" || actual == "mesh" {
						switch v.Name.Local {
						case "resources", "mesh", "vertices":
							bit = 1
						case "build", "components", "triangles":
							bit = 2
						}
					}
					if bit != 0 {
						prior := &stack[len(stack)-1]
						if prior.seen&bit != 0 {
							return invalid("3MF 含重复的网格或打印结构")
						}
						prior.seen |= bit
					}
				}
				stack = append(stack, frame{name: v.Name.Local})
				for _, a := range v.Attr {
					switch a.Name.Local {
					case "id", "unit", "requiredextensions", "objectid", "transform", "printable", "x", "y", "z", "v1", "v2", "v3":
						if a.Name.Space != "" {
							return invalid("3MF 核心属性命名空间无效")
						}
					}
				}
				switch v.Name.Local {
				case "model", "resources", "object", "mesh", "vertices", "vertex", "triangles", "triangle", "components", "component", "build", "item":
					if v.Name.Space != mfCore {
						return invalid("3MF 网格结构命名空间无效")
					}
				}
				if v.Name.Local == "component" || v.Name.Local == "item" {
					for _, a := range v.Attr {
						if a.Name.Local == "path" && a.Name.Space != mfProduction {
							return invalid("3MF 组件路径扩展无效")
						}
					}
				}
			}
			if depth > 40 || len(v.Attr) > 64 {
				return invalid("3MF XML 层级或属性过多")
			}
			switch v.Name.Local {
			case "vertex":
				vertices++
			case "triangle":
				triangles++
			case "object":
				objects++
			}
			if vertices > 900000 || triangles > 600000 || objects > 1024 || nodes > 4000000 {
				return invalid("3MF 单文件网格或对象数量超限")
			}
		case xml.EndElement:
			depth--
			if isModel {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			if depth == 0 && strings.TrimSpace(string(v)) != "" {
				return invalid("3MF XML 根节点之外有多余内容")
			}
		}
	}
	if e := xml.Unmarshal(raw, out); e != nil {
		return invalid("3MF XML 结构无效")
	}
	return nil
}
func mfNumber(s string) (float64, error) {
	v, e := strconv.ParseFloat(s, 64)
	if e != nil || !finite(v) || math.Abs(v) > 100000 {
		return 0, invalid("3MF 坐标或矩阵无效")
	}
	return v, nil
}
func mfID(s string) (int, error) {
	i, e := strconv.Atoi(s)
	if e != nil || i <= 0 {
		return 0, invalid("3MF 对象编号无效")
	}
	return i, nil
}

type mfMatrix [12]float64

func mfTransform(s string) (mfMatrix, error) {
	m := mfMatrix{1, 0, 0, 0, 1, 0, 0, 0, 1, 0, 0, 0}
	if s == "" {
		return m, nil
	}
	parts := strings.Fields(s)
	if len(parts) != 12 {
		return m, invalid("3MF 变换矩阵需要 12 个数")
	}
	for i, p := range parts {
		v, e := mfNumber(p)
		if e != nil {
			return m, e
		}
		m[i] = v
	}
	if math.Abs(m.det()) < 1e-12 {
		return m, invalid("3MF 包含不可逆的缩放或变换")
	}
	return m, nil
}
func (m mfMatrix) det() float64 {
	return m[0]*(m[4]*m[8]-m[5]*m[7]) - m[3]*(m[1]*m[8]-m[2]*m[7]) + m[6]*(m[1]*m[5]-m[2]*m[4])
}
func (m mfMatrix) apply(v Point3) Point3 {
	return Point3{m[0]*v[0] + m[3]*v[1] + m[6]*v[2] + m[9], m[1]*v[0] + m[4]*v[1] + m[7]*v[2] + m[10], m[2]*v[0] + m[5]*v[1] + m[8]*v[2] + m[11]}
}
func mfApply(mesh ModelMesh, m mfMatrix) ModelMesh {
	r := ModelMesh{Vertices: make([]Point3, len(mesh.Vertices))}
	for i, p := range mesh.Vertices {
		r.Vertices[i] = m.apply(p)
	}
	if m.det() < 0 {
		for i := 0; i < len(r.Vertices); i += 3 {
			r.Vertices[i+1], r.Vertices[i+2] = r.Vertices[i+2], r.Vertices[i+1]
		}
	}
	return r
}
func mfMetadata(items []mfMeta, key string) string {
	for _, v := range items {
		if v.Key == key {
			return v.Value
		}
	}
	return ""
}

func parseModel3MF(ctx context.Context, raw []byte) (modelImport, error) {
	p := modelImport{}
	fail := func(message string) (modelImport, error) { return p, invalid(message) }
	if len(raw) == 0 || len(raw) > maxProjectImportBytes {
		return fail("工程 3MF 须在 32 MB 以内")
	}
	digest := sha256.Sum256(raw)
	p.Digest = hex.EncodeToString(digest[:])
	z, e := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if e != nil {
		return fail("无法识别 3MF 压缩包")
	}
	if len(z.File) > 512 {
		return fail("3MF 文件条目过多")
	}
	files := map[string]*zip.File{}
	folded := map[string]bool{}
	total := uint64(0)
	for _, f := range z.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if !mfPartName(f.Name) || f.Mode()&os.ModeSymlink != 0 {
			return fail("3MF 含无效路径或链接文件")
		}
		key := strings.ToLower(f.Name)
		if strings.Contains(key, "slic3r_pe_model.config") || strings.Contains(key, "cura_settings") {
			return fail("此工程含暂不支持的切片器对象设置，可能包含修改器；请在原软件导出实体 STL，或另存普通网格 3MF。")
		}
		if folded[key] {
			return fail("3MF 包含重复文件路径")
		}
		folded[key] = true
		if f.UncompressedSize64 > 128<<20-total {
			return fail("3MF 解压内容超过 128 MB")
		}
		total += f.UncompressedSize64
		files[f.Name] = f
		if strings.HasSuffix(strings.ToLower(f.Name), ".gcode") {
			p.HasGcode = true
		}
	}
	if files["[Content_Types].xml"] == nil {
		return fail("缺少 3MF 内容类型文件")
	}
	rels, e := mfRead(ctx, files["_rels/.rels"], 1<<20)
	if e != nil {
		return p, e
	}
	var relationships struct {
		Items []struct {
			Type   string `xml:"Type,attr"`
			Target string `xml:"Target,attr"`
			Mode   string `xml:"TargetMode,attr"`
		} `xml:"Relationship"`
	}
	if e = mfXML(ctx, rels, &relationships); e != nil {
		return p, e
	}
	rootName := ""
	for _, r := range relationships.Items {
		if r.Type == "http://schemas.microsoft.com/3dmanufacturing/2013/01/3dmodel" {
			if rootName != "" || r.Mode != "" && r.Mode != "Internal" {
				return fail("3MF 主模型关系无效，不读取外部模型")
			}
			rootName, e = mfResolve("root", r.Target)
			if e != nil {
				return p, e
			}
		}
	}
	if rootName == "" {
		return fail("3MF 缺少主模型关系")
	}
	docs := map[string]*mfDocument{}
	unitScale := 0.0
	allFaces := 0
	var document func(string) (*mfDocument, error)
	document = func(name string) (*mfDocument, error) {
		if d := docs[name]; d != nil {
			return d, nil
		}
		b, e := mfRead(ctx, files[name], 32<<20)
		if e != nil {
			return nil, e
		}
		var d mfDocument
		if e = mfXML(ctx, b, &d); e != nil {
			return nil, e
		}
		if d.XMLName.Space != mfCore {
			return nil, invalid("不支持的 3MF 模型命名空间")
		}
		units := map[string]float64{"": 1, "millimeter": 1, "micron": .001, "centimeter": 10, "meter": 1000, "inch": 25.4, "foot": 304.8}
		scale, ok := units[d.Unit]
		if !ok {
			return nil, invalid("3MF 长度单位不受支持")
		}
		if unitScale == 0 {
			unitScale = scale
		} else if scale != unitScale {
			return nil, invalid("3MF 子模型的长度单位不一致")
		}
		ns := map[string]string{}
		for _, a := range d.Attrs {
			if a.Name.Space == "xmlns" {
				ns[a.Name.Local] = a.Value
			}
		}
		for _, prefix := range strings.Fields(d.Required) {
			if ns[prefix] != mfProduction {
				return nil, invalid("3MF 要求暂不支持的扩展，请导出普通网格工程")
			}
		}
		seen := map[int]bool{}
		for _, o := range d.Objects {
			id, e := mfID(o.ID)
			if e != nil || seen[id] {
				return nil, invalid("3MF 对象编号重复或无效")
			}
			seen[id] = true
			if o.Mesh != nil {
				allFaces += len(o.Mesh.Triangles)
			}
			if allFaces > 600000 {
				return nil, invalid("3MF 源网格合计超过 600,000 个三角面")
			}
		}
		docs[name] = &d
		return &d, nil
	}
	root, e := document(rootName)
	if e != nil {
		return p, e
	}
	if len(root.Items) == 0 {
		if p.HasGcode {
			return fail("该文件只有切片路径，无法恢复可编辑模型。请在 Bambu Studio 另存工程 3MF；直接打印请使用看板上传。")
		}
		return fail("3MF 不包含可编辑的打印对象")
	}
	if len(root.Items) > 256 {
		return fail("一个工程最多 256 个打印对象")
	}
	var config mfConfig
	if files["Metadata/model_settings.config"] != nil {
		b, e := mfRead(ctx, files["Metadata/model_settings.config"], 4<<20)
		if e != nil {
			return p, e
		}
		if e = mfXML(ctx, b, &config); e != nil {
			return p, e
		}
	}
	names := map[string]string{}
	for _, o := range config.Objects {
		names[o.ID] = mfMetadata(o.Metadata, "name")
		for _, part := range o.Parts {
			if part.Subtype != "" && part.Subtype != "normal_part" {
				return fail("工程包含修改器、负体积或支撑辅助体，暂不能作为普通模型导入；请在 Studio 移除辅助体或导出实体 STL。")
			}
			if v := mfMetadata(part.Metadata, "volume_type"); v != "" && v != "0" {
				return fail("工程包含暂不支持的特殊体积，请先在 Studio 整理为普通实体")
			}
		}
	}
	cache := map[string]ModelMesh{}
	visiting := map[string]bool{}
	cachedFaces := 0
	var meshFor func(string, string, int) (ModelMesh, error)
	meshFor = func(part, idText string, depth int) (ModelMesh, error) {
		empty := ModelMesh{}
		if depth > 16 {
			return empty, invalid("3MF 组件嵌套过深")
		}
		key := part + ":" + idText
		if m, ok := cache[key]; ok {
			return m, nil
		}
		if visiting[key] {
			return empty, invalid("3MF 组件循环引用")
		}
		visiting[key] = true
		defer delete(visiting, key)
		d, e := document(part)
		if e != nil {
			return empty, e
		}
		var o *mfObject
		for i := range d.Objects {
			if d.Objects[i].ID == idText {
				o = &d.Objects[i]
				break
			}
		}
		if o == nil {
			return empty, invalid("3MF 引用了不存在的对象")
		}
		if o.Type != "" && o.Type != "model" {
			return empty, invalid("3MF 含非实体模型对象，请先导出普通网格")
		}
		m := ModelMesh{}
		if o.Mesh != nil {
			if len(o.Components) > 0 {
				return empty, invalid("3MF 对象不能同时含网格和组件")
			}
			v := make([]Point3, len(o.Mesh.Vertices))
			for i, p := range o.Mesh.Vertices {
				for k, s := range []string{p.X, p.Y, p.Z} {
					n, e := mfNumber(s)
					if e != nil {
						return empty, e
					}
					v[i][k] = n
				}
			}
			for _, face := range o.Mesh.Triangles {
				for _, a := range face.Attrs {
					if a.Value != "" {
						return empty, invalid("模型包含逐面颜色、绘制支撑或其他三角面扩展；请导出普通 STL 后导入")
					}
				}
				for _, s := range []string{face.V1, face.V2, face.V3} {
					i, e := strconv.Atoi(s)
					if e != nil || i < 0 || i >= len(v) {
						return empty, invalid("3MF 三角面顶点索引无效")
					}
					m.Vertices = append(m.Vertices, v[i])
				}
			}
		} else {
			if len(o.Components) == 0 || len(o.Components) > 256 {
				return empty, invalid("3MF 对象缺少网格或组件过多")
			}
			for _, c := range o.Components {
				next := part
				if c.Path != "" {
					next, e = mfResolve(part, c.Path)
					if e != nil {
						return empty, e
					}
				}
				if _, e = mfID(c.ObjectID); e != nil {
					return empty, e
				}
				child, e := meshFor(next, c.ObjectID, depth+1)
				if e != nil {
					return empty, e
				}
				matrix, e := mfTransform(c.Transform)
				if e != nil {
					return empty, e
				}
				if len(m.Vertices)+len(child.Vertices) > maxModelTriangles*3 {
					return empty, invalid("3MF 单个组合对象超过 150,000 个三角面")
				}
				m.Vertices = append(m.Vertices, mfApply(child, matrix).Vertices...)
			}
		}
		if len(m.Vertices) == 0 || len(m.Vertices)/3 > maxModelTriangles {
			return empty, invalid("3MF 网格为空或三角面过多")
		}
		cachedFaces += len(m.Vertices) / 3
		if cachedFaces > 1200000 {
			return empty, invalid("3MF 组件展开量过大")
		}
		cache[key] = m
		return m, nil
	}
	// Match Bambu plate metadata to the build's per-object instance ordinal.
	assignments := map[string]int{}
	if len(config.Plates) > maxProjectPlates {
		return fail("工程最多支持 8 块打印板")
	}
	for i, b := range config.Plates {
		p.Boards = append(p.Boards, importBoard{Name: importLabel(mfMetadata(b.Metadata, "plater_name"), fmt.Sprintf("打印板 %02d", i+1), 40)})
		for _, instance := range b.Instances {
			objectID := mfMetadata(instance.Metadata, "object_id")
			if _, e = mfID(objectID); e != nil {
				return p, e
			}
			n, e := strconv.Atoi(mfMetadata(instance.Metadata, "instance_id"))
			if e != nil || n < 0 {
				return fail("3MF 打印板实例编号无效")
			}
			key := objectID + ":" + strconv.Itoa(n)
			if _, exists := assignments[key]; exists {
				return fail("同一对象实例被重复分配到打印板")
			}
			assignments[key] = i
		}
	}
	if len(p.Boards) == 0 {
		p.Boards = []importBoard{{Name: "打印板 01"}}
	}
	instances := map[string]int{}
	normalizedBytes := 0
	usedAssignments := map[string]bool{}
	sourceHashes := map[[32]byte]int{}
	for _, item := range root.Items {
		if e = ctx.Err(); e != nil {
			return p, e
		}
		if _, e = mfID(item.ObjectID); e != nil {
			return p, e
		}
		instance := instances[item.ObjectID]
		instances[item.ObjectID]++
		key := item.ObjectID + ":" + strconv.Itoa(instance)
		board, assigned := assignments[key]
		if len(config.Plates) > 0 && !assigned {
			return fail("对象的打印板归属不完整，请先在 Studio 重新保存工程")
		}
		if assigned {
			usedAssignments[key] = true
		}
		if item.Printable == "0" || item.Printable == "false" {
			p.Skipped++
			continue
		}
		if item.Printable != "" && item.Printable != "1" && item.Printable != "true" {
			return fail("3MF printable 标记无效")
		}
		part := rootName
		if item.Path != "" {
			part, e = mfResolve(rootName, item.Path)
			if e != nil {
				return p, e
			}
		}
		mesh, e := meshFor(part, item.ObjectID, 0)
		if e != nil {
			return p, e
		}
		matrix, e := mfTransform(item.Transform)
		if e != nil {
			return p, e
		}
		mesh = mfApply(mesh, matrix)
		for i := range mesh.Vertices {
			for k := range mesh.Vertices[i] {
				mesh.Vertices[i][k] *= unitScale
			}
		}
		b, e := meshBounds(mesh.Vertices)
		if e != nil {
			return p, e
		}
		for i := range mesh.Vertices {
			mesh.Vertices[i][0] -= (b.Min[0] + b.Max[0]) / 2
			mesh.Vertices[i][1] -= (b.Min[1] + b.Max[1]) / 2
			mesh.Vertices[i][2] -= b.Min[2]
		}
		mesh.Bounds, _ = meshBounds(mesh.Vertices)
		stl := encodeModelSTL(mesh)
		mesh, e = parseModelSTL(stl)
		if e != nil {
			return p, e
		}
		hash := sha256.Sum256(stl)
		source, exists := sourceHashes[hash]
		if !exists {
			normalizedBytes += len(stl)
			if normalizedBytes > 128<<20 {
				return fail("工程展开后的模型超过 128 MB，请拆分工程")
			}
			if len(p.Sources) >= 30 {
				return fail("工程包含超过 30 种变换后的模型，请拆分工程")
			}
			source = len(p.Sources)
			sourceHashes[hash] = source
			name := names[item.ObjectID]
			if name == "" {
				d, _ := document(part)
				for _, o := range d.Objects {
					if o.ID == item.ObjectID {
						name = o.Name
					}
				}
			}
			p.Sources = append(p.Sources, importMesh{Name: importLabel(name, fmt.Sprintf("工程对象 %d", source+1), 100), Mesh: mesh, Raw: stl})
		}
		p.Boards[board].Objects = append(p.Boards[board].Objects, source)
	}
	if len(usedAssignments) != len(assignments) {
		return fail("打印板引用了不存在的模型实例，请在 Studio 重新保存工程")
	}
	if len(p.Sources) == 0 {
		return fail("工程没有启用打印的实体模型")
	}
	for i := range p.Boards {
		if e = arrangeImportedBoard(&p.Boards[i], p.Sources); e != nil {
			return p, invalid(fmt.Sprintf("%s：%s", p.Boards[i].Name, e.Error()))
		}
	}
	return p, nil
}

func arrangeImportedBoard(b *importBoard, sources []importMesh) error {
	if len(b.Objects) > 64 {
		return invalid("一盘最多 64 个对象")
	}
	x, y, row, maxX, maxY, faces := 20.5, 0.0, 0.0, 20.5, 0.0, 0
	for _, index := range b.Objects {
		s := sources[index]
		faces += len(s.Mesh.Vertices) / 3
		size := s.Mesh.Bounds.Size
		if faces > maxModelTriangles {
			return invalid("整盘超过 150,000 个三角面")
		}
		if size[0] > 235.5 || size[1] > 256 || size[2] > 256 {
			return invalid("模型超出 X2D 可打印尺寸，请在 Studio 缩小后再导入")
		}
		if x+size[0] > 256+.001 {
			x = 20.5
			y += row + 5
			row = 0
		}
		if y+size[1] > 256+.001 {
			return invalid("当前板放不下，请先在 Studio 将对象拆到更多盘后导入")
		}
		b.Placements = append(b.Placements, ModelPlacement{X: x + size[0]/2, Y: y + size[1]/2, Scale: 1})
		x += size[0] + 5
		row = math.Max(row, size[1])
		maxX = math.Max(maxX, x-5)
		maxY = math.Max(maxY, y+size[1])
	}
	for i := range b.Placements {
		b.Placements[i].X = math.Round((b.Placements[i].X+(256-maxX)/2)*1000) / 1000
		b.Placements[i].Y = math.Round((b.Placements[i].Y+(256-maxY)/2)*1000) / 1000
	}
	return nil
}
