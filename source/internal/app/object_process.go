package app

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
)

// Only explicit, supported per-instance overrides are stored. Nil inherits.
type ObjectProcess struct {
	Walls           *int     `json:"walls,omitempty"`
	Infill          *int     `json:"infill,omitempty"`
	TopThickness    *float64 `json:"topThickness,omitempty"`
	BottomThickness *float64 `json:"bottomThickness,omitempty"`
	Pattern         *string  `json:"pattern,omitempty"`
}

func objectSettings(p *ObjectProcess, layer float64) map[string]string {
	result := map[string]string{}
	if p == nil {
		return result
	}
	if p.Walls != nil {
		result["wall_loops"] = fmt.Sprint(*p.Walls)
	}
	if p.Infill != nil {
		result["sparse_infill_density"] = fmt.Sprint(*p.Infill) + "%"
	}
	for key, n := range map[string]*float64{"top": p.TopThickness, "bottom": p.BottomThickness} {
		if n != nil {
			result[key+"_shell_thickness"] = strconv.FormatFloat(*n, 'f', -1, 64)
			result[key+"_shell_layers"] = fmt.Sprint(int(math.Ceil(*n/layer - 1e-8)))
		}
	}
	if p.Pattern != nil {
		pattern := *p.Pattern
		if pattern == "rectilinear" {
			pattern = "zig-zag"
		}
		result["sparse_infill_pattern"] = pattern
	}
	return result
}
func extendedObjects(o SliceSetup) bool {
	for _, p := range o.Copies {
		if p.Process != nil && (p.Process.TopThickness != nil || p.Process.BottomThickness != nil || p.Process.Pattern != nil) {
			return true
		}
	}
	return false
}

var instancePattern = regexp.MustCompile(`^[a-z0-9-]{1,48}$`)

func hasObjectOverrides(o SliceSetup) bool {
	for _, p := range o.Copies {
		if len(objectSettings(p.Process, o.Layer)) > 0 {
			return true
		}
	}
	return false
}
func hasInstanceMetadata(o SliceSetup) bool {
	for _, p := range o.Copies {
		if p.InstanceID != "" || p.Process != nil {
			return true
		}
	}
	return false
}
func validateObjectPlacements(o SliceSetup) error {
	seen := map[string]bool{}
	identified := false
	for _, p := range o.Copies {
		if p.InstanceID != "" {
			identified = true
			if !instancePattern.MatchString(p.InstanceID) || seen[p.InstanceID] {
				return invalid("对象实例标识无效或重复")
			}
			seen[p.InstanceID] = true
		}
		if p.Process != nil {
			for _, n := range []*float64{p.Process.TopThickness, p.Process.BottomThickness} {
				if n != nil && (!finite(*n) || *n < .12 || *n > 3) {
					return invalid("对象顶底厚度须在 0.12–3 mm 之间")
				}
			}
			if p.Process.Pattern != nil && *p.Process.Pattern != "grid" && *p.Process.Pattern != "gyroid" && *p.Process.Pattern != "rectilinear" {
				return invalid("对象填充图案不受支持")
			}
			if p.Process.Walls != nil && (*p.Process.Walls < 1 || *p.Process.Walls > 6) {
				return invalid("对象墙数须为 1–6")
			}
			if p.Process.Infill != nil && (*p.Process.Infill < 5 || *p.Process.Infill > 50) {
				return invalid("对象填充须为 5–50%")
			}
			if len(objectSettings(p.Process, o.Layer)) > 0 && p.InstanceID == "" {
				return invalid("单独工艺需要稳定对象标识，请刷新页面")
			}
		}
	}
	if identified {
		for _, p := range o.Copies {
			if p.InstanceID == "" {
				return invalid("部分对象缺少实例标识，请重新打开工程")
			}
		}
	}
	return nil
}
func instanceStateVersion(s SlicerState) int {
	if extendedObjects(s.Options) {
		return 4
	}
	for _, p := range s.Plates {
		if extendedObjects(p.Options) {
			return 4
		}
	}
	if hasInstanceMetadata(s.Options) {
		return 3
	}
	for _, p := range s.Plates {
		if hasInstanceMetadata(p.Options) {
			return 3
		}
	}
	return 2
}
func validateProjectInstances(s SlicerState) error {
	if len(s.Plates) == 0 {
		return validateObjectPlacements(s.Options)
	}
	seen := map[string]bool{}
	for _, p := range s.Plates {
		if e := validateObjectPlacements(p.Options); e != nil {
			return e
		}
		for _, o := range p.Options.Copies {
			if o.InstanceID != "" {
				if seen[o.InstanceID] {
					return invalid("同一工程的对象实例标识不能重复")
				}
				seen[o.InstanceID] = true
			}
		}
	}
	return nil
}
func preventInstanceDowngrade(raw string, next SlicerState) error {
	var saved savedSlicerOptions
	if e := json.Unmarshal([]byte(raw), &saved); e != nil {
		return e
	}
	if saved.Project != nil && saved.Project.Version == 4 && next.FormatVersion < 4 {
		return conflict("该工程含扩展对象工艺，请刷新页面后保存")
	}
	if saved.Project != nil && saved.Project.Version == 3 && instanceStateVersion(next) < 3 {
		return conflict("该工程已使用独立对象格式，请刷新页面后保存，避免丢失对象工艺")
	}
	return nil
}

type sliceObject struct {
	Placement ModelPlacement
	Mesh      ModelMesh
}

func loadSliceObjects(tx *sql.Tx, u User, primary int64, o SliceSetup) ([]sliceObject, error) {
	objects := []sliceObject{}
	sources := map[int64]ModelMesh{}
	for _, p := range platePlacements(primary, o) {
		mesh, ok := sources[p.ModelID]
		if !ok {
			var raw []byte
			if e := tx.QueryRow("SELECT stl FROM model_sources WHERE id=? AND user_id=?", p.ModelID, u.ID).Scan(&raw); e != nil {
				return nil, missing("对象源模型不存在或无权访问")
			}
			var e error
			mesh, e = parseModelSTL(raw)
			if e != nil {
				return nil, e
			}
			sources[p.ModelID] = mesh
		}
		setup := o
		setup.Copies = nil
		setup.X = p.X
		setup.Y = p.Y
		setup.Scale = p.Scale
		setup.Rotation = p.Rotation
		transformed, e := transformSingleModel(mesh, setup)
		if e != nil {
			return nil, e
		}
		objects = append(objects, sliceObject{p, transformed})
	}
	return objects, nil
}
