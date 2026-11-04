package device

import (
	"context"
	"errors"
	"fmt"
	"layerhub.local/server/internal/app"
	"time"
)

func readFilament(l *listener, out *app.Equipment) {
	if l.config.ModelID != "x2d" {
		return
	}
	raw := object(object(l.raw["device"])["extruder"])
	state, ok := integer(raw["state"])
	if !ok || state&15 != 2 {
		return
	}
	for _, v := range list(raw["info"]) {
		x := object(v)
		id, ok := integer(x["id"])
		if !ok || id != 1 {
			continue
		}
		bits, b := integer(x["info"])
		stat, c := integer(x["stat"])
		main, d := integer(l.raw["ams_status"])
		if !b || !c || !d || bits&8 == 0 {
			return
		}
		f := app.FilamentReading{Extruder: 1, Busy: out.Switching, Step: stat & 255, MainState: (main >> 8) & 255, Updated: l.fields["extruder.feed.1"], HasFilament: bits&2 != 0, Source: packedSource(x["snow"]), Target: packedSource(x["star"])}
		for _, key := range []string{"extruder.state", "ams_status"} {
			if t := l.fields[key]; t.IsZero() || t.Before(f.Updated) {
				f.Updated = t
			}
		}
		if !f.HasFilament {
			f.Source = ""
		}
		for _, unit := range out.AMSUnits {
			if unit.ID == 0 && unit.Kind == "AMS 2 Pro" && unit.Extruder != nil && *unit.Extruder == 1 {
				f.Supported = true
				if unit.Updated.Before(f.Updated) {
					f.Updated = unit.Updated
				}
			}
		}
		out.Filament = f
		return
	}
}
func (m *Manager) ChangeFilament(ctx context.Context, p app.Printer, in app.FilamentInput) error {
	plan, e := app.PlanFilament(p, m.Status(p.ID), in)
	if e != nil {
		return e
	}
	if m.mode != "live" {
		return errors.New("live filament control required")
	}
	m.mu.Lock()
	l := m.items[p.ID]
	if l == nil || l.client == nil || !l.client.IsConnectionOpen() {
		m.mu.Unlock()
		return errors.New("disconnected")
	}
	client := l.client
	seq := fmt.Sprint(100000 + m.sequence.Add(1)%1000000000)
	if l.replies == nil {
		l.replies = map[string]map[string]any{}
	}
	l.replies[seq] = nil
	m.mu.Unlock()
	defer func() { m.mu.Lock(); delete(l.replies, seq); m.mu.Unlock() }()
	sent := time.Now()
	raw := app.FilamentPayload(plan, seq)
	// QoS0 and one Publish: reconnect must never replay a physical operation.
	token := client.Publish("device/"+p.Serial+"/request", 0, false, raw)
	if !token.WaitTimeout(3*time.Second) || token.Error() != nil {
		return errors.New("filament delivery unknown; never retry")
	}
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	var stable time.Time
	for {
		select {
		case <-ctx.Done():
			return errors.New("filament completion unknown; never retry")
		case <-ticker.C:
			m.mu.Lock()
			reply := l.replies[seq]
			result := text(reply["result"])
			negative := result == "fail" || result == "failed" || result == "error" || number(reply["errno"]) != 0 || number(reply["err_code"]) != 0
			m.mu.Unlock()
			if negative {
				return app.ErrDeviceRejected
			}
			t := m.Status(p.ID)
			if !t.Connected || t.PrintError != 0 || len(t.Equipment.HMS) > 0 {
				return errors.New("filament interrupted")
			}
			if app.FilamentComplete(t, plan, sent) {
				if stable.IsZero() {
					stable = time.Now()
				}
				if time.Since(stable) >= 3*time.Second {
					return nil
				}
			} else {
				stable = time.Time{}
			}
		}
	}
}
