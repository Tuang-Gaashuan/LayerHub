package app

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
)

const objectNamePrefix = "LayerHub-"

func escaped(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// World-space triangles are compiled into one engine object per instance.
// All configuration comes from our bounded profiles and explicit whitelist.
func writeObjectProject(dir string, objects []sliceObject, o SliceSetup) (string, error) {
	project := map[string]any{}
	presets := map[string]string{}
	for _, kind := range []string{"machine", "process", "filament"} {
		raw, e := os.ReadFile(filepath.Join(dir, kind+".json"))
		if e != nil {
			return "", e
		}
		var values map[string]any
		if json.Unmarshal(raw, &values) != nil {
			return "", invalid("对象切片系统配置无效")
		}
		presets[kind] = fmt.Sprint(values["name"])
		for key, value := range values {
			project[key] = value
		}
	}
	for _, key := range []string{"type", "name", "inherits", "from", "include", "instantiation"} {
		delete(project, key)
	}
	project["printer_settings_id"] = presets["machine"]
	project["print_settings_id"] = presets["process"]
	project["filament_settings_id"] = []string{presets["filament"]}
	project["filament_colour"] = []string{o.Color}
	if id, ok := project["filament_id"].(string); ok {
		project["filament_ids"] = []string{id}
	}
	var model, config bytes.Buffer
	model.WriteString(xml.Header + `<model unit="millimeter" xmlns="` + mfCore + `"><metadata name="Application">BambuStudio-02.08.02.61</metadata><metadata name="BambuStudio:3mfVersion">1</metadata><resources>`)
	config.WriteString(xml.Header + `<config>`)
	for i, obj := range objects {
		id := strconv.Itoa(i + 1)
		name := objectNamePrefix + obj.Placement.InstanceID
		fmt.Fprintf(&model, `<object id="%s" type="model"><mesh><vertices>`, id)
		for _, p := range obj.Mesh.Vertices {
			fmt.Fprintf(&model, `<vertex x="%s" y="%s" z="%s"/>`, strconv.FormatFloat(float64(float32(p[0])), 'g', -1, 32), strconv.FormatFloat(float64(float32(p[1])), 'g', -1, 32), strconv.FormatFloat(float64(float32(p[2])), 'g', -1, 32))
		}
		model.WriteString(`</vertices><triangles>`)
		for j := 0; j < len(obj.Mesh.Vertices); j += 3 {
			fmt.Fprintf(&model, `<triangle v1="%d" v2="%d" v3="%d"/>`, j, j+1, j+2)
		}
		model.WriteString(`</triangles></mesh></object>`)
		fmt.Fprintf(&config, `<object id="%s"><metadata key="name" value="%s"/><metadata key="extruder" value="1"/>`, id, escaped(name))
		settings := objectSettings(obj.Placement.Process, o.Layer)
		for _, key := range []string{"wall_loops", "sparse_infill_density", "top_shell_layers", "top_shell_thickness", "bottom_shell_layers", "bottom_shell_thickness", "sparse_infill_pattern"} {
			if value, ok := settings[key]; ok {
				fmt.Fprintf(&config, `<metadata key="%s" value="%s"/>`, key, escaped(value))
			}
		}

		config.WriteString(`</object>`)
	}
	model.WriteString(`</resources><build>`)
	config.WriteString(`<plate><metadata key="plater_id" value="1"/>`)
	for i := range objects {
		id := strconv.Itoa(i + 1)
		fmt.Fprintf(&model, `<item objectid="%s" transform="1 0 0 0 1 0 0 0 1 0 0 0" printable="1"/>`, id)
		fmt.Fprintf(&config, `<model_instance><metadata key="object_id" value="%s"/><metadata key="instance_id" value="0"/><metadata key="identify_id" value="%s"/></model_instance>`, id, id)
	}
	model.WriteString(`</build></model>`)
	config.WriteString(`</plate></config>`)
	settings, e := json.Marshal(project)
	if e != nil {
		return "", e
	}
	var b bytes.Buffer
	archive := zip.NewWriter(&b)
	for _, entry := range []struct {
		name string
		raw  []byte
	}{
		{"3D/3dmodel.model", model.Bytes()}, {"Metadata/model_settings.config", config.Bytes()}, {"Metadata/project_settings.config", settings},
		{"[Content_Types].xml", []byte(xml.Header + `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="model" ContentType="application/vnd.ms-package.3dmanufacturing-3dmodel+xml"/><Default Extension="config" ContentType="application/xml"/></Types>`)},
		{"_rels/.rels", []byte(xml.Header + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Target="/3D/3dmodel.model" Id="rel-1" Type="http://schemas.microsoft.com/3dmanufacturing/2013/01/3dmodel"/></Relationships>`)},
	} {
		w, e := archive.Create(entry.name)
		if e != nil {
			return "", e
		}
		if _, e = w.Write(entry.raw); e != nil {
			return "", e
		}
	}
	if e = archive.Close(); e != nil {
		return "", e
	}
	if b.Len() > 32<<20 {
		return "", invalid("独立对象输入超过 32 MB")
	}
	path := filepath.Join(dir, "objects.3mf")
	return path, os.WriteFile(path, b.Bytes(), 0600)
}

// Match each output triangle with an input triangle, retaining winding and a
// small export rounding tolerance. Spatial buckets avoid a quadratic scan.
func sameObjectGeometry(want, got ModelMesh) bool {
	if len(want.Vertices) != len(got.Vertices) {
		return false
	}
	type key [3]int64
	bucket := func(v []Point3) key {
		var k key
		for axis := 0; axis < 3; axis++ {
			k[axis] = int64(math.Floor((v[0][axis] + v[1][axis] + v[2][axis]) / 3 / .01))
		}
		return k
	}
	index := map[key][]int{}
	for i := 0; i < len(want.Vertices); i += 3 {
		k := bucket(want.Vertices[i : i+3])
		index[k] = append(index[k], i)
	}
	for j := 0; j < len(got.Vertices); j += 3 {
		face := got.Vertices[j : j+3]
		center := bucket(face)
		found := false
		for dx := -1; dx <= 1 && !found; dx++ {
			for dy := -1; dy <= 1 && !found; dy++ {
				for dz := -1; dz <= 1 && !found; dz++ {
					k := key{center[0] + int64(dx), center[1] + int64(dy), center[2] + int64(dz)}
					candidates := index[k]
					for n, i := range candidates {
						match := false
						for shift := 0; shift < 3 && !match; shift++ {
							ok := true
							for v := 0; v < 3; v++ {
								for axis := 0; axis < 3; axis++ {
									if math.Abs(face[v][axis]-want.Vertices[i+(v+shift)%3][axis]) > .002 {
										ok = false
									}
								}
							}
							match = ok
						}
						if match {
							candidates[n] = candidates[len(candidates)-1]
							index[k] = candidates[:len(candidates)-1]
							found = true
							break
						}
					}
				}
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func verifyObjectOutput(filename string, objects []sliceObject, o SliceSetup) error {
	archive, e := zip.OpenReader(filename)
	if e != nil {
		return e
	}
	defer archive.Close()
	entries := map[string]*zip.File{}
	for _, f := range archive.File {
		if entries[f.Name] != nil {
			return invalid("对象结果包含重复文件")
		}
		entries[f.Name] = f
	}
	globalEntry := entries["Metadata/project_settings.config"]
	if globalEntry == nil {
		return invalid("缺少对象切片的整盘配置")
	}
	raw, e := mfRead(context.Background(), globalEntry, 4<<20)
	if e != nil {
		return e
	}
	var global map[string]any
	if json.Unmarshal(raw, &global) != nil {
		return invalid("对象切片的整盘配置无效")
	}
	expectedWalls := 2
	if o.Advanced != nil {
		expectedWalls = o.Advanced.Walls
	}
	if fmt.Sprint(global["wall_loops"]) != strconv.Itoa(expectedWalls) || fmt.Sprint(global["sparse_infill_density"]) != strconv.Itoa(o.Infill)+"%" {
		return conflict("对象切片的整盘继承配置不一致")
	}
	read := func(name string, target any) error {
		f := entries[name]
		if f == nil || f.UncompressedSize64 > 32<<20 {
			return invalid("对象结果缺少文件或过大")
		}
		r, e := f.Open()
		if e != nil {
			return e
		}
		defer r.Close()
		raw, e := io.ReadAll(io.LimitReader(r, 32<<20))
		if e != nil {
			return e
		}
		return mfXML(context.Background(), raw, target)
	}
	var config mfConfig
	if e = read("Metadata/model_settings.config", &config); e != nil {
		return e
	}
	byID := map[string]string{}
	byName := map[string]map[string]string{}
	for _, obj := range config.Objects {
		name := mfMetadata(obj.Metadata, "name")
		if name == "" || byName[name] != nil {
			return invalid("对象结果身份缺失或重复")
		}
		values := map[string]string{}
		for _, m := range obj.Metadata {
			if m.Key != "" {
				if _, ok := values[m.Key]; ok {
					return invalid("对象结果配置键重复")
				}
				values[m.Key] = m.Value
			}
		}
		byID[obj.ID] = name
		byName[name] = values
	}
	if len(byName) != len(objects) {
		return conflict("切片引擎改变了对象数量")
	}
	var main mfDocument
	if e = read("3D/3dmodel.model", &main); e != nil {
		return e
	}
	if main.Unit != "millimeter" || main.XMLName.Space != mfCore {
		return invalid("对象结果单位或命名空间无效")
	}
	if len(main.Items) != len(objects) {
		return conflict("切片引擎改变了实例数量")
	}
	actual := map[string]ModelMesh{}
	for _, item := range main.Items {
		name := byID[item.ObjectID]
		if name == "" || actual[name].Vertices != nil {
			return conflict("切片结果的对象身份无法对应")
		}
		var obj *mfObject
		for i := range main.Objects {
			if main.Objects[i].ID == item.ObjectID {
				obj = &main.Objects[i]
				break
			}
		}
		if obj == nil || len(obj.Components) != 1 {
			return invalid("对象结果几何结构不受支持")
		}
		component := obj.Components[0]
		partName, e := mfResolve("3D/3dmodel.model", component.Path)
		if e != nil {
			return e
		}
		var part mfDocument
		if e = read(partName, &part); e != nil {
			return e
		}
		var mesh *mfObject
		for i := range part.Objects {
			if part.Objects[i].ID == component.ObjectID {
				mesh = &part.Objects[i]
				break
			}
		}
		if mesh == nil || mesh.Mesh == nil || len(mesh.Mesh.Vertices) > maxModelTriangles*3 || len(mesh.Mesh.Triangles) > maxModelTriangles {
			return invalid("对象结果网格无效")
		}
		a, e := mfTransform(component.Transform)
		if e != nil {
			return e
		}
		b, e := mfTransform(item.Transform)
		if e != nil {
			return e
		}
		vertices := make([]Point3, len(mesh.Mesh.Vertices))
		for i, p := range mesh.Mesh.Vertices {
			for axis, v := range []string{p.X, p.Y, p.Z} {
				n, e := mfNumber(v)
				if e != nil {
					return e
				}
				vertices[i][axis] = n
			}
			vertices[i] = b.apply(a.apply(vertices[i]))
		}
		got := ModelMesh{}
		for _, t := range mesh.Mesh.Triangles {
			for _, v := range []string{t.V1, t.V2, t.V3} {
				i, e := strconv.Atoi(v)
				if e != nil || i < 0 || i >= len(vertices) {
					return invalid("对象结果面索引无效")
				}
				got.Vertices = append(got.Vertices, vertices[i])
			}
		}
		actual[name] = got
	}
	for _, obj := range objects {
		name := objectNamePrefix + obj.Placement.InstanceID
		values, ok := byName[name]
		if !ok || !sameObjectGeometry(obj.Mesh, actual[name]) {
			return conflict("切片引擎改变了某个对象的形状、位置或朝向")
		}
		expected := objectSettings(obj.Placement.Process, o.Layer)
		for key, value := range expected {
			if values[key] != value {
				return conflict("对象工艺未被切片引擎正确采用")
			}
		}
		for _, key := range []string{"wall_loops", "sparse_infill_density", "top_shell_layers", "top_shell_thickness", "bottom_shell_layers", "bottom_shell_thickness", "sparse_infill_pattern"} {
			inherited := fmt.Sprint(global[key])
			if _, explicit := expected[key]; !explicit && values[key] != "" && values[key] != inherited {
				return conflict("继承对象被引擎附加了其他工艺")
			}
		}
	}
	return nil
}
