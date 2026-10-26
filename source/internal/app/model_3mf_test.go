package app

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
)

func mfMeshXML(mesh ModelMesh, id string) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<object id="%s" type="model"><mesh><vertices>`, id)
	for _, v := range mesh.Vertices {
		fmt.Fprintf(&b, `<vertex x="%.9g" y="%.9g" z="%.9g"/>`, v[0], v[1], v[2])
	}
	b.WriteString(`</vertices><triangles>`)
	for i := 0; i < len(mesh.Vertices); i += 3 {
		fmt.Fprintf(&b, `<triangle v1="%d" v2="%d" v3="%d"/>`, i, i+1, i+2)
	}
	b.WriteString(`</triangles></mesh></object>`)
	return b.String()
}
func mfFixtureFiles() map[string]string {
	head := `<model unit="millimeter" xmlns="` + mfCore + `" xmlns:p="` + mfProduction + `" requiredextensions="p">`
	return map[string]string{
		"[Content_Types].xml":              `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="model" ContentType="application/vnd.ms-package.3dmanufacturing-3dmodel+xml"/></Types>`,
		"_rels/.rels":                      `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="model" Type="http://schemas.microsoft.com/3dmanufacturing/2013/01/3dmodel" Target="/3D/3dmodel.model"/></Relationships>`,
		"3D/3dmodel.model":                 head + `<resources><object id="10" type="model"><components><component objectid="1" p:path="/3D/Objects/cube.model" transform="0 1 0 -1 0 0 0 0 1 0 0 5"/></components></object><object id="20" type="model"><components><component objectid="1" p:path="/3D/Objects/box.model"/></components></object></resources><build><item objectid="10" transform="1 0 0 0 1 0 0 0 1 80 100 -5"/><item objectid="10" transform="1 0 0 0 1 0 0 0 1 125 100 -5"/><item objectid="20" transform="0 1 0 -1 0 0 0 0 1 600 -100 0"/></build></model>`,
		"3D/Objects/cube.model":            head + `<resources>` + mfMeshXML(workbenchMesh(), "1") + `</resources></model>`,
		"3D/Objects/box.model":             head + `<resources>` + mfMeshXML(mixedMesh(), "1") + `</resources></model>`,
		"Metadata/model_settings.config":   `<config><object id="10"><metadata key="name" value="外壳 &amp; 支架"/><part id="1" subtype="normal_part"/></object><object id="20"><metadata key="name" value="底座"/><part id="1" subtype="normal_part"/></object><plate><metadata key="plater_id" value="1"/><metadata key="plater_name" value="外壳盘"/><model_instance><metadata key="object_id" value="10"/><metadata key="instance_id" value="0"/></model_instance><model_instance><metadata key="object_id" value="10"/><metadata key="instance_id" value="1"/></model_instance></plate><plate><metadata key="plater_id" value="2"/><metadata key="plater_name" value="底座盘"/><model_instance><metadata key="object_id" value="20"/><metadata key="instance_id" value="0"/></model_instance></plate><plate><metadata key="plater_id" value="3"/><metadata key="plater_name" value="备用空盘"/></plate></config>`,
		"Metadata/project_settings.config": `{"printer_model":"Bambu Lab A1 mini","filament_type":["PLA"],"layer_height":"0.08","nozzle_diameter":["0.2"],"machine_start_gcode":"; ORIGINAL_PROFILE_SENTINEL"}`,
	}
}
func mfTestZIP(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	names := []string{}
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		f, e := w.Create(name)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = f.Write([]byte(files[name])); e != nil {
			t.Fatal(e)
		}
	}
	if e := w.Close(); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func TestModel3MFProductionGeometryAndBoards(t *testing.T) {
	raw := mfTestZIP(t, mfFixtureFiles())
	p, e := parseModel3MF(context.Background(), raw)
	if e != nil {
		t.Fatal(e)
	}
	if len(p.Sources) != 2 || len(p.Boards) != 3 || p.summary().Objects != 3 || len(p.Boards[0].Objects) != 2 || len(p.Boards[2].Objects) != 0 {
		t.Fatal(p.summary())
	}
	if p.Sources[0].Mesh.Bounds.Size != (Point3{20, 20, 10}) || p.Sources[1].Mesh.Bounds.Size != (Point3{12, 40, 18}) {
		t.Fatal("component/build rotation lost", p.summary())
	}
	if p.Boards[0].Objects[0] != p.Boards[0].Objects[1] || p.Boards[0].Name != "外壳盘" || p.Sources[0].Name != "外壳 & 支架" {
		t.Fatal("instances, labels, or dedup lost")
	}
	if dir := os.Getenv("LAYERHUB_WRITE_3MF_FIXTURE"); dir != "" {
		if e = os.WriteFile(dir, raw, 0600); e != nil {
			t.Fatal(e)
		}
	}
}
func TestModel3MFCoreUnitsMirrorAndNestedTransforms(t *testing.T) {
	f := mfFixtureFiles()
	delete(f, "Metadata/model_settings.config")
	f["3D/3dmodel.model"] = `<model unit="centimeter" xmlns="` + mfCore + `"><resources>` + mfMeshXML(workbenchMesh(), "1") + `<object id="2"><components><component objectid="1" transform="-0.5 0 0 0 0.5 0 0 0 0.5 2 3 4"/></components></object><object id="3"><components><component objectid="2" transform="1 0 0 0 1 0 0 0 1 5 6 7"/></components></object></resources><build><item objectid="3" transform="1 0 0 0 1 0 0 0 1 10 20 30"/></build></model>`
	p, e := parseModel3MF(context.Background(), mfTestZIP(t, f))
	if e != nil {
		t.Fatal(e)
	}
	m := p.Sources[0].Mesh
	if m.Bounds.Size != (Point3{100, 100, 50}) || m.Bounds.Min != (Point3{-50, -50, 0}) {
		t.Fatal(m.Bounds)
	}
	volume := 0.0
	for i := 0; i < len(m.Vertices); i += 3 {
		a, b, c := m.Vertices[i], m.Vertices[i+1], m.Vertices[i+2]
		volume += (a[0]*(b[1]*c[2]-b[2]*c[1]) + a[1]*(b[2]*c[0]-b[0]*c[2]) + a[2]*(b[0]*c[1]-b[1]*c[0])) / 6
	}
	if volume <= 0 {
		t.Fatal("mirror inverted triangle winding", volume)
	}
}
func TestModel3MFRejectsAmbiguousOrUnsafeGeometry(t *testing.T) {
	cases := map[string]func(map[string]string){
		"external root": func(f map[string]string) {
			f["_rels/.rels"] = strings.Replace(f["_rels/.rels"], `Target="/3D/3dmodel.model"`, `Target="https://example.invalid/x.model" TargetMode="External"`, 1)
		},
		"encoded traversal": func(f map[string]string) {
			f["3D/3dmodel.model"] = strings.Replace(f["3D/3dmodel.model"], "/3D/Objects/cube.model", "%2e%2e%2foutside.model", 1)
		},
		"archive traversal": func(f map[string]string) { f["../evil.model"] = "bad" },
		"case collision":    func(f map[string]string) { f["3D/OBJECTS/CUBE.MODEL"] = f["3D/Objects/cube.model"] },
		"required extension": func(f map[string]string) {
			f["3D/3dmodel.model"] = strings.Replace(f["3D/3dmodel.model"], `requiredextensions="p"`, `xmlns:x="https://example.invalid/extension" requiredextensions="x"`, 1)
		},
		"foreign mesh namespace": func(f map[string]string) {
			f["3D/Objects/cube.model"] = strings.ReplaceAll(f["3D/Objects/cube.model"], mfCore, "https://example.invalid/core")
		},
		"DTD": func(f map[string]string) {
			f["3D/Objects/cube.model"] = `<!DOCTYPE model [<!ENTITY a SYSTEM "file:///etc/passwd">]>` + f["3D/Objects/cube.model"]
		},
		"duplicate XML attribute": func(f map[string]string) {
			f["3D/Objects/cube.model"] = strings.Replace(f["3D/Objects/cube.model"], `id="1"`, `id="1" id="2"`, 1)
		},
		"duplicate build": func(f map[string]string) {
			f["3D/3dmodel.model"] = strings.Replace(f["3D/3dmodel.model"], "</model>", "<build/></model>", 1)
		},
		"multiple root nodes": func(f map[string]string) { f["3D/3dmodel.model"] += `<model xmlns="` + mfCore + `"/>` },
		"unadapted modifier format": func(f map[string]string) {
			f["Metadata/Slic3r_PE_model.config"] = `<config><volume type="modifier"/></config>`
		},
		"foreign vertex attribute": func(f map[string]string) {
			f["3D/Objects/cube.model"] = strings.Replace(f["3D/Objects/cube.model"], `x="-10"`, `xmlns:e="https://example.invalid/extension" e:x="-10"`, 1)
		},
		"NaN": func(f map[string]string) {
			f["3D/Objects/cube.model"] = strings.Replace(f["3D/Objects/cube.model"], `x="-10"`, `x="NaN"`, 1)
		},
		"missing vertex coordinate": func(f map[string]string) {
			f["3D/Objects/cube.model"] = strings.Replace(f["3D/Objects/cube.model"], `x="-10"`, "", 1)
		},
		"triangle index": func(f map[string]string) {
			f["3D/Objects/cube.model"] = strings.Replace(f["3D/Objects/cube.model"], `v1="0"`, `v1="9999"`, 1)
		},
		"painted support": func(f map[string]string) {
			f["3D/Objects/cube.model"] = strings.Replace(f["3D/Objects/cube.model"], `v1="0"`, `v1="0" paint_supports="1"`, 1)
		},
		"negative volume": func(f map[string]string) {
			f["Metadata/model_settings.config"] = strings.Replace(f["Metadata/model_settings.config"], "normal_part", "negative_part", 1)
		},
		"invalid transform": func(f map[string]string) {
			f["3D/3dmodel.model"] = strings.Replace(f["3D/3dmodel.model"], "0 1 0 -1 0 0 0 0 1 0 0 5", "0 0 0 0 0 0 0 0 0 0 0 0", 1)
		},
		"component cycle": func(f map[string]string) {
			f["3D/Objects/cube.model"] = `<model xmlns="` + mfCore + `"><resources><object id="1"><components><component objectid="1"/></components></object></resources></model>`
		},
		"missing component": func(f map[string]string) { delete(f, "3D/Objects/cube.model") },
		"different units": func(f map[string]string) {
			f["3D/Objects/cube.model"] = strings.Replace(f["3D/Objects/cube.model"], "millimeter", "inch", 1)
		},
		"duplicate assignment": func(f map[string]string) {
			f["Metadata/model_settings.config"] = strings.Replace(f["Metadata/model_settings.config"], `value="1"/></model_instance>`, `value="0"/></model_instance>`, 1)
		},
		"missing assignment": func(f map[string]string) {
			f["Metadata/model_settings.config"] = strings.Replace(f["Metadata/model_settings.config"], `value="20"`, `value="99"`, 1)
		},
		"unknown instance": func(f map[string]string) {
			f["Metadata/model_settings.config"] = strings.Replace(f["Metadata/model_settings.config"], `value="10"/><metadata key="instance_id" value="0"`, `value="10"/><metadata key="instance_id" value="3"`, 1)
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			f := mfFixtureFiles()
			change(f)
			if _, e := parseModel3MF(context.Background(), mfTestZIP(t, f)); e == nil {
				t.Fatal("accepted unsupported/unsafe geometry")
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := parseModel3MF(ctx, mfTestZIP(t, mfFixtureFiles())); e == nil {
		t.Fatal("cancelled parser accepted")
	}
}
func TestModel3MFDisabledObjectsGcodeAndPackingLimits(t *testing.T) {
	f := mfFixtureFiles()
	f["3D/3dmodel.model"] = strings.Replace(f["3D/3dmodel.model"], `<item objectid="20"`, `<item objectid="20" printable="0"`, 1)
	p, e := parseModel3MF(context.Background(), mfTestZIP(t, f))
	if e != nil || p.Skipped != 1 || p.summary().Objects != 2 || len(p.Boards[1].Objects) != 0 {
		t.Fatal(p.summary(), e)
	}
	f = mfFixtureFiles()
	f["Metadata/plate_1.gcode"] = "G1 X999"
	root := f["3D/3dmodel.model"]
	start, end := strings.Index(root, "<build>"), strings.Index(root, "</build>")
	f["3D/3dmodel.model"] = root[:start] + "<build>" + root[end:]
	if _, e = parseModel3MF(context.Background(), mfTestZIP(t, f)); e == nil || !strings.Contains(e.Error(), "只有切片路径") {
		t.Fatal(e)
	}
	f = mfFixtureFiles()
	delete(f, "Metadata/model_settings.config")
	root = f["3D/3dmodel.model"]
	start, end = strings.Index(root, "<build>"), strings.Index(root, "</build>")
	f["3D/3dmodel.model"] = root[:start] + "<build>" + strings.Repeat(`<item objectid="10"/>`, 65) + root[end:]
	if _, e = parseModel3MF(context.Background(), mfTestZIP(t, f)); e == nil {
		t.Fatal("65 objects in one board accepted")
	}
	b := importBoard{Objects: []int{0}}
	if e = arrangeImportedBoard(&b, []importMesh{{Mesh: ModelMesh{Bounds: ModelBounds{Size: Point3{236, 20, 20}}}}}); e == nil {
		t.Fatal("oversized source accepted")
	}
	if _, e = mfRead(context.Background(), &zip.File{FileHeader: zip.FileHeader{UncompressedSize64: 33 << 20}}, 32<<20); e == nil {
		t.Fatal("decompression limit bypass")
	}
}
func TestModel3MFOfficialEngineExport(t *testing.T) {
	file := os.Getenv("LAYERHUB_TEST_3MF")
	if file == "" {
		t.Skip("optional locally retained official export")
	}
	raw, e := os.ReadFile(file)
	if e != nil {
		t.Fatal(e)
	}
	p, e := parseModel3MF(context.Background(), raw)
	if e != nil {
		t.Fatal(e)
	}
	if p.summary().Objects != 1 || p.Sources[0].Mesh.Bounds.Size != (Point3{20, 20, 10}) {
		t.Fatal(p.summary())
	}
}
