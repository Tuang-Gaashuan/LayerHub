package app

import (
	"archive/zip"
	"encoding/json"
	"image/png"
	"io"
	"os"
	"testing"
)

// Optional real-engine integration check. The caller supplies a newly sliced
// artifact; no database, HTTP server, device driver or print operation is used.
func TestSlicingCLIArtifactCompatibility(t *testing.T) {
	file := os.Getenv("LAYERHUB_SLICING_POC_INPUT")
	if file == "" {
		t.Skip("set LAYERHUB_SLICING_POC_INPUT to a real CLI-generated X2D/PETG 3MF")
	}
	options := PrintOptions{Plate: 1, UseAMS: true, BedLeveling: true, Mapping: []AMSTarget{{Filament: 0, AMSID: 0, SlotID: 3, Grams: 3}}}
	info, plate, err := x2dPrintSlice(Job{Path: file}, options)
	if err != nil {
		t.Fatal(err)
	}
	if modelKey(info.Model) != modelKey("Bambu Lab X2D") || info.BedType != "Textured PEI Plate" || len(info.Plates) != 1 || plate.MultiNozzle || plate.Nozzle == nil || plate.Nozzle.Logical != 0 || plate.Nozzle.Physical != 1 || plate.Nozzle.Diameter != "0.4" {
		t.Fatalf("incompatible machine/nozzle: %+v %+v", info, plate)
	}
	if len(plate.Filaments) != 1 || plate.Filaments[0].Material != "PETG" || plate.Filaments[0].ProfileID != "GFG00" || plate.Filaments[0].Color != "#FFFFFF" || plate.Seconds <= 0 || plate.Filaments[0].Grams <= 0 || !plate.Thumbnail {
		t.Fatalf("invalid material/time/thumbnail: %+v", plate)
	}
	archive, err := zip.OpenReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	for _, entry := range archive.File {
		if entry.Name != "Metadata/plate_1.png" {
			continue
		}
		stream, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := png.DecodeConfig(io.LimitReader(stream, 4<<20))
		stream.Close()
		if err != nil || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 2048 || cfg.Height > 2048 {
			t.Fatal("preview outside LayerHub bounds", cfg, err)
		}
	}
	encoded, _ := json.Marshal(info)
	t.Log(string(encoded))
}
