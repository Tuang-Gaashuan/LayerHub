package app

import (
	"context"
	"database/sql"
	"sync"
	"sync/atomic"
	"time"
)

type Config struct {
	Address, DataDir, WebDir, AdminUser, AdminPassword, DeviceMode string
	EnablePrintStart, CookieSecure                                 bool
	MaxUpload                                                      int64
	MailToken, PublicURL                                           string
	DisableBackgroundEvents                                        bool
}
type User struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
	Admin    bool   `json:"admin"`
	Owner    bool   `json:"owner"`
	Language string `json:"language,omitempty"`
}
type Member struct {
	User
	Role     string `json:"role"`
	Color    string `json:"color"`
	Disabled bool   `json:"disabled"`
}
type Mount struct {
	PrinterID int64  `json:"printerId"`
	Slot      string `json:"slot"`
}
type Spool struct {
	ID            string `json:"id"`
	Label         string `json:"label"`
	Brand         string `json:"brand"`
	Material      string `json:"material"`
	ColorName     string `json:"colorName"`
	Color         string `json:"color"`
	Capacity      int    `json:"capacity"`
	Remaining     int    `json:"remaining"`
	Owner         string `json:"owner"`
	Shared        bool   `json:"shared"`
	State         string `json:"state"`
	Location      string `json:"location"`
	Mount         *Mount `json:"mount"`
	OwnerID       int64  `json:"-"`
	LowThreshold  int    `json:"lowThreshold"`
	AlertRevision int64  `json:"alertRevision"`
	Revision      int64  `json:"revision"`
}
type Printer struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	IP            string    `json:"ip"`
	Serial        string    `json:"serial"`
	ModelID       string    `json:"modelId"`
	Configuration string    `json:"configuration"`
	Model         string    `json:"model"`
	Enabled       bool      `json:"enabled"`
	State         string    `json:"state"`
	Connected     bool      `json:"connected"`
	Updated       int64     `json:"updated"`
	Percent       int       `json:"percent"`
	Remaining     string    `json:"remaining"`
	Layer         int       `json:"layer"`
	Layers        int       `json:"layers"`
	Nozzle        float64   `json:"nozzle"`
	Bed           float64   `json:"bed"`
	File          string    `json:"file"`
	User          string    `json:"user"`
	Material      string    `json:"material"`
	Warning       string    `json:"warning"`
	Color         string    `json:"color"`
	AMS           any       `json:"ams,omitempty"`
	Equipment     Equipment `json:"equipment"`
	RawState      string    `json:"rawState"`
	PrintError    int       `json:"printError"`
	Fingerprint   string    `json:"fingerprint,omitempty"`
	Code          string    `json:"-"`
}
type Job struct {
	ID                   int64         `json:"id"`
	UserID               int64         `json:"-"`
	User                 string        `json:"user"`
	Filename             string        `json:"filename"`
	Size                 float64       `json:"size"`
	Printer              int64         `json:"printer"`
	State                string        `json:"state"`
	DisplayState         string        `json:"displayState,omitempty"`
	FinishedAt           int64         `json:"finishedAt,omitempty"`
	CollectedAt          int64         `json:"collectedAt,omitempty"`
	SettledAt            int64         `json:"settledAt,omitempty"`
	Note                 string        `json:"note"`
	Created              string        `json:"created"`
	Order                int64         `json:"order"`
	Estimate             int           `json:"estimate"`
	Grams                int           `json:"grams"`
	SpoolID              string        `json:"spoolId"`
	ActualGrams          *int          `json:"actualGrams,omitempty"`
	Color                string        `json:"color"`
	Error                string        `json:"error,omitempty"`
	Path, SHA256, Remote string        `json:"-"`
	Bytes                int64         `json:"-"`
	CreatedAt            int64         `json:"createdAt"`
	PrintOptions         *PrintOptions `json:"printOptions,omitempty"`
}
type Telemetry struct {
	Connected                         bool
	Updated                           time.Time
	State                             string
	File                              string
	Percent, Remaining, Layer, Layers int
	Nozzle, Bed                       float64
	AMS                               any
	Equipment                         Equipment
	Error                             string
	RawState, GcodeFile               string
	PrintError                        int
	StateUpdated, TaskUpdated         time.Time
}
type AMSTarget struct {
	Filament int    `json:"filament"`
	AMSID    int    `json:"amsId"`
	SlotID   int    `json:"slotId"`
	SpoolID  string `json:"spoolId"`
	Grams    int    `json:"grams"`
}
type PrintOptions struct {
	Plate           int         `json:"plate"`
	UseAMS          bool        `json:"useAMS"`
	Mapping         []AMSTarget `json:"mapping"`
	BedLeveling     bool        `json:"bedLeveling"`
	FlowCalibration bool        `json:"flowCalibration"`
	Timelapse       bool        `json:"timelapse"`
}
type PrintRequest struct {
	Remote, TaskName string
	Options          *PrintOptions
	BedType          string
}
type PrinterController interface {
	StartPrint(context.Context, Printer, PrintRequest) error
	Control(context.Context, Printer, string) error
}
type Device interface {
	Reconcile(context.Context, []Printer)
	Status(int64) Telemetry
	Upload(context.Context, Printer, string, string, func(int64)) error
	Start(context.Context, Printer, string) error
	Close()
}
type Service struct {
	fileInspectBusy atomic.Bool
	DB              *sql.DB
	Config          Config
	Device          Device
	key             []byte
	writes          sync.Mutex
	ctx             context.Context
	cancel          context.CancelFunc
	workers         sync.WaitGroup
	transfers       chan struct{}
	pendingRestore  atomic.Bool
	restoreMu       sync.Mutex
	restoreTicket   string
	restoreOwner    int64
	releaseLock     func()
	closeOnce       sync.Once
}
type apiError struct {
	Code    int
	Message string
}

func (e *apiError) Error() string    { return e.Message }
func invalid(message string) error   { return &apiError{400, message} }
func conflict(message string) error  { return &apiError{409, message} }
func forbidden(message string) error { return &apiError{403, message} }
func missing(message string) error   { return &apiError{404, message} }

var activeStates = map[string]bool{"queued": true, "uploading": true, "assigned": true, "starting": true, "printing": true, "paused": true, "stopped": true, "checking": true}
var models = map[string]string{"a1-mini": "A1 mini", "a1": "A1", "a2l": "A2L", "p1p": "P1P", "p1s": "P1S", "p2s": "P2S", "x1": "X1", "x1-carbon": "X1 Carbon", "x1e": "X1E", "x2d": "X2D", "h2s": "H2S", "h2d": "H2D", "h2d-pro": "H2D Pro", "h2c": "H2C"}

func supply(id string) string {
	switch id {
	case "a1-mini", "a1", "a2l":
		return "AMS lite"
	case "p1p", "p1s", "x1", "x1-carbon", "x1e":
		return "AMS"
	case "h2d-pro":
		return "AMS 2 Pro + AMS HT"
	default:
		return "AMS 2 Pro"
	}
}
func validSlot(p Printer, slot string) bool {
	if slot == "external" {
		return true
	}
	if ValidExtendedSlot(p, slot) {
		return true
	}
	if p.Configuration != "ams" {
		return false
	}
	return slot == "1" || slot == "2" || slot == "3" || slot == "4" || p.ModelID == "h2d-pro" && slot == "5"
}
