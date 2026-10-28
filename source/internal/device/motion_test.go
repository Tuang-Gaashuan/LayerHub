package device

import (
	"context"
	"encoding/json"
	"errors"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"layerhub.local/server/internal/app"
	"testing"
	"time"
)

type motionToken struct{}

func (motionToken) Wait() bool                     { return true }
func (motionToken) WaitTimeout(time.Duration) bool { return true }
func (motionToken) Done() <-chan struct{}          { c := make(chan struct{}); close(c); return c }
func (motionToken) Error() error                   { return nil }

type motionClient struct {
	mqtt.Client
	publish func(string, byte, bool, interface{}) mqtt.Token
}

func (c motionClient) IsConnectionOpen() bool { return true }
func (c motionClient) Publish(topic string, qos byte, retain bool, payload interface{}) mqtt.Token {
	return c.publish(topic, qos, retain, payload)
}
func TestNativeJogUsesQoSZeroOnceAndDoesNotTreatAckAsHoming(t *testing.T) {
	for _, ack := range []string{"success", "failed", "missing"} {
		t.Run(ack, func(t *testing.T) {
			p := app.Printer{ID: 1, ModelID: "x2d", Enabled: true, Serial: "TEST"}
			l := &listener{config: p, raw: map[string]any{}, connected: true}
			m, _ := New("live")
			m.items[1] = l
			reportJSON(t, l, `{"gcode_state":"IDLE","fun":"44029FD1930F9CB7","home_flag":-1066969697,"print_error":0,"hms":[]}`)
			count := 0
			l.client = motionClient{publish: func(topic string, qos byte, retain bool, v interface{}) mqtt.Token {
				count++
				if qos != 0 || retain || topic != "device/TEST/request" {
					t.Fatal("unsafe transport")
				}
				var body map[string]map[string]any
				if json.Unmarshal(v.([]byte), &body) != nil {
					t.Fatal("invalid JSON")
				}
				req := body["print"]
				if req["command"] != "xyz_ctrl" || req["axis"] != "Z" || req["dir"] != float64(-1) || req["mode"] != float64(0) {
					t.Fatal(req)
				}
				if ack != "missing" {
					m.mu.Lock()
					l.replies[req["sequence_id"].(string)] = map[string]any{"result": ack}
					applyReport(l, map[string]any{"gcode_state": "IDLE"})
					m.mu.Unlock()
				}
				return motionToken{}
			}}
			ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
			defer cancel()
			err := m.Move(ctx, p, app.MotionInput{Action: "jog", Axis: "Z", Direction: -1, Step: 1})
			if count != 1 || err == nil {
				t.Fatal("missing result incorrectly completed or resent", count, err)
			}
			if ack == "failed" && !errors.Is(err, app.ErrDeviceRejected) {
				t.Fatal("rejection not explicit", err)
			}
		})
	}
}

func TestMotionSignedHomeFlagsAndCapabilityFreshness(t *testing.T) {
	l := &listener{raw: map[string]any{}, replies: map[string]map[string]any{"10": nil}}
	reportJSON(t, l, `{"fun":"44029FD1930F9CB7","home_flag":-1066969704}`)
	m := readMotion(l)
	if !m.NativeHome || !m.NativeJog || m.HomeMask == nil || *m.HomeMask != 0 || m.HomeAt.IsZero() {
		t.Fatalf("signed flags lost: %+v", m)
	}
	reportJSON(t, l, `{"home_flag":-1066969698}`)
	if *readMotion(l).HomeMask != 6 {
		t.Fatal("partial home invented")
	}
	reportJSON(t, l, `{"home_flag":-1066969697}`)
	if *readMotion(l).HomeMask != 7 {
		t.Fatal("complete home lost")
	}
	previous := readMotion(l)
	reportJSON(t, l, `{"command":"xyz_ctrl","sequence_id":"10","result":"success","home_flag":0}`)
	if *readMotion(l).HomeMask != 7 || readMotion(l).HomeAt != previous.HomeAt {
		t.Fatal("echo overwrote actual home")
	}
	l.fields["fun"] = time.Now().Add(-time.Minute)
	status := app.Telemetry{Connected: true, RawState: "FINISH", Updated: time.Now(), StateUpdated: time.Now(), Equipment: app.Equipment{Motion: readMotion(l)}}
	if app.ValidateMotion(status, app.MotionInput{Action: "jog"}) == nil {
		t.Fatal("stale capabilities accepted")
	}
}
func TestMotionMissingMalformedFlagsRemainUnknown(t *testing.T) {
	for _, raw := range []string{`{}`, `{"home_flag":null}`, `{"home_flag":1.5}`, `{"home_flag":-9999999999}`, `{"home_flag":"7"}`, `{"fun":"bogus"}`} {
		l := &listener{raw: map[string]any{}}
		reportJSON(t, l, raw)
		m := readMotion(l)
		if m.HomeMask != nil || m.NativeJog || m.NativeHome {
			t.Fatalf("invented state from %s: %+v", raw, m)
		}
	}
	l := &listener{raw: map[string]any{}}
	reportJSON(t, l, `{"home_flag":0}`)
	if *readMotion(l).HomeMask != 0 {
		t.Fatal("zero interpreted as all homed")
	}
}
func TestHomeWaitsForLateXAxisInsteadOfCompletingAt25Seconds(t *testing.T) {
	sent := time.Now()
	partial := 6
	complete := 7
	for _, v := range []struct {
		elapsed int
		mask    *int
		want    bool
	}{{25, &partial, false}, {25, &complete, false}, {61, &partial, false}, {61, &complete, true}, {61, nil, false}} {
		now := sent.Add(time.Duration(v.elapsed) * time.Second)
		r := app.MotionReading{HomeMask: v.mask, HomeAt: now}
		if motionHomeComplete(r, sent, now) != v.want {
			t.Fatal(v)
		}
	}
	if motionHomeComplete(app.MotionReading{HomeMask: &complete, HomeAt: sent.Add(-time.Second)}, sent, sent.Add(61*time.Second)) {
		t.Fatal("old home completed new action")
	}
}
