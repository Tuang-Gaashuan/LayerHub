package app

import (
	"archive/zip"
	"encoding/json"
	"io"
	"math"
	"strconv"
)

// Nil preserves the original pinned preset, including old bottom layer counts.
type SliceAdvanced struct {
	Walls           int     `json:"walls"`
	TopThickness    float64 `json:"topThickness"`
	BottomThickness float64 `json:"bottomThickness"`
	Pattern         string  `json:"pattern"`
	SupportType     string  `json:"supportType"`
	SupportAngle    int     `json:"supportAngle"`
	PlateOnly       bool    `json:"plateOnly"`
}

func validateSliceAdvanced(a *SliceAdvanced) error {
	if a == nil {
		return nil
	}
	if a.Walls < 1 || a.Walls > 6 {
		return invalid("墙壁层数须为 1–6")
	}
	if !finite(a.TopThickness) || !finite(a.BottomThickness) || a.TopThickness < .12 || a.TopThickness > 3 || a.BottomThickness < .12 || a.BottomThickness > 3 {
		return invalid("顶底厚度须在 0.12–3 mm 之间")
	}
	if a.Pattern != "grid" && a.Pattern != "gyroid" && a.Pattern != "rectilinear" {
		return invalid("请选择已支持的填充图案")
	}
	if a.SupportType != "normal" && a.SupportType != "tree" {
		return invalid("请选择普通或树状自动支撑")
	}
	if a.SupportAngle < 1 || a.SupportAngle > 80 {
		return invalid("支撑悬垂角须为 1–80°")
	}
	return nil
}
func advancedProcessSettings(o SliceSetup) map[string]string {
	a := o.Advanced
	if a == nil {
		return nil
	}
	f := func(n float64) string { return strconv.FormatFloat(n, 'f', -1, 64) }
	layers := func(n float64) string { return strconv.Itoa(int(math.Ceil(n/o.Layer - 1e-8))) }
	pattern := a.Pattern
	if pattern == "rectilinear" {
		pattern = "zig-zag"
	}
	plate := "0"
	if a.PlateOnly {
		plate = "1"
	}
	return map[string]string{"wall_loops": strconv.Itoa(a.Walls), "top_shell_layers": layers(a.TopThickness), "top_shell_thickness": f(a.TopThickness), "bottom_shell_layers": layers(a.BottomThickness), "bottom_shell_thickness": f(a.BottomThickness), "sparse_infill_pattern": pattern, "support_type": a.SupportType + "(auto)", "support_style": "default", "support_threshold_angle": strconv.Itoa(a.SupportAngle), "support_on_build_plate_only": plate}
}

// The exported project must confirm the requested settings; an ignored option
// is a failed slice rather than a successful result with misleading UI values.
func verifyAdvancedProcess(path string, o SliceSetup) error {
	if o.Advanced == nil {
		return nil
	}
	archive, e := zip.OpenReader(path)
	if e != nil {
		return e
	}
	defer archive.Close()
	for _, entry := range archive.File {
		if entry.Name != "Metadata/project_settings.config" {
			continue
		}
		if entry.UncompressedSize64 > 4<<20 {
			break
		}
		r, e := entry.Open()
		if e != nil {
			return e
		}
		raw, e := io.ReadAll(io.LimitReader(r, 4<<20))
		r.Close()
		if e != nil {
			return e
		}
		var got map[string]any
		if json.Unmarshal(raw, &got) != nil {
			break
		}
		expected := advancedProcessSettings(o)
		expected["layer_height"] = strconv.FormatFloat(o.Layer, 'f', -1, 64)
		expected["sparse_infill_density"] = strconv.Itoa(o.Infill) + "%"
		expected["enable_support"] = "0"
		if o.Support {
			expected["enable_support"] = "1"
		}
		for key, want := range expected {
			actual, ok := got[key].(string)
			if !ok {
				return conflict("切片结果缺少工艺参数：" + key)
			}
			a, ae := strconv.ParseFloat(actual, 64)
			b, be := strconv.ParseFloat(want, 64)
			if ae == nil && be == nil && finite(a) && math.Abs(a-b) < .00001 {
				continue
			}
			if actual != want {
				return conflict("切片引擎未采用所选工艺参数：" + key)
			}
		}
		return nil
	}
	return conflict("无法核对切片结果的高级工艺参数")
}
