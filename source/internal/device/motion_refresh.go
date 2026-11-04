package device

import (
	"context"
	"errors"
	"layerhub.local/server/internal/app"
	"time"
)

// Read-only pushall, never homing or a fallback relative movement.
func (m *Manager) RefreshMotion(ctx context.Context, p app.Printer) error {
	m.mu.Lock()
	l := m.items[p.ID]
	if l == nil || l.client == nil || !l.client.IsConnectionOpen() {
		m.mu.Unlock()
		return errors.New("disconnected")
	}
	client := l.client
	m.mu.Unlock()
	sent := time.Now()
	token := client.Publish("device/"+p.Serial+"/request", 0, false, `{"pushing":{"command":"pushall","sequence_id":"0","version":1,"push_target":1}}`)
	if !token.WaitTimeout(3*time.Second) || token.Error() != nil {
		return errors.New("snapshot unavailable")
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			t := m.Status(p.ID)
			if !t.Connected {
				return errors.New("disconnected")
			}
			if t.StateUpdated.After(sent) && t.Equipment.Motion.HomeAt.After(sent) {
				return nil
			}
		}
	}
}
