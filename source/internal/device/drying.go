package device

import "strconv"

// BambuStudio DevFilaSystem.cpp: AMS info bits 0..3 type, 4..7 drying state.
func amsDrying(raw map[string]any) bool {
	for _, v := range list(object(raw["ams"])["ams"]) {
		u := object(v)
		info, e := strconv.ParseUint(text(u["info"]), 16, 32)
		if e != nil {
			continue
		}
		kind, status := info&15, (info>>4)&15
		if (kind == 3 || kind == 4) && status >= 1 && status <= 7 {
			return true
		}
	}
	return false
}
