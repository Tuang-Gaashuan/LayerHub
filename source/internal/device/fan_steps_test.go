package device

import (
	"context"
	"layerhub.local/server/internal/app"
	"testing"
	"time"
)

func TestFanLevelFeedbackRequiresFreshReadingAndExactOff(t *testing.T) {
	sent := time.Now()
	for _, key := range []string{"fan:1", "fan:2", "fan:3", "fan:10", "part_fan", "aux_fan", "chamber_fan"} {
		for _, c := range []struct {
			target, actual int
			fresh, want    bool
		}{
			{25, 20, true, false}, // Real X2D #96: never offer or silently accept this target.
			{75, 70, true, false},
			{20, 20, true, true},
			{20, 20, false, false},
			{0, 0, true, true},
			{0, 1, true, false},
			{0, 10, true, false},
			{10, 0, true, false},
			{100, 100, true, true},
		} {
			stamp := sent.Add(time.Second)
			if !c.fresh {
				stamp = sent.Add(-time.Second)
			}
			tel := app.Telemetry{Equipment: app.Equipment{Settings: []app.SettingReading{{Key: key, Value: c.actual, Min: 0, Max: 100, Updated: stamp}}}}
			if got := settingMatches(tel, app.SettingInput{Key: key, Value: c.target}, sent); got != c.want {
				t.Fatalf("key=%s target=%d actual=%d fresh=%v got=%v", key, c.target, c.actual, c.fresh, got)
			}
		}
		if _, p := settingPayload(app.SettingInput{Key: key, Value: 25}); p != nil {
			t.Fatalf("off-level target reached protocol builder for %s", key)
		}
	}
}

func TestSimulatedFanRejectsUnsupportedLevelWithoutChangingState(t *testing.T) {
	m, _ := New("simulated")
	defer m.Close()
	p := app.Printer{ID: 1, ModelID: "x2d", Enabled: true}
	m.Reconcile(context.Background(), []app.Printer{p})
	if err := m.SetEquipment(context.Background(), p, app.SettingInput{Key: "fan:1", Value: 25}); err == nil {
		t.Fatal("driver accepted unsupported fine-grained target")
	}
	r, _ := findSetting(m.Status(1), "fan:1")
	if r.Value != 0 {
		t.Fatal("rejected input changed fan")
	}
	if err := m.SetEquipment(context.Background(), p, app.SettingInput{Key: "fan:1", Value: 20}); err != nil {
		t.Fatal(err)
	}
	r, _ = findSetting(m.Status(1), "fan:1")
	if r.Value != 20 {
		t.Fatal("supported level not applied")
	}
}
