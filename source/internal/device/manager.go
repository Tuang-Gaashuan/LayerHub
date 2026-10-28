package device

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/jlaffaye/ftp"
	"layerhub.local/server/internal/app"
)

type listener struct {
	lampRequest               *int
	config                    app.Printer
	cancel                    context.CancelFunc
	client                    mqtt.Client
	raw                       map[string]any
	connected                 bool
	updated                   time.Time
	err                       string
	started                   time.Time
	stateUpdated, taskUpdated time.Time
	pausedAt                  time.Time
	replies                   map[string]map[string]any
	fields                    map[string]time.Time
	ctx                       context.Context
	camera                    *cameraSession
}
type Manager struct {
	mode               string
	mu                 sync.Mutex
	items              map[int64]*listener
	wg                 sync.WaitGroup
	mqttPort, ftpPort  string
	sequence           atomic.Uint32
	simulationDuration time.Duration
}

func New(mode string) (*Manager, error) {
	if mode != "offline" && mode != "simulated" && mode != "live" {
		return nil, fmt.Errorf("device mode must be offline, simulated or live")
	}
	return &Manager{mode: mode, items: map[int64]*listener{}, mqttPort: "8883", ftpPort: "990", simulationDuration: 12 * time.Second}, nil
}
func sameConfig(a, b app.Printer) bool {
	return a.ID == b.ID && a.Name == b.Name && a.IP == b.IP && a.Serial == b.Serial && a.Code == b.Code && a.Fingerprint == b.Fingerprint && a.Enabled == b.Enabled && a.ModelID == b.ModelID && a.Configuration == b.Configuration
}
func (m *Manager) Reconcile(ctx context.Context, printers []app.Printer) {
	m.mu.Lock()
	defer m.mu.Unlock()
	keep := map[int64]bool{}
	for _, p := range printers {
		keep[p.ID] = true
		old := m.items[p.ID]
		if old != nil && sameConfig(old.config, p) {
			continue
		}
		if old != nil {
			old.cancel()
		}
		child, cancel := context.WithCancel(ctx)
		l := &listener{config: p, cancel: cancel, ctx: child, raw: map[string]any{}, err: "设备连接未启用"}
		m.items[p.ID] = l
		if !p.Enabled {
			l.err = "设备已停用"
			continue
		}
		switch m.mode {
		case "simulated":
			l.connected = true
			l.err = ""
			l.updated = time.Now()
			l.stateUpdated = l.updated
			l.raw = map[string]any{}
			applyReport(l, simulationReport(p))
		case "live":
			l.err = "正在连接设备"
			m.wg.Add(1)
			go m.run(child, p.ID, l)
		}
	}
	for id, l := range m.items {
		if !keep[id] {
			l.cancel()
			delete(m.items, id)
		}
	}
}
func (m *Manager) Close() {
	m.mu.Lock()
	for _, l := range m.items {
		l.cancel()
	}
	m.mu.Unlock()
	m.wg.Wait()
}
func tlsFor(p app.Printer) *tls.Config {
	cfg := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: p.IP}
	if p.Fingerprint != "" {
		expected, _ := hex.DecodeString(p.Fingerprint)
		cfg.InsecureSkipVerify = true
		cfg.VerifyConnection = func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return errors.New("device certificate missing")
			}
			actual := sha256.Sum256(cs.PeerCertificates[0].Raw)
			if subtle.ConstantTimeCompare(actual[:], expected) != 1 {
				return errors.New("device certificate fingerprint mismatch")
			}
			return nil
		}
	}
	return cfg
}
func (m *Manager) run(ctx context.Context, id int64, l *listener) {
	defer m.wg.Done()
	p := l.config
	options := mqtt.NewClientOptions().AddBroker("ssl://" + net.JoinHostPort(p.IP, m.mqttPort)).SetClientID(fmt.Sprintf("layerhub-%d-%d", id, time.Now().UnixNano())).SetUsername("bblp").SetPassword(p.Code).SetTLSConfig(tlsFor(p)).SetCleanSession(true).SetKeepAlive(20 * time.Second).SetPingTimeout(8 * time.Second).SetConnectTimeout(12 * time.Second).SetAutoReconnect(true).SetConnectRetry(true).SetConnectRetryInterval(8 * time.Second).SetMaxReconnectInterval(30 * time.Second)
	options.SetConnectionLostHandler(func(_ mqtt.Client, _ error) {
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.items[id] == l {
			l.connected = false
			l.lampRequest = nil
			l.stateUpdated = time.Time{}
			l.taskUpdated = time.Time{}
			l.raw = map[string]any{}
			l.fields = nil
			l.err = "设备连接断开，正在重试"
		}
	})
	options.SetOnConnectHandler(func(client mqtt.Client) {
		token := client.Subscribe("device/"+p.Serial+"/report", 0, func(_ mqtt.Client, message mqtt.Message) {
			if len(message.Payload()) > 1<<20 {
				return
			}
			var payload map[string]any
			if json.Unmarshal(message.Payload(), &payload) != nil {
				return
			}
			m.mu.Lock()
			defer m.mu.Unlock()
			if m.items[id] != l {
				return
			}
			if report, ok := payload["print"].(map[string]any); ok {
				applyReport(l, report)
			}
			if report, ok := payload["system"].(map[string]any); ok {
				if seq := text(report["sequence_id"]); seq != "" {
					if _, waiting := l.replies[seq]; waiting {
						l.replies[seq] = report
					}
				}
			}
		})
		if !token.WaitTimeout(10*time.Second) || token.Error() != nil {
			return
		}
		m.mu.Lock()
		if m.items[id] == l {
			l.connected = true
			l.err = ""
		}
		m.mu.Unlock()
		client.Publish("device/"+p.Serial+"/request", 0, false, `{"pushing":{"sequence_id":"0","command":"pushall","version":1,"push_target":1}}`)
	})
	client := mqtt.NewClient(options)
	m.mu.Lock()
	l.client = client
	m.mu.Unlock()
	client.Connect()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for running := true; running; {
		select {
		case <-ctx.Done():
			running = false
		case <-ticker.C:
			if client.IsConnectionOpen() {
				client.Publish("device/"+p.Serial+"/request", 0, false, `{"pushing":{"sequence_id":"0","command":"pushall","version":1,"push_target":1}}`)
			}
		}
	}
	client.Disconnect(250)
}

func applyReport(l *listener, report map[string]any) {
	now := time.Now()
	// Command echoes are not observations of hardware state.
	if text(report["command"]) != "" && text(report["command"]) != "push_status" {
		seq := text(report["sequence_id"])
		if _, waiting := l.replies[seq]; waiting {
			l.replies[seq] = report
		}
		return
	}
	observeFields(l, report, now)
	if text(report["gcode_state"]) != "" {
		l.stateUpdated = now
	}
	if _, ok := report["subtask_name"]; ok {
		l.taskUpdated = now
	}
	if _, ok := report["gcode_file"]; ok {
		l.taskUpdated = now
	}
	seq := text(report["sequence_id"])
	if _, waiting := l.replies[seq]; waiting {
		l.replies[seq] = report
	}
	merge(l.raw, report)
	l.updated = now
}

// Merge nested unit/tray deltas by their device IDs, while explicit empty arrays clear slots.
func merge(dst, src map[string]any) {
	for key, value := range src {
		if nested, ok := value.(map[string]any); ok {
			old, _ := dst[key].(map[string]any)
			if old == nil {
				old = map[string]any{}
			}
			merge(old, nested)
			dst[key] = old
			continue
		}
		if list, ok := value.([]any); ok && len(list) > 0 {
			old, _ := dst[key].([]any)
			merged := append([]any(nil), old...)
			byID := true
			for _, item := range list {
				record, ok := item.(map[string]any)
				if !ok || record["id"] == nil {
					byID = false
					break
				}
				found := false
				for i, existing := range merged {
					e, ok := existing.(map[string]any)
					if ok && fmt.Sprint(e["id"]) == fmt.Sprint(record["id"]) {
						merge(e, record)
						merged[i] = e
						found = true
						break
					}
				}
				if !found {
					merged = append(merged, record)
				}
			}
			if byID {
				dst[key] = merged
				continue
			}
		}
		dst[key] = value
	}
}
func number(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	}
	return 0
}
func text(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
func (m *Manager) Status(id int64) app.Telemetry {
	m.mu.Lock()
	defer m.mu.Unlock()
	l := m.items[id]
	if l == nil {
		return app.Telemetry{State: "offline", Error: "设备尚未登记"}
	}
	updated := l.updated
	if m.mode == "simulated" && l.config.Enabled {
		updated = time.Now()
		l.stateUpdated = updated
		observeFields(l, l.raw, updated)
		if !l.started.IsZero() && l.pausedAt.IsZero() {
			percent := min(100, int(time.Since(l.started)*100/m.simulationDuration))
			l.raw["mc_percent"] = percent
			if layers := number(l.raw["total_layer_num"]); layers > 0 {
				l.raw["layer_num"] = max(1, percent*layers/100)
			}
			l.raw["mc_remaining_time"] = max(1, int((m.simulationDuration-time.Since(l.started)+time.Minute-1)/time.Minute))
			if percent == 100 {
				l.raw["gcode_state"] = "FINISH"
				l.raw["mc_remaining_time"] = 0
				l.started = time.Time{}
			}
			l.stateUpdated = updated
		}
	}
	raw := l.raw
	state := map[string]string{"RUNNING": "printing", "IDLE": "idle", "FINISH": "idle", "PAUSE": "pause", "PREPARE": "printing", "SLICING": "printing", "FAILED": "failed"}[text(raw["gcode_state"])]
	if state == "" {
		state = "offline"
	}
	var ams any
	if raw["ams"] != nil {
		encoded, _ := json.Marshal(raw["ams"])
		_ = json.Unmarshal(encoded, &ams)
	}
	return app.Telemetry{Connected: l.connected, Updated: updated, State: state, File: text(raw["subtask_name"]), Percent: number(raw["mc_percent"]), Remaining: number(raw["mc_remaining_time"]), Layer: number(raw["layer_num"]), Layers: number(raw["total_layer_num"]), Nozzle: float64(number(raw["nozzle_temper"])), Bed: float64(number(raw["bed_temper"])), AMS: ams, Equipment: readEquipment(l), Error: l.err, RawState: text(raw["gcode_state"]), GcodeFile: text(raw["gcode_file"]), PrintError: number(raw["print_error"]), StateUpdated: l.stateUpdated, TaskUpdated: l.taskUpdated}
}

type reuseCache struct{ tls.ClientSessionCache }

func (c reuseCache) Get(_ string) (*tls.ClientSessionState, bool) {
	return c.ClientSessionCache.Get("printer")
}
func (c reuseCache) Put(_ string, s *tls.ClientSessionState) { c.ClientSessionCache.Put("printer", s) }

type counted struct {
	reader   io.Reader
	ctx      context.Context
	total    int64
	progress func(int64)
}

func (c *counted) Read(b []byte) (int, error) {
	select {
	case <-c.ctx.Done():
		return 0, c.ctx.Err()
	default:
	}
	n, err := c.reader.Read(b)
	c.total += int64(n)
	c.progress(c.total)
	return n, err
}
func (m *Manager) Upload(ctx context.Context, p app.Printer, path, remote string, progress func(int64)) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	reader := &counted{reader: file, ctx: ctx, progress: progress}
	if m.mode == "simulated" {
		_, err = io.Copy(io.Discard, reader)
		return err
	}
	if m.mode != "live" {
		return errors.New("device connection disabled")
	}
	cfg := tlsFor(p)
	cfg.MaxVersion = tls.VersionTLS12
	cfg.ClientSessionCache = reuseCache{tls.NewLRUClientSessionCache(1)}
	dial := func(network, address string) (net.Conn, error) {
		d := &net.Dialer{Timeout: 12 * time.Second}
		raw, e := d.DialContext(ctx, network, address)
		if e != nil {
			return nil, e
		}
		// Data TLS starts after STOR/150; eager handshakes deadlock some FTPS servers.
		conn := tls.Client(raw, cfg)
		if deadline, ok := ctx.Deadline(); ok {
			_ = conn.SetDeadline(deadline)
		}
		context.AfterFunc(ctx, func() { conn.Close() })
		return conn, nil
	}
	client, err := ftp.Dial(net.JoinHostPort(p.IP, m.ftpPort), ftp.DialWithTLS(cfg), ftp.DialWithDialFunc(dial), ftp.DialWithDisabledMLSD(true), ftp.DialWithContext(ctx))
	if err != nil {
		return err
	}
	defer client.Quit()
	if err = client.Login("bblp", p.Code); err != nil {
		return err
	}
	if err = client.Stor(remote, reader); err != nil {
		return err
	}
	size, err := client.FileSize(remote)
	if err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if size != info.Size() {
		return errors.New("remote file size mismatch")
	}
	return nil
}
func (m *Manager) Start(ctx context.Context, p app.Printer, remote string) error {
	return m.StartPrint(ctx, p, app.PrintRequest{Remote: remote, TaskName: remote})
}
