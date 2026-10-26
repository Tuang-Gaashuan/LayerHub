package app

import "fmt"

type ModelPlacement struct {
	InstanceID string         `json:"instanceId,omitempty"`
	Process    *ObjectProcess `json:"process,omitempty"`
	ModelID    int64          `json:"modelId,omitempty"`
	X          float64        `json:"x"`
	Y          float64        `json:"y"`
	Scale      float64        `json:"scale"`
	Rotation   Point3         `json:"rotation"`
}

// Copies share slicing settings, but each has its own placement. The disjoint
// transformed meshes are sliced together, by layer, in one plate and one color.
func transformModel(source ModelMesh, setup SliceSetup) (ModelMesh, error) {
	if len(setup.Copies) == 0 {
		return transformSingleModel(source, setup)
	}
	return transformModels(map[int64]ModelMesh{0: source}, 0, setup)
}

// Each instance resolves its own private source, while sharing one material and
// process preset. Legacy instances without modelId use the primary source.
func transformModels(sources map[int64]ModelMesh, primary int64, setup SliceSetup) (ModelMesh, error) {
	placements := platePlacements(primary, setup)
	if len(placements) > 64 {
		return ModelMesh{}, invalid("整盘最多 64 个对象，合计不得超过 150,000 个三角面")
	}
	margin := 0.0
	if setup.Brim {
		margin += 5
	}
	if setup.Support {
		margin += 5
	}
	boxes := make([]ModelBounds, 0, len(placements))
	result := ModelMesh{}
	for index, p := range placements {
		source, exists := sources[p.ModelID]
		if !exists {
			return ModelMesh{}, missing("盘内源模型缺失，请重新读取工程")
		}
		if len(result.Vertices)/3+len(source.Vertices)/3 > maxModelTriangles {
			return ModelMesh{}, invalid("整盘合计不得超过 150,000 个三角面")
		}
		o := setup
		o.Copies = nil
		o.X = p.X
		o.Y = p.Y
		o.Scale = p.Scale
		o.Rotation = p.Rotation
		mesh, e := transformSingleModel(source, o)
		if e != nil {
			return source, e
		}
		b := mesh.Bounds
		if len(setup.Copies) > 0 && (b.Min[0] < 20.5+margin-.001 || b.Min[1] < margin-.001 || b.Max[0] > 256-margin+.001 || b.Max[1] > 256-margin+.001) {
			return source, conflict(fmt.Sprintf("副本 %d 靠近边缘，请为裙边或支撑预留空间", index+1))
		}
		gap := 5 + 2*margin
		for j, other := range boxes {
			if b.Min[0] < other.Max[0]+gap-.001 && b.Max[0] > other.Min[0]-gap+.001 && b.Min[1] < other.Max[1]+gap-.001 && b.Max[1] > other.Min[1]-gap+.001 {
				return source, conflict(fmt.Sprintf("副本 %d 与 %d 距离不足，请自动排版或增大间距", index+1, j+1))
			}
		}
		boxes = append(boxes, b)
		result.Vertices = append(result.Vertices, mesh.Vertices...)
	}
	var e error
	result.Bounds, e = meshBounds(result.Vertices)
	return result, e
}
