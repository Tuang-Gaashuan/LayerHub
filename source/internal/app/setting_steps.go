package app

// Fan controls use whole ten-percent levels. Finer UI targets such as 25%
// may be reported as 20% by the device and cannot be confirmed reliably.
func EquipmentSettingStep(key string) int {
	switch key {
	case "fan:1", "fan:2", "fan:3", "fan:10", "part_fan", "aux_fan", "chamber_fan":
		return 10
	}
	return 1
}
