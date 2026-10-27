package app

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"io"
)

// Only re-encoded, bounded PNG previews leave inspection. Geometry, SVGs,
// arbitrary archive paths and embedded machine instructions are never rendered.
func importPreviews(z *zip.ReadCloser, plates []SlicePlate) map[int]string {
	out := map[int]string{}
	for index, plate := range plates {
		if index >= 24 {
			break
		}
		for _, f := range z.File {
			if f.Name != fmt.Sprintf("Metadata/plate_%d.png", plate.ID) || f.UncompressedSize64 > 4<<20 {
				continue
			}
			reader, err := f.Open()
			if err != nil {
				continue
			}
			raw, err := io.ReadAll(io.LimitReader(reader, (4<<20)+1))
			reader.Close()
			if err != nil || len(raw) > 4<<20 {
				continue
			}
			cfg, err := png.DecodeConfig(bytes.NewReader(raw))
			if err != nil || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 2048 || cfg.Height > 2048 {
				continue
			}
			img, err := png.Decode(bytes.NewReader(raw))
			if err != nil {
				continue
			}
			w, h := cfg.Width, cfg.Height
			if w > 160 || h > 160 {
				if w >= h {
					h, w = max(1, h*160/w), 160
				} else {
					w, h = max(1, w*160/h), 160
				}
			}
			preview := image.NewNRGBA(image.Rect(0, 0, w, h))
			for y := 0; y < h; y++ {
				for x := 0; x < w; x++ {
					preview.Set(x, y, img.At(img.Bounds().Min.X+x*cfg.Width/w, img.Bounds().Min.Y+y*cfg.Height/h))
				}
			}
			var encoded bytes.Buffer
			if png.Encode(&encoded, preview) == nil && encoded.Len() <= 96<<10 {
				out[plate.ID] = "data:image/png;base64," + base64.StdEncoding.EncodeToString(encoded.Bytes())
			}
		}
	}
	return out
}
