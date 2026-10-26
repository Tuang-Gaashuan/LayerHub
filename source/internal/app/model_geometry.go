package app

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

// Check the actual exported 3MF geometry, including both component and build
// transforms. result.json omits object bounds when automatic arrange is off.
func outputModelBounds(filename string) (ModelBounds, error) {
	empty := ModelBounds{}
	archive, err := zip.OpenReader(filename)
	if err != nil {
		return empty, err
	}
	defer archive.Close()
	read := func(name string, target any) error {
		for _, entry := range archive.File {
			if entry.Name == name {
				if entry.UncompressedSize64 > 32<<20 {
					return invalid("结果模型过大")
				}
				stream, e := entry.Open()
				if e != nil {
					return e
				}
				defer stream.Close()
				return xml.NewDecoder(io.LimitReader(stream, 32<<20)).Decode(target)
			}
		}
		return invalid("结果模型缺少几何文件")
	}
	var root struct {
		Objects []struct {
			Components []struct {
				Path      string `xml:"path,attr"`
				Transform string `xml:"transform,attr"`
			} `xml:"components>component"`
		} `xml:"resources>object"`
		Items []struct {
			Transform string `xml:"transform,attr"`
		} `xml:"build>item"`
	}
	if err = read("3D/3dmodel.model", &root); err != nil {
		return empty, err
	}
	if len(root.Objects) != 1 || len(root.Objects[0].Components) != 1 || len(root.Items) != 1 {
		return empty, invalid("当前只支持单个模型的摆放验证")
	}
	component := root.Objects[0].Components[0]
	name := strings.TrimPrefix(component.Path, "/")
	if !strings.HasPrefix(name, "3D/Objects/") || strings.Contains(name, "..") || strings.Contains(name, "\\") {
		return empty, invalid("结果几何路径无效")
	}
	var model struct {
		Vertices []struct {
			X float64 `xml:"x,attr"`
			Y float64 `xml:"y,attr"`
			Z float64 `xml:"z,attr"`
		} `xml:"resources>object>mesh>vertices>vertex"`
	}
	if err = read(name, &model); err != nil {
		return empty, err
	}
	if len(model.Vertices) == 0 || len(model.Vertices) > maxModelTriangles*3 {
		return empty, invalid("结果网格数量无效")
	}
	matrix := func(text string) ([12]float64, error) {
		m := [12]float64{1, 0, 0, 0, 1, 0, 0, 0, 1, 0, 0, 0}
		if text == "" {
			return m, nil
		}
		parts := strings.Fields(text)
		if len(parts) != 12 {
			return m, invalid("结果变换矩阵无效")
		}
		for i, part := range parts {
			v, e := strconv.ParseFloat(part, 64)
			if e != nil || !finite(v) {
				return m, invalid("结果变换矩阵无效")
			}
			m[i] = v
		}
		return m, nil
	}
	a, err := matrix(component.Transform)
	if err != nil {
		return empty, err
	}
	b, err := matrix(root.Items[0].Transform)
	if err != nil {
		return empty, err
	}
	apply := func(p Point3, m [12]float64) Point3 {
		return Point3{m[0]*p[0] + m[3]*p[1] + m[6]*p[2] + m[9], m[1]*p[0] + m[4]*p[1] + m[7]*p[2] + m[10], m[2]*p[0] + m[5]*p[1] + m[8]*p[2] + m[11]}
	}
	vertices := make([]Point3, len(model.Vertices))
	for i, p := range model.Vertices {
		vertices[i] = apply(apply(Point3{p.X, p.Y, p.Z}, a), b)
	}
	return meshBounds(vertices)
}

const maxModelBytes = 8 << 20
const maxModelTriangles = 150000

type Point3 [3]float64
type ModelBounds struct {
	Min  Point3 `json:"min"`
	Max  Point3 `json:"max"`
	Size Point3 `json:"size"`
}
type ModelMesh struct {
	Vertices []Point3
	Bounds   ModelBounds
}

func meshBounds(vertices []Point3) (ModelBounds, error) {
	b := ModelBounds{Min: Point3{math.Inf(1), math.Inf(1), math.Inf(1)}, Max: Point3{math.Inf(-1), math.Inf(-1), math.Inf(-1)}}
	for _, p := range vertices {
		for k, v := range p {
			if math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) > 100000 {
				return b, invalid("模型含无效坐标或尺寸过大")
			}
			b.Min[k] = math.Min(b.Min[k], v)
			b.Max[k] = math.Max(b.Max[k], v)
		}
	}
	for k := range b.Size {
		b.Size[k] = b.Max[k] - b.Min[k]
		if b.Size[k] < .001 {
			return b, invalid("模型没有有效的三维体积，请检查单位和网格")
		}
	}
	return b, nil
}
func parseModelSTL(raw []byte) (ModelMesh, error) {
	mesh := ModelMesh{}
	if len(raw) == 0 || len(raw) > maxModelBytes {
		return mesh, invalid("STL 文件须在 8 MB 以内")
	}
	if len(raw) >= 84 && uint64(binary.LittleEndian.Uint32(raw[80:84]))*50+84 == uint64(len(raw)) {
		n := int(binary.LittleEndian.Uint32(raw[80:84]))
		if n == 0 || n > maxModelTriangles {
			return mesh, invalid("模型三角面数量须在 1–150000 之间")
		}
		mesh.Vertices = make([]Point3, 0, n*3)
		for i := 0; i < n; i++ {
			for j := 0; j < 3; j++ {
				var p Point3
				for k := 0; k < 3; k++ {
					offset := 84 + i*50 + 12 + j*12 + k*4
					p[k] = float64(math.Float32frombits(binary.LittleEndian.Uint32(raw[offset : offset+4])))
				}
				mesh.Vertices = append(mesh.Vertices, p)
			}
		}
	} else {
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(string(raw[:min(len(raw), 80)]))), "solid") {
			return mesh, invalid("无法识别 STL 文件；请导出二进制或 ASCII STL")
		}
		scan := bufio.NewScanner(bytes.NewReader(raw))
		scan.Buffer(make([]byte, 1024), 64<<10)
		for scan.Scan() {
			fields := strings.Fields(scan.Text())
			if len(fields) == 0 || strings.ToLower(fields[0]) != "vertex" {
				continue
			}
			if len(fields) != 4 {
				return mesh, invalid("STL 顶点格式无效")
			}
			var p Point3
			for k := 0; k < 3; k++ {
				v, e := strconv.ParseFloat(fields[k+1], 64)
				if e != nil {
					return mesh, invalid("STL 坐标无效")
				}
				p[k] = v
			}
			mesh.Vertices = append(mesh.Vertices, p)
			if len(mesh.Vertices) > maxModelTriangles*3 {
				return mesh, invalid("模型超过 150000 个三角面")
			}
		}
		if scan.Err() != nil || len(mesh.Vertices) == 0 || len(mesh.Vertices)%3 != 0 {
			return mesh, invalid("STL 网格不完整")
		}
	}
	var err error
	mesh.Bounds, err = meshBounds(mesh.Vertices)
	return mesh, err
}
func encodeModelSTL(mesh ModelMesh) []byte {
	raw := make([]byte, 84+len(mesh.Vertices)/3*50)
	copy(raw, "LayerHub normalized STL, millimeters")
	binary.LittleEndian.PutUint32(raw[80:84], uint32(len(mesh.Vertices)/3))
	for i := 0; i < len(mesh.Vertices); i += 3 {
		a, b, c := mesh.Vertices[i], mesh.Vertices[i+1], mesh.Vertices[i+2]
		u := Point3{b[0] - a[0], b[1] - a[1], b[2] - a[2]}
		v := Point3{c[0] - a[0], c[1] - a[1], c[2] - a[2]}
		n := Point3{u[1]*v[2] - u[2]*v[1], u[2]*v[0] - u[0]*v[2], u[0]*v[1] - u[1]*v[0]}
		l := math.Sqrt(n[0]*n[0] + n[1]*n[1] + n[2]*n[2])
		if l > 0 {
			for k := range n {
				n[k] /= l
			}
		}
		for j, p := range []Point3{n, a, b, c} {
			for k, x := range p {
				offset := 84 + i/3*50 + j*12 + k*4
				binary.LittleEndian.PutUint32(raw[offset:offset+4], math.Float32bits(float32(x)))
			}
		}
	}
	return raw
}

type SliceSetup struct {
	Material string           `json:"material,omitempty"`
	Advanced *SliceAdvanced   `json:"advanced,omitempty"`
	Copies   []ModelPlacement `json:"copies,omitempty"`
	X        float64          `json:"x"`
	Y        float64          `json:"y"`
	Rotation Point3           `json:"rotation"`
	Scale    float64          `json:"scale"`
	Layer    float64          `json:"layer"`
	Infill   int              `json:"infill"`
	Support  bool             `json:"support"`
	Brim     bool             `json:"brim"`
	Color    string           `json:"color"`
}

func transformSingleModel(mesh ModelMesh, o SliceSetup) (ModelMesh, error) {
	if _, e := sliceMaterial(o.Material); e != nil {
		return mesh, e
	}
	if e := validateSliceAdvanced(o.Advanced); e != nil {
		return mesh, e
	}
	if o.Scale < .01 || o.Scale > 10 || math.IsNaN(o.Scale) || math.IsInf(o.Scale, 0) || !finite(o.X) || !finite(o.Y) {
		return mesh, invalid("缩放或位置无效")
	}
	if o.Layer != .12 && o.Layer != .16 && o.Layer != .2 {
		return mesh, invalid("请选择已支持的层高")
	}
	if o.Infill < 5 || o.Infill > 50 {
		return mesh, invalid("填充密度须在 5%–50% 之间")
	}
	for _, angle := range o.Rotation {
		if !finite(angle) || math.Abs(angle) > 360 {
			return mesh, invalid("旋转角度须在 -360°–360° 之间")
		}
	}
	center := Point3{}
	for k := range center {
		center[k] = (mesh.Bounds.Min[k] + mesh.Bounds.Max[k]) / 2
	}
	cx, sx := math.Cos(o.Rotation[0]*math.Pi/180), math.Sin(o.Rotation[0]*math.Pi/180)
	cy, sy := math.Cos(o.Rotation[1]*math.Pi/180), math.Sin(o.Rotation[1]*math.Pi/180)
	cz, sz := math.Cos(o.Rotation[2]*math.Pi/180), math.Sin(o.Rotation[2]*math.Pi/180)
	vertices := make([]Point3, len(mesh.Vertices))
	for i, p := range mesh.Vertices {
		x, y, z := (p[0]-center[0])*o.Scale, (p[1]-center[1])*o.Scale, (p[2]-center[2])*o.Scale
		x, y = cz*x-sz*y, sz*x+cz*y
		x, z = cy*x+sy*z, -sy*x+cy*z
		y, z = cx*y-sx*z, sx*y+cx*z
		vertices[i] = Point3{x, y, z}
	}
	b, err := meshBounds(vertices)
	if err != nil {
		return mesh, err
	}
	dx, dy, dz := o.X-(b.Min[0]+b.Max[0])/2, o.Y-(b.Min[1]+b.Max[1])/2, -b.Min[2]
	for i := range vertices {
		vertices[i][0] += dx
		vertices[i][1] += dy
		vertices[i][2] += dz
	}
	b, err = meshBounds(vertices)
	if err != nil {
		return mesh, err
	}
	if b.Min[0] < 20.5-.001 || b.Min[1] < -0.001 || b.Max[0] > 256.001 || b.Max[1] > 256.001 || b.Max[2] > 256.001 {
		return mesh, conflict(fmt.Sprintf("模型超出当前喷嘴可用范围（X 20.5–256、Y 0–256、高度 256 mm）；请缩小、旋转或调整位置"))
	}
	return ModelMesh{Vertices: vertices, Bounds: b}, nil
}
func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
