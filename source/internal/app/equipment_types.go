package app

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Device readings are separate from the manually maintained inventory ledger.
type ExtruderReading struct {
	ID               int      `json:"id"`
	Name             string   `json:"name"`
	Temperature      *int     `json:"temperature"`
	TemperatureExact *float64 `json:"temperatureExact,omitempty"`
	Target           *int     `json:"target"`
	CurrentSource    string   `json:"currentSource"`
	TargetSource     string   `json:"targetSource"`
}
type FeedReading struct {
	Kind          string    `json:"kind"`
	Slot          string    `json:"slot"`
	Label         string    `json:"label"`
	Material      string    `json:"material"`
	ProfileID     string    `json:"profileId,omitempty"`
	Color         string    `json:"color"`
	Remaining     *int      `json:"remaining"`
	RemainingUnit string    `json:"remainingUnit"`
	Present       *bool     `json:"present"`
	Updated       time.Time `json:"updated"`
}

type AMSUnitReading struct {
	ID            int       `json:"id"`
	Kind          string    `json:"kind"`
	Temperature   *float64  `json:"temperature"`
	HumidityIndex *int      `json:"humidityIndex"`
	Extruder      *int      `json:"extruder"`
	Updated       time.Time `json:"updated"`
}

type NozzleReading struct {
	ID       int       `json:"id"`
	Diameter string    `json:"diameter"`
	Type     string    `json:"type"`
	Updated  time.Time `json:"updated"`
}

type CameraReading struct {
	Supported  bool   `json:"supported"`
	Protocol   string `json:"protocol"`
	Resolution string `json:"resolution"`
}

type CameraFrame struct {
	JPEG    []byte
	Updated time.Time
}

type CameraProvider interface {
	CameraFrame(context.Context, Printer) (CameraFrame, error)
}
type SettingReading struct {
	Feedback string    `json:"feedback,omitempty"`
	Key      string    `json:"key"`
	Label    string    `json:"label"`
	Unit     string    `json:"unit"`
	Value    int       `json:"value"`
	Min      int       `json:"min"`
	Max      int       `json:"max"`
	Updated  time.Time `json:"updated"`
}
type Equipment struct {
	BedExact     *float64          `json:"bedExact,omitempty"`
	Motion       MotionReading     `json:"motion"`
	ControlIssue string            `json:"controlIssue,omitempty"`
	Bed          *int              `json:"bed"`
	BedTarget    *int              `json:"bedTarget"`
	Extruders    []ExtruderReading `json:"extruders"`
	Feeds        []FeedReading     `json:"feeds"`
	Settings     []SettingReading  `json:"settings"`
	HMS          []string          `json:"hms"`
	Switching    bool              `json:"switching"`
	AMSUnits     []AMSUnitReading  `json:"amsUnits"`
	Nozzles      []NozzleReading   `json:"nozzles"`
	Camera       CameraReading     `json:"camera"`
}

// An explicit negative device response is different from a missing response.
// A positive command receipt confirms acceptance, not measured actuator state.
var ErrDeviceAccepted = errors.New("printer accepted command; physical state not reported")

var ErrDeviceRejected = errors.New("printer rejected command")

type SettingInput struct {
	PrinterID int64  `json:"printerId"`
	Key       string `json:"key"`
	Value     int    `json:"value"`
}
type EquipmentController interface {
	SetEquipment(context.Context, Printer, SettingInput) error
}

func dualModel(id string) bool { return id == "h2d" || id == "h2d-pro" || id == "h2c" || id == "x2d" }

// Existing four-slot mount IDs keep their meaning. Extended IDs identify the
// physical source, never the slicing filament index or the active extruder.
func sourceSlot(unit, slot int) string {
	if unit == 0 && slot >= 0 && slot < 4 {
		return strconv.Itoa(slot + 1)
	}
	return fmt.Sprintf("ams:%d:%d", unit, slot)
}
func normalizedSlot(slot string) string {
	if slot == "5" {
		return "ams:128:0"
	}
	return slot
}
func ValidExtendedSlot(p Printer, slot string) bool {
	if slot == "external:1" {
		return dualModel(p.ModelID)
	}
	if p.Configuration != "ams" {
		return false
	}
	parts := strings.Split(slot, ":")
	if len(parts) != 3 || parts[0] != "ams" {
		return false
	}
	u, e := strconv.Atoi(parts[1])
	n, e2 := strconv.Atoi(parts[2])
	return e == nil && e2 == nil && (u >= 0 && u <= 31 || u >= 128 && u <= 135) && n >= 0 && n < 4 && (u < 128 || n == 0) && slot == sourceSlot(u, n)
}
func SourceSlot(unit, slot int) string { return sourceSlot(unit, slot) }
