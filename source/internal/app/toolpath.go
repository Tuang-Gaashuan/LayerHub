package app

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

// Segment coordinates are millimetres; kind 0 is travel, 1 wall, 2 infill,
// 3 solid/bridge, 4 support, 5 adhesion, 6 other extrusion.
type PathSegment [7]float32
type PathLayer struct {
	Number   int           `json:"number"`
	Z        float64       `json:"z"`
	Counts   [7]int        `json:"counts"`
	Segments []PathSegment `json:"-"`
}
type Toolpath struct {
	Layers []PathLayer
	Total  int
}

func pathKind(feature string) int {
	f := strings.ToLower(feature)
	switch {
	case strings.Contains(f, "support"):
		return 4
	case strings.Contains(f, "skirt") || strings.Contains(f, "brim"):
		return 5
	case strings.Contains(f, "wall") || strings.Contains(f, "perimeter"):
		return 1
	case strings.Contains(f, "solid") || strings.Contains(f, "surface") || strings.Contains(f, "bridge"):
		return 3
	case strings.Contains(f, "infill"):
		return 2
	default:
		return 6
	}
}
func pathWords(line string) (string, map[byte]float64, error) {
	line = strings.TrimSpace(strings.SplitN(line, ";", 2)[0])
	end := 0
	for end < len(line) && line[end] != ' ' && line[end] != '\t' {
		if end > 0 && (line[end] < '0' || line[end] > '9') && line[end] != '.' {
			break
		}
		end++
	}
	if end == 0 {
		return "", nil, nil
	}
	cmd := line[:end]
	values := map[byte]float64{}
	switch cmd {
	case "G0", "G00", "G1", "G01", "G2", "G02", "G3", "G03", "G92":
	default:
		return cmd, values, nil
	}
	for i := end; i < len(line); {
		if line[i] == ' ' || line[i] == '\t' {
			i++
			continue
		}
		if line[i] == '*' {
			break
		}
		letter := line[i]
		i++
		start := i
		for i < len(line) && ((line[i] >= '0' && line[i] <= '9') || line[i] == '.' || line[i] == '-' || line[i] == '+') {
			i++
		}
		if start == i {
			return cmd, nil, invalid("路径含无法识别的移动参数")
		}
		value, e := strconv.ParseFloat(line[start:i], 64)
		if e != nil || !finite(value) {
			return cmd, nil, invalid("路径坐标无效")
		}
		values[letter] = value
	}
	return cmd, values, nil
}
func parseToolpath(ctx context.Context, input io.Reader) (*Toolpath, error) {
	out := &Toolpath{}
	position := Point3{}
	offset := Point3{}
	known := [3]bool{}
	relative, relativeE := false, false
	unit, ePosition := 1.0, 0.0
	plane, kind := 17, 6
	layer := -1
	scanner := bufio.NewScanner(io.LimitReader(input, (64<<20)+1))
	scanner.Buffer(make([]byte, 4096), 512<<10)
	read, lines := 0, 0
	appendSegment := func(a, b Point3, k int) error {
		if layer < 0 {
			return nil
		}
		if math.Abs(a[0]-b[0])+math.Abs(a[1]-b[1])+math.Abs(a[2]-b[2]) < .00001 {
			return nil
		}
		for _, p := range []Point3{a, b} {
			for _, v := range p {
				if !finite(v) || math.Abs(v) > 10000 {
					return invalid("路径坐标超出预览范围")
				}
			}
		}
		if out.Total >= 1_000_000 || len(out.Layers[layer].Segments) >= 100_000 {
			return conflict("路径过于复杂，请下载切片在 Bambu Studio 检查")
		}
		out.Layers[layer].Segments = append(out.Layers[layer].Segments, PathSegment{float32(a[0]), float32(a[1]), float32(a[2]), float32(b[0]), float32(b[1]), float32(b[2]), float32(k)})
		out.Layers[layer].Counts[k]++
		out.Total++
		return nil
	}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		read += len(scanner.Bytes()) + 1
		lines++
		if read > 64<<20 {
			return nil, invalid("路径文件超过 64 MiB")
		}
		if lines%1024 == 0 {
			if e := ctx.Err(); e != nil {
				return nil, e
			}
		}
		if strings.HasPrefix(line, "; MACHINE_END_GCODE_START") || strings.HasPrefix(line, "; EXECUTABLE_BLOCK_END") {
			break
		}
		if line == "; CHANGE_LAYER" || line == ";LAYER_CHANGE" {
			if len(out.Layers) >= 4096 {
				return nil, invalid("预览层数过多")
			}
			layer++
			out.Layers = append(out.Layers, PathLayer{Number: layer + 1, Segments: []PathSegment{}})
			continue
		}
		if strings.HasPrefix(line, "; Z_HEIGHT:") {
			v, e := strconv.ParseFloat(strings.TrimSpace(strings.TrimPrefix(line, "; Z_HEIGHT:")), 64)
			if e != nil || !finite(v) || v < 0 || v > 10000 {
				return nil, invalid("层高无效")
			}
			if layer >= 0 {
				out.Layers[layer].Z = v
			}
			continue
		}
		if strings.HasPrefix(line, "; FEATURE:") {
			kind = pathKind(strings.TrimSpace(strings.TrimPrefix(line, "; FEATURE:")))
			continue
		}
		if line == "" || line[0] == ';' {
			continue
		}
		// Vendor M/T commands include text and flags. They are not geometry.
		if line[0] != 'G' && !strings.HasPrefix(line, "M82") && !strings.HasPrefix(line, "M83") {
			continue
		}
		cmd, v, err := pathWords(line)
		if err != nil {
			return nil, invalid(fmt.Sprintf("路径第 %d 行（%s）：%s", lines, cmd, err.Error()))
		}
		switch cmd {
		case "G90":
			relative = false
			continue
		case "G91":
			relative = true
			continue
		case "M82":
			relativeE = false
			continue
		case "M83":
			relativeE = true
			continue
		case "G20":
			unit = 25.4
			continue
		case "G21":
			unit = 1
			continue
		case "G17":
			plane = 17
			continue
		case "G18":
			plane = 18
			continue
		case "G19":
			plane = 19
			continue
		case "G92":
			for i, k := range []byte{'X', 'Y', 'Z'} {
				if n, ok := v[k]; ok {
					offset[i] = position[i] - n*unit
				}
			}
			if n, ok := v['E']; ok {
				ePosition = n * unit
			}
			continue
		case "G28":
			known = [3]bool{}
			continue
		case "G0", "G00", "G1", "G01", "G2", "G02", "G3", "G03":
		default:
			if layer >= 0 && (cmd == "G5" || cmd == "G6") {
				return nil, invalid("此移动指令尚不支持预览，请在 Bambu Studio 检查")
			}
			continue
		}
		start := position
		wasKnown := known[0] && known[1] && known[2]
		for i, k := range []byte{'X', 'Y', 'Z'} {
			if n, ok := v[k]; ok {
				if relative {
					position[i] += n * unit
				} else {
					position[i] = n*unit + offset[i]
					known[i] = true
				}
			}
		}
		extrusion := 0.0
		if n, ok := v['E']; ok {
			n *= unit
			if relativeE {
				extrusion = n
				ePosition += n
			} else {
				extrusion = n - ePosition
				ePosition = n
			}
		}
		k := kind
		if extrusion <= .000001 {
			k = 0
		}
		if layer < 0 || !wasKnown || !known[0] || !known[1] || !known[2] {
			continue
		}
		arc := cmd == "G2" || cmd == "G02" || cmd == "G3" || cmd == "G03"
		if !arc {
			if err = appendSegment(start, position, k); err != nil {
				return nil, err
			}
			continue
		}
		if plane != 17 {
			return nil, invalid("暂不支持非 XY 平面圆弧预览")
		}
		if _, ok := v['R']; ok {
			return nil, invalid("暂不支持 R 圆弧预览，请在 Bambu Studio 检查")
		}
		i, j := v['I']*unit, v['J']*unit
		radius := math.Hypot(i, j)
		if radius < .00001 {
			return nil, invalid("圆弧中心无效")
		}
		center := Point3{start[0] + i, start[1] + j, 0}
		endRadius := math.Hypot(position[0]-center[0], position[1]-center[1])
		if math.Abs(radius-endRadius) > .1 {
			return nil, invalid("圆弧半径不一致")
		}
		angle := math.Atan2(start[1]-center[1], start[0]-center[0])
		finish := math.Atan2(position[1]-center[1], position[0]-center[0])
		sweep := finish - angle
		clockwise := cmd == "G2" || cmd == "G02"
		if clockwise {
			if sweep >= -1e-9 {
				sweep -= 2 * math.Pi
			}
		} else {
			if sweep <= 1e-9 {
				sweep += 2 * math.Pi
			}
		}
		if turns, ok := v['P']; ok {
			if turns < 1 || turns > 10 || turns != math.Trunc(turns) {
				return nil, invalid("圆弧圈数无效")
			}
			direction := 1.0
			if clockwise {
				direction = -1
			}
			sweep += direction * (turns - 1) * 2 * math.Pi
		}
		steps := int(math.Ceil(math.Max(math.Abs(sweep)/(.08726646), math.Abs(sweep)*radius/.5)))
		if steps > 4096 {
			return nil, invalid("圆弧超过预览精度范围")
		}
		previous := start
		for step := 1; step <= steps; step++ {
			ratio := float64(step) / float64(steps)
			a := angle + sweep*ratio
			next := Point3{center[0] + radius*math.Cos(a), center[1] + radius*math.Sin(a), start[2] + (position[2]-start[2])*ratio}
			if step == steps {
				next = position
			}
			if err = appendSegment(previous, next, k); err != nil {
				return nil, err
			}
			previous = next
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, invalid("路径文件读取失败")
	}
	if len(out.Layers) == 0 || out.Total == 0 {
		return nil, invalid("切片没有可识别的逐层路径")
	}
	return out, nil
}

func (s *Service) toolpathRequest(ctx context.Context, u User, idText, layerText string) (any, error) {
	id, e := strconv.ParseInt(idText, 10, 64)
	if e != nil || id <= 0 {
		return nil, invalid("切片编号无效")
	}
	// Always authorize against the database, including on a cache hit after deletion/restore.
	var raw []byte
	if e = s.DB.QueryRowContext(ctx, "SELECT output FROM slice_tasks WHERE id=? AND user_id=? AND state='succeeded'", id, u.ID).Scan(&raw); e != nil {
		return nil, missing("切片不存在、尚未完成或无权访问")
	}
	key := sha256.Sum256(raw)
	s.pathMu.Lock()
	defer s.pathMu.Unlock()
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	path := s.pathCache[key]
	if path == nil {
		archive, e := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
		if e != nil {
			return nil, invalid("切片归档无效")
		}
		for _, f := range archive.File {
			if f.Name != "Metadata/plate_1.gcode" {
				continue
			}
			if f.UncompressedSize64 > 64<<20 {
				return nil, invalid("路径文件超过 64 MiB")
			}
			reader, e := f.Open()
			if e != nil {
				return nil, e
			}
			path, e = parseToolpath(ctx, reader)
			reader.Close()
			if e != nil {
				return nil, e
			}
			break
		}
		if path == nil {
			return nil, missing("切片没有第 1 盘路径")
		}
		if s.pathCache == nil || len(s.pathCache) >= 2 {
			s.pathCache = make(map[[32]byte]*Toolpath)
		}
		s.pathCache[key] = path
	}
	if layerText == "" {
		return map[string]any{"ok": true, "layers": path.Layers, "segments": path.Total, "note": "显示切片指令路径；不模拟固件宏、机械运动或挤出宽度。启动和机尾动作不显示。"}, nil
	}
	number, e := strconv.Atoi(layerText)
	if e != nil || number < 1 || number > len(path.Layers) {
		return nil, invalid(fmt.Sprintf("请选择 1–%d 层", len(path.Layers)))
	}
	layer := path.Layers[number-1]
	return map[string]any{"ok": true, "layer": layer.Number, "z": layer.Z, "segments": layer.Segments, "counts": layer.Counts}, nil
}
