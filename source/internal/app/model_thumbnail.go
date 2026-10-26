package app

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"strconv"
)

// modelThumbnail is a model illustration, not a toolpath or support preview.
// The Linux CLI can omit thumbnails when its bundled OpenGL loader fails.
func modelThumbnail(raw []byte, setup SliceSetup) ([]byte, error) {
	mesh, err := parseModelSTL(raw)
	if err != nil {
		return nil, err
	}
	mesh, err = transformModel(mesh, setup)
	if err != nil {
		return nil, err
	}
	return meshThumbnail(mesh, setup)
}
func meshThumbnail(mesh ModelMesh, setup SliceSetup) ([]byte, error) {
	const size = 320
	type projected struct{ x, y, z float64 }
	vertices := make([]projected, len(mesh.Vertices))
	minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for i, p := range mesh.Vertices {
		v := projected{(p[0] - p[1]) * .70710678, (p[0]+p[1])*.40824829 - p[2]*.81649658, (p[0] + p[1] + p[2]) * .57735027}
		vertices[i] = v
		minX = math.Min(minX, v.x)
		maxX = math.Max(maxX, v.x)
		minY = math.Min(minY, v.y)
		maxY = math.Max(maxY, v.y)
	}
	scale := 260 / math.Max(maxX-minX, maxY-minY)
	for i := range vertices {
		vertices[i].x = (vertices[i].x-(minX+maxX)/2)*scale + 160
		vertices[i].y = (vertices[i].y-(minY+maxY)/2)*scale + 160
	}
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	depth := make([]float64, size*size)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.SetRGBA(x, y, color.RGBA{242, 245, 251, 255})
			depth[y*size+x] = math.Inf(-1)
		}
	}
	if len(setup.Color) != 7 || setup.Color[0] != '#' {
		return nil, fmt.Errorf("invalid model color")
	}
	base, e := strconv.ParseUint(setup.Color[1:], 16, 32)
	if e != nil {
		return nil, e
	}
	channels := [3]float64{float64(base >> 16 & 255), float64(base >> 8 & 255), float64(base & 255)}
	edge := func(a, b projected, x, y float64) float64 { return (x-a.x)*(b.y-a.y) - (y-a.y)*(b.x-a.x) }
	budget := 32_000_000 // Bound raster work even for many huge overlapping triangles.
	for i := 0; i < len(vertices); i += 3 {
		a, b, c := vertices[i], vertices[i+1], vertices[i+2]
		area := edge(a, b, c.x, c.y)
		if math.Abs(area) < .00001 {
			continue
		}
		x0 := max(0, int(math.Floor(math.Min(a.x, math.Min(b.x, c.x)))))
		x1 := min(size-1, int(math.Ceil(math.Max(a.x, math.Max(b.x, c.x)))))
		y0 := max(0, int(math.Floor(math.Min(a.y, math.Min(b.y, c.y)))))
		y1 := min(size-1, int(math.Ceil(math.Max(a.y, math.Max(b.y, c.y)))))
		budget -= (x1 - x0 + 1) * (y1 - y0 + 1)
		if budget < 0 {
			return nil, fmt.Errorf("thumbnail geometry too complex")
		}
		p, q, r := mesh.Vertices[i], mesh.Vertices[i+1], mesh.Vertices[i+2]
		u, v := Point3{q[0] - p[0], q[1] - p[1], q[2] - p[2]}, Point3{r[0] - p[0], r[1] - p[1], r[2] - p[2]}
		n := Point3{u[1]*v[2] - u[2]*v[1], u[2]*v[0] - u[0]*v[2], u[0]*v[1] - u[1]*v[0]}
		length := math.Sqrt(n[0]*n[0] + n[1]*n[1] + n[2]*n[2])
		shade := .65
		if length > 0 {
			shade = .55 + .4*math.Abs((n[0]*.3-n[1]*.4+n[2]*.866)/length)
		}
		paint := color.RGBA{uint8(channels[0] * shade), uint8(channels[1] * shade), uint8(channels[2] * shade), 255}
		for y := y0; y <= y1; y++ {
			for x := x0; x <= x1; x++ {
				w0 := edge(b, c, float64(x)+.5, float64(y)+.5) / area
				w1 := edge(c, a, float64(x)+.5, float64(y)+.5) / area
				w2 := 1 - w0 - w1
				if w0 < 0 || w1 < 0 || w2 < 0 {
					continue
				}
				z := w0*a.z + w1*b.z + w2*c.z
				index := y*size + x
				if z > depth[index] {
					depth[index] = z
					img.SetRGBA(x, y, paint)
				}
			}
		}
	}
	var output bytes.Buffer
	err := png.Encode(&output, img)
	return output.Bytes(), err
}
