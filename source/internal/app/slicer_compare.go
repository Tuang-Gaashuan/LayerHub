package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"math"
	"sort"
	"strconv"
	"strings"
)

type SliceComparisonEntry struct {
	Task       SliceTask `json:"task"`
	Seconds    int       `json:"seconds"`
	Grams      float64   `json:"grams"`
	ExactUsage bool      `json:"exactUsage"`
	Objects    int       `json:"objects"`
	Size       Point3    `json:"size"`
}
type SliceComparison struct {
	Entries      []SliceComparisonEntry `json:"entries"`
	SameGeometry bool                   `json:"sameGeometry"`
}

func comparisonIDs(raw string) ([]int64, error) {
	parts := strings.Split(raw, ",")
	if len(parts) < 2 || len(parts) > 3 {
		return nil, invalid("请选择 2–3 份已完成的切片")
	}
	ids := []int64{}
	seen := map[int64]bool{}
	for _, part := range parts {
		id, e := strconv.ParseInt(part, 10, 64)
		if e != nil || id <= 0 || seen[id] {
			return nil, invalid("切片编号无效或重复")
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids, nil
}

// Compare the mesh actually submitted to Studio, at STL's float32 precision.
// Triangle ordering is irrelevant; bounds, filenames and object counts alone
// cannot establish identical geometry. Winding and vertex data are retained.
func comparisonGeometry(mesh ModelMesh) [32]byte {
	triangles := make([]string, 0, len(mesh.Vertices)/3)
	for i := 0; i < len(mesh.Vertices); i += 3 {
		var b [36]byte
		for v := 0; v < 3; v++ {
			for axis, x := range mesh.Vertices[i+v] {
				n := float32(x)
				if n == 0 {
					n = 0
				}
				binary.LittleEndian.PutUint32(b[v*12+axis*4:], math.Float32bits(n))
			}
		}
		triangles = append(triangles, string(b[:]))
	}
	sort.Strings(triangles)
	h := sha256.New()
	for _, t := range triangles {
		h.Write([]byte(t))
	}
	var digest [32]byte
	copy(digest[:], h.Sum(nil))
	return digest
}

func (s *Service) compareSlices(ctx context.Context, u User, ids []int64) (SliceComparison, error) {
	out := SliceComparison{Entries: []SliceComparisonEntry{}, SameGeometry: true}
	if len(ids) < 2 || len(ids) > 3 {
		return out, invalid("请选择 2–3 份已完成的切片")
	}
	seen := map[int64]bool{}
	for _, id := range ids {
		if id <= 0 || seen[id] {
			return out, invalid("切片编号无效或重复")
		}
		seen[id] = true
	}
	err := s.transaction(func(tx *sql.Tx) error {
		var first [32]byte
		for i, id := range ids {
			if err := ctx.Err(); err != nil {
				return err
			}
			var t SliceTask
			var options, info string
			err := tx.QueryRow("SELECT t.id,t.model_id,m.filename,t.state,t.options,t.info,t.error,t.created,t.updated,COALESCE(t.job_id,0) FROM slice_tasks t JOIN model_sources m ON m.id=t.model_id WHERE t.id=? AND t.user_id=? AND t.state='succeeded'", id, u.ID).Scan(&t.ID, &t.ModelID, &t.Filename, &t.State, &options, &info, &t.Error, &t.Created, &t.Updated, &t.JobID)
			if err == sql.ErrNoRows {
				return missing("切片不存在、尚未完成或无权访问")
			}
			if err != nil {
				return err
			}
			if json.Unmarshal([]byte(options), &t.Options) != nil || json.Unmarshal([]byte(info), &t.Info) != nil || t.Info == nil || len(t.Info.Plates) != 1 {
				return conflict("切片元数据不完整，请重新切片后对比")
			}
			mesh, err := loadPlate(tx, u, t.ModelID, t.Options)
			if err != nil {
				return err
			}
			hash := comparisonGeometry(mesh)
			if i == 0 {
				first = hash
			} else if hash != first {
				out.SameGeometry = false
			}
			plate := t.Info.Plates[0]
			entry := SliceComparisonEntry{Task: t, Seconds: plate.Seconds, Objects: len(platePlacements(t.ModelID, t.Options)), Size: mesh.Bounds.Size, ExactUsage: true}
			for _, f := range plate.Filaments {
				g := f.UsedGrams
				if g == 0 {
					g = float64(f.Grams)
					entry.ExactUsage = false
				}
				if !finite(g) || g < 0 {
					return conflict("切片用量元数据无效")
				}
				entry.Grams += g
			}
			if !finite(entry.Grams) || entry.Seconds < 0 || len(plate.Filaments) == 0 {
				return conflict("切片统计元数据无效")
			}
			out.Entries = append(out.Entries, entry)
		}
		return nil
	})
	return out, err
}
