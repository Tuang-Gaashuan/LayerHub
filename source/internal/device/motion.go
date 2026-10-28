package device

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"layerhub.local/server/internal/app"
	"math"
	"strconv"
	"strings"
	"time"
)

func readMotion(l *listener) app.MotionReading {
	out := app.MotionReading{}
	if f, ok := l.raw["fun"].(string); ok && len(f) <= 16 {
		if n, e := strconv.ParseUint(f, 16, 64); e == nil {
			out.NativeHome = n&(1<<32) != 0
			out.NativeJog = n&(1<<38) != 0
			out.CapabilitiesAt = l.fields["fun"]
		}
	}
	// Signed JSON int32 values are real device observations, not missing data.
	var value int64
	valid := false
	switch n := l.raw["home_flag"].(type) {
	case float64:
		if !math.IsNaN(n) && n >= math.MinInt32 && n <= math.MaxUint32 && n == math.Trunc(n) {
			value = int64(n)
			valid = true
		}
	case int:
		value = int64(n)
		valid = value >= math.MinInt32 && value <= math.MaxUint32
	}
	if valid {
		mask := int(uint32(value) & 7)
		out.HomeMask = &mask
		out.HomeAt = l.fields["home_flag"]
	}
	return out
}
func (m *Manager) Move(ctx context.Context, p app.Printer, in app.MotionInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if p.ModelID != "x2d" || in.Action != "home" && in.Action != "jog" {
		return errors.New("unsupported motion")
	}
	if in.Action == "jog" && (in.Step != 1 || in.Direction != 1 && in.Direction != -1 || in.Axis != "X" && in.Axis != "Y" && in.Axis != "Z") {
		return errors.New("invalid jog")
	}
	if err := app.ValidateMotion(m.Status(p.ID), in); err != nil {
		return err
	}
	if m.mode != "live" {
		return errors.New("live native motion required")
	}
	seq := fmt.Sprint(100000 + m.sequence.Add(1)%1000000000)
	m.mu.Lock()
	l := m.items[p.ID]
	if l == nil || l.client == nil || !l.client.IsConnectionOpen() {
		m.mu.Unlock()
		return errors.New("device disconnected")
	}
	client := l.client
	if l.replies == nil {
		l.replies = map[string]map[string]any{}
	}
	l.replies[seq] = nil
	m.mu.Unlock()
	defer func() { m.mu.Lock(); delete(l.replies, seq); m.mu.Unlock() }()
	payload := map[string]any{"command": "back_to_center", "sequence_id": seq}
	if in.Action == "jog" {
		payload["command"] = "xyz_ctrl"
		payload["axis"] = in.Axis
		payload["dir"] = in.Direction
		payload["mode"] = 0
	}
	data, _ := json.Marshal(map[string]any{"print": payload})
	sent := time.Now()
	if err := ctx.Err(); err != nil {
		return err
	}
	// Relative movement must never enter the QoS1 reconnect retransmit path.
	token := client.Publish("device/"+p.Serial+"/request", 0, false, data)
	if !token.WaitTimeout(3*time.Second) || token.Error() != nil {
		return errors.New("motion delivery unknown")
	}
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return errors.New("motion result unknown; never resend")
		case <-ticker.C:
			m.mu.Lock()
			ack := l.replies[seq]
			result := strings.ToLower(text(ack["result"]))
			errno := number(ack["errno"])
			reasonCode := number(ack["reason_code"])
			errCode := number(ack["err_code"])
			m.mu.Unlock()
			if result == "fail" || result == "failed" || result == "error" || errno != 0 || reasonCode != 0 || errCode != 0 {
				return app.ErrDeviceRejected
			}
			t := m.Status(p.ID)
			if !t.Connected || len(t.Equipment.HMS) > 0 || t.PrintError != 0 {
				return errors.New("motion interrupted")
			}
			if err := app.ValidateMotion(t, app.MotionInput{Action: "home"}); err != nil {
				return errors.New("motion observations expired or unsafe")
			}
			if result != "success" {
				continue
			}
			if !t.StateUpdated.After(sent) || t.RawState != "IDLE" && t.RawState != "FINISH" {
				continue
			}
			if in.Action == "jog" && time.Since(sent) > 2*time.Second {
				return nil
			} // acceptance only; app requires onsite review.
			if in.Action == "home" && motionHomeComplete(t.Equipment.Motion, sent, time.Now()) {
				return nil
			}
		}
	}
}
func motionHomeComplete(r app.MotionReading, sent, now time.Time) bool {
	return r.HomeMask != nil && *r.HomeMask == 7 && r.HomeAt.After(sent) && now.Sub(sent) > 60*time.Second && now.Sub(r.HomeAt) < 5*time.Second
}
