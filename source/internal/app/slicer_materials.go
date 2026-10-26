package app

import (
	"embed"
	"os"
	"path/filepath"
)

//go:embed profiles/*.json
var materialProfiles embed.FS

type SlicerMaterial struct {
	Key        string `json:"key"`
	Label      string `json:"label"`
	ProfileID  string `json:"profileId"`
	Material   string `json:"material"`
	PrintReady bool   `json:"printReady"`
}

var slicerMaterials = []SlicerMaterial{
	{"petg-basic", "Bambu PETG Basic", "GFG00", "PETG", true},
	{"pla-basic", "Bambu PLA Basic", "GFA00", "PLA", false},
	{"petg-hf", "Bambu PETG HF", "GFG02", "PETG", false},
}

func sliceMaterial(key string) (SlicerMaterial, error) {
	if key == "" {
		key = "petg-basic"
	}
	for _, m := range slicerMaterials {
		if m.Key == key {
			return m, nil
		}
	}
	return SlicerMaterial{}, invalid("请选择已支持的材料预设")
}
func (s *Service) filamentProfile(key string) ([]byte, error) {
	m, err := sliceMaterial(key)
	if err != nil {
		return nil, err
	}
	if m.Key == "petg-basic" {
		return os.ReadFile(filepath.Join(s.Config.SlicerProfiles, "filament.json"))
	}
	return materialProfiles.ReadFile("profiles/" + m.Key + ".json")
}
