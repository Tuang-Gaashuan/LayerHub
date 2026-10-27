package device

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"layerhub.local/server/internal/app"
	"net"
	"strconv"
	"testing"
	"time"
)

func TestMQTTControlNeedsFreshMatchingStateAndRejectsFailure(t *testing.T) {
	cfg, pin := testCertificate(t)
	broker, e := tls.Listen("tcp", "127.0.0.1:0", cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer broker.Close()
	brokerResult := make(chan error, 1)
	release := make(chan struct{})
	ackOnly := make(chan struct{})
	go func() {
		conn, e := broker.Accept()
		if e != nil {
			brokerResult <- e
			return
		}
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(8 * time.Second))
		r := bufio.NewReader(conn)
		if _, _, e = readMQTT(r); e != nil {
			brokerResult <- e
			return
		}
		conn.Write([]byte{0x20, 2, 0, 0})
		_, b, e := readMQTT(r)
		if e != nil {
			brokerResult <- e
			return
		}
		conn.Write([]byte{0x90, 3, b[0], b[1], 0})
		if _, _, e = readMQTT(r); e != nil {
			brokerResult <- e
			return
		}
		publishMQTT(conn, "device/TEST/report", `{"print":{"gcode_state":"IDLE"}}`)
		for i := 0; i < 2; i++ {
			kind, b, e := readMQTT(r)
			if e != nil || kind>>4 != 3 {
				brokerResult <- fmt.Errorf("control packet: %v", e)
				return
			}
			n := int(binary.BigEndian.Uint16(b))
			packetID := b[2+n : 4+n]
			conn.Write([]byte{0x40, 2, packetID[0], packetID[1]})
			var request map[string]map[string]any
			if e = json.Unmarshal(b[4+n:], &request); e != nil {
				brokerResult <- e
				return
			}
			seq := request["print"]["sequence_id"]
			if i == 0 {
				msg, _ := json.Marshal(map[string]any{"print": map[string]any{"sequence_id": seq, "command": "project_file", "result": "success"}})
				publishMQTT(conn, "device/TEST/report", string(msg))
				publishMQTT(conn, "device/TEST/report", `{"print":{"gcode_state":"RUNNING","subtask_name":"someone-else"}}`)
				close(ackOnly)
				<-release
				publishMQTT(conn, "device/TEST/report", `{"print":{"gcode_state":"RUNNING","subtask_name":"job-7","gcode_file":"Metadata/plate_2.gcode"}}`)
			} else {
				msg, _ := json.Marshal(map[string]any{"print": map[string]any{"sequence_id": seq, "command": "pause", "result": "fail"}})
				publishMQTT(conn, "device/TEST/report", string(msg))
			}
		}
		for {
			kind, _, e := readMQTT(r)
			if e != nil || kind>>4 == 14 {
				brokerResult <- nil
				return
			}
		}
	}()
	m, _ := New("live")
	m.mqttPort = strconv.Itoa(broker.Addr().(*net.TCPAddr).Port)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer m.Close()
	p := app.Printer{ID: 1, IP: "127.0.0.1", Serial: "TEST", Enabled: true, Fingerprint: pin}
	m.Reconcile(ctx, []app.Printer{p})
	until := time.Now().Add(3 * time.Second)
	for time.Now().Before(until) {
		if m.Status(1).State == "idle" {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	operation, stop := context.WithTimeout(ctx, 4*time.Second)
	defer stop()
	result := make(chan error, 1)
	go func() {
		result <- m.StartPrint(operation, p, app.PrintRequest{Remote: "sample.3mf", TaskName: "job-7", Options: &app.PrintOptions{Plate: 2}})
	}()
	select {
	case <-ackOnly:
	case <-time.After(3 * time.Second):
		t.Fatal("no command ack")
	}
	select {
	case err := <-result:
		t.Fatalf("ack or unrelated task confirmed execution: %v", err)
	case <-time.After(250 * time.Millisecond):
	}
	close(release)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	if err := m.Control(operation, p, "pause"); err == nil {
		t.Fatal("negative command result accepted")
	}
	cancel()
	m.Close()
	if err := <-brokerResult; err != nil {
		t.Fatal(err)
	}
}

func TestFailedStateAndUnrelatedDeltasCannotConfirmStart(t *testing.T) {
	l := &listener{raw: map[string]any{}}
	applyReport(l, map[string]any{"gcode_state": "FAILED", "subtask_name": "old.gcode"})
	old := l.stateUpdated
	applyReport(l, map[string]any{"mc_percent": 12})
	if !old.Equal(l.stateUpdated) {
		t.Fatal("unrelated delta refreshed state timestamp")
	}
	m, _ := New("offline")
	m.items[1] = l
	if m.Status(1).State != "failed" {
		t.Fatal("failed exposed as idle")
	}
	req := app.PrintRequest{Remote: "unique.3mf", TaskName: "job-7"}
	if matchesPrint(app.Telemetry{File: "Metadata/plate_1.gcode"}, req) {
		t.Fatal("generic plate name matches job")
	}
	if !matchesPrint(app.Telemetry{File: "job-7"}, req) {
		t.Fatal("display task name not matched")
	}
}
func TestProjectPayloadPreservesSparseFilamentIndices(t *testing.T) {
	payload := printPayload(app.PrintRequest{Remote: "unique.3mf", TaskName: "job-7", Options: &app.PrintOptions{Plate: 2, UseAMS: true, Mapping: []app.AMSTarget{{Filament: 0, AMSID: 0, SlotID: 2}, {Filament: 3, AMSID: 1, SlotID: 1}}, BedLeveling: true}})
	legacy := payload["ams_mapping"].([]int)
	modern := payload["ams_mapping2"].([]map[string]int)
	if payload["command"] != "project_file" || payload["param"] != "Metadata/plate_2.gcode" || payload["url"] != "ftp:///unique.3mf" || len(legacy) != 4 || legacy[0] != 2 || legacy[1] != -1 || legacy[3] != 5 || modern[3]["ams_id"] != 1 || modern[3]["slot_id"] != 1 {
		t.Fatalf("bad project payload: %+v", payload)
	}
}

func TestX2DSingleNozzleProjectEnvelope(t *testing.T) {
	payload := printPayload(app.PrintRequest{Remote: "job-1-test.3mf", TaskName: "job-1", BedType: "pte", Options: &app.PrintOptions{Plate: 1, UseAMS: true, BedLeveling: true, Mapping: []app.AMSTarget{{Filament: 0, AMSID: 0, SlotID: 3, Grams: 38}}}})
	modern := payload["ams_mapping2"].([]map[string]int)
	if payload["bed_type"] != "pte" || payload["bed_levelling"] != true || payload["url"] != "ftp:///job-1-test.3mf" || payload["param"] != "Metadata/plate_1.gcode" || modern[0]["ams_id"] != 0 || modern[0]["slot_id"] != 3 || payload["use_ams"] != true {
		t.Fatal(payload)
	}
	if _, present := payload["nozzle_mapping"]; present {
		t.Fatal("fixed-nozzle file was given an unverified nozzle-rack override")
	}
}

func TestA1BareExternalSpoolSelectsExactPlateWithoutAMS(t *testing.T) {
	payload := printPayload(app.PrintRequest{Remote: "a1-plates.3mf", TaskName: "job-fixture", Options: &app.PrintOptions{Plate: 7, UseAMS: false, Mapping: []app.AMSTarget{{Filament: 0, AMSID: 0, SlotID: 0, SpoolID: "external", Grams: 12}}, BedLeveling: true}})
	legacy := payload["ams_mapping"].([]int)
	modern := payload["ams_mapping2"].([]map[string]int)
	if payload["param"] != "Metadata/plate_7.gcode" || payload["use_ams"] != false || legacy[0] != -1 || modern[0]["ams_id"] != 255 || modern[0]["slot_id"] != 255 {
		t.Fatal(payload)
	}
	if _, present := payload["nozzle_mapping"]; present {
		t.Fatal("A1 was given a dual-nozzle override")
	}
}
func TestSimulatedPauseResumeAndStop(t *testing.T) {
	m, _ := New("simulated")
	defer m.Close()
	p := app.Printer{ID: 1, Enabled: true}
	m.Reconcile(context.Background(), []app.Printer{p})
	m.Start(context.Background(), p, "local.gcode")
	m.Control(context.Background(), p, "pause")
	before := m.Status(1)
	m.mu.Lock()
	m.items[1].started = time.Now().Add(-13 * time.Second)
	m.mu.Unlock()
	if m.Status(1).State != "pause" {
		t.Fatal("paused simulation completed")
	}
	m.Control(context.Background(), p, "stop")
	after := m.Status(1)
	if after.State != "idle" || before.File != after.File {
		t.Fatal("stop status incorrect")
	}
}
