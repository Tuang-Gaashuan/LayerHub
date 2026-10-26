package device

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"layerhub.local/server/internal/app"
)

func testCertificate(t *testing.T) (*tls.Config, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cert := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test printer"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(der)
	return &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}, MinVersion: tls.VersionTLS12, MaxVersion: tls.VersionTLS12}, hex.EncodeToString(hash[:])
}

func TestCertificatePinRejectsUnexpectedDevice(t *testing.T) {
	cfg, pin := testCertificate(t)
	cert, err := x509.ParseCertificate(cfg.Certificates[0].Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	state := tls.ConnectionState{PeerCertificates: []*x509.Certificate{cert}}
	if err = tlsFor(app.Printer{IP: "127.0.0.1", Fingerprint: pin}).VerifyConnection(state); err != nil {
		t.Fatal(err)
	}
	if tlsFor(app.Printer{Fingerprint: strings.Repeat("0", 64)}).VerifyConnection(state) == nil {
		t.Fatal("wrong certificate accepted")
	}
	if tlsFor(app.Printer{IP: "127.0.0.1"}).InsecureSkipVerify {
		t.Fatal("default verification disabled")
	}
}

func TestImplicitFTPSUploadReusesTLSSession(t *testing.T) {
	cfg, pin := testCertificate(t)
	control, err := tls.Listen("tcp", "127.0.0.1:0", cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer control.Close()
	payload := append(bytes.Repeat([]byte{0, 13, 10, 255}, 16000), 13, 10)
	path := filepath.Join(t.TempDir(), "sample.gcode.3mf")
	if err = os.WriteFile(path, payload, 0600); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		conn, e := control.Accept()
		if e != nil {
			result <- e
			return
		}
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(8 * time.Second))
		reply := func(s string) { fmt.Fprint(conn, s+"\r\n") }
		reply("220 test printer")
		reader := bufio.NewReader(conn)
		var passive net.Listener
		defer func() {
			if passive != nil {
				passive.Close()
			}
		}()
		var stored []byte
		for {
			line, e := reader.ReadString('\n')
			if e != nil {
				result <- e
				return
			}
			cmd := strings.Fields(line)
			if len(cmd) == 0 {
				continue
			}
			switch cmd[0] {
			case "USER":
				reply("331 password")
			case "PASS":
				reply("230 logged in")
			case "FEAT":
				reply("211 no features")
			case "PBSZ", "PROT", "TYPE", "OPTS":
				reply("200 okay")
			case "EPSV":
				passive, e = net.Listen("tcp", "127.0.0.1:0")
				if e != nil {
					result <- e
					return
				}
				port := passive.Addr().(*net.TCPAddr).Port
				reply(fmt.Sprintf("229 Entering Extended Passive Mode (|||%d|)", port))
			case "STOR":
				reply("150 opening data")
				data, e := passive.Accept()
				if e != nil {
					result <- e
					return
				}
				data.SetDeadline(time.Now().Add(5 * time.Second))
				secured := tls.Server(data, cfg)
				if e = secured.Handshake(); e != nil {
					data.Close()
					result <- e
					return
				}
				resumed := secured.ConnectionState().DidResume
				stored, e = io.ReadAll(secured)
				secured.Close()
				passive.Close()
				passive = nil
				if e != nil {
					result <- e
					return
				}
				if !resumed {
					result <- fmt.Errorf("data TLS connection did not reuse control session")
					return
				}
				if !bytes.Equal(stored, payload) {
					result <- fmt.Errorf("binary payload changed")
					return
				}
				reply("226 stored")
			case "SIZE":
				reply(fmt.Sprintf("213 %d", len(stored)))
			case "QUIT":
				reply("221 bye")
				result <- nil
				return
			default:
				result <- fmt.Errorf("unexpected FTP command %s", cmd[0])
				return
			}
		}
	}()
	m, _ := New("live")
	m.ftpPort = strconv.Itoa(control.Addr().(*net.TCPAddr).Port)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	var progress int64
	err = m.Upload(ctx, app.Printer{IP: "127.0.0.1", Code: "test-only", Fingerprint: pin}, path, "job-1-test.3mf", func(n int64) { progress = n })
	if err != nil {
		t.Fatal(err)
	}
	if progress != int64(len(payload)) {
		t.Fatalf("progress %d", progress)
	}
	if err = <-result; err != nil {
		t.Fatal(err)
	}
}

func readMQTT(reader *bufio.Reader) (byte, []byte, error) {
	first, err := reader.ReadByte()
	if err != nil {
		return 0, nil, err
	}
	size, mul := 0, 1
	for i := 0; i < 4; i++ {
		v, e := reader.ReadByte()
		if e != nil {
			return 0, nil, e
		}
		size += int(v&127) * mul
		if v&128 == 0 {
			if size > 1<<20 {
				return 0, nil, fmt.Errorf("packet too large")
			}
			body := make([]byte, size)
			_, e = io.ReadFull(reader, body)
			return first, body, e
		}
		mul *= 128
	}
	return 0, nil, fmt.Errorf("invalid MQTT size")
}
func publishMQTT(conn net.Conn, topic, payload string) error {
	body := make([]byte, 2)
	binary.BigEndian.PutUint16(body, uint16(len(topic)))
	body = append(body, topic...)
	body = append(body, payload...)
	packet := []byte{0x30}
	n := len(body)
	for {
		v := byte(n % 128)
		n /= 128
		if n > 0 {
			v |= 128
		}
		packet = append(packet, v)
		if n == 0 {
			break
		}
	}
	_, err := conn.Write(append(packet, body...))
	return err
}

func TestMQTTSubscriptionDeltaMergeAndShutdown(t *testing.T) {
	cfg, pin := testCertificate(t)
	broker, err := tls.Listen("tcp", "127.0.0.1:0", cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer broker.Close()
	result := make(chan error, 1)
	go func() {
		conn, e := broker.Accept()
		if e != nil {
			result <- e
			return
		}
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(8 * time.Second))
		reader := bufio.NewReader(conn)
		kind, _, e := readMQTT(reader)
		if e != nil || kind>>4 != 1 {
			result <- fmt.Errorf("CONNECT: %v", e)
			return
		}
		conn.Write([]byte{0x20, 2, 0, 0})
		kind, body, e := readMQTT(reader)
		if e != nil || kind>>4 != 8 || len(body) < 2 {
			result <- fmt.Errorf("SUBSCRIBE: %v", e)
			return
		}
		conn.Write([]byte{0x90, 3, body[0], body[1], 0})
		kind, body, e = readMQTT(reader)
		if e != nil || kind>>4 != 3 {
			result <- fmt.Errorf("pushall: %v", e)
			return
		}
		topicLength := int(binary.BigEndian.Uint16(body))
		var request map[string]any
		if json.Unmarshal(body[2+topicLength:], &request) != nil || request["pushing"].(map[string]any)["command"] != "pushall" || request["pushing"].(map[string]any)["version"] != float64(1) || request["pushing"].(map[string]any)["push_target"] != float64(1) {
			result <- fmt.Errorf("invalid pushall payload")
			return
		}
		publishMQTT(conn, "device/TEST/report", `{"print":{"gcode_state":"IDLE","nozzle_temper":210,"ams":{"ams":[{"id":"0","tray":[{"id":"0","tray_type":"PLA","remain":75},{"id":"1","tray_type":"PETG"}]}]}}}`)
		publishMQTT(conn, "device/TEST/report", `{"print":{"mc_percent":42,"ams":{"ams":[{"id":"0","tray":[{"id":"0","remain":60}]}]}}}`)
		for {
			kind, _, e = readMQTT(reader)
			if e != nil || kind>>4 == 14 {
				result <- nil
				return
			}
			if kind>>4 == 12 {
				conn.Write([]byte{0xd0, 0})
			}
		}
	}()
	m, _ := New("live")
	m.mqttPort = strconv.Itoa(broker.Addr().(*net.TCPAddr).Port)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := app.Printer{ID: 1, IP: "127.0.0.1", Serial: "TEST", Enabled: true, Code: "test-only", Fingerprint: pin, AMS: map[string]any{"ignored": true}}
	m.Reconcile(ctx, []app.Printer{p})
	m.Reconcile(ctx, []app.Printer{p}) // Non-comparable telemetry must not panic or restart listeners.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		status := m.Status(1)
		if status.Percent == 42 {
			units := status.AMS.(map[string]any)["ams"].([]any)
			trays := units[0].(map[string]any)["tray"].([]any)
			if len(trays) != 2 || trays[0].(map[string]any)["tray_type"] != "PLA" || trays[0].(map[string]any)["remain"] != float64(60) {
				t.Fatal("incremental tray payload lost earlier fields")
			}
			trays[0].(map[string]any)["remain"] = 0
			fresh := m.Status(1).AMS.(map[string]any)["ams"].([]any)[0].(map[string]any)["tray"].([]any)[0].(map[string]any)["remain"]
			if fresh != float64(60) {
				t.Fatal("caller mutated listener state")
			}
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if m.Status(1).Percent != 42 {
		t.Fatal("MQTT reports not received")
	}
	cancel()
	closed := make(chan struct{})
	go func() { m.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("MQTT listener shutdown blocked")
	}
	if err = <-result; err != nil {
		t.Fatal(err)
	}
}

func TestSimulatedPrintFinishesWithoutPhysicalDevice(t *testing.T) {
	m, _ := New("simulated")
	defer m.Close()
	m.Reconcile(context.Background(), []app.Printer{{ID: 1, Enabled: true}})
	if err := m.Start(context.Background(), app.Printer{ID: 1}, "sample.gcode"); err != nil {
		t.Fatal(err)
	}
	if m.Status(1).State != "printing" {
		t.Fatal("simulation did not start")
	}
	m.mu.Lock()
	m.items[1].started = time.Now().Add(-13 * time.Second)
	m.mu.Unlock()
	if got := m.Status(1); got.State != "idle" || got.Percent != 100 {
		t.Fatalf("simulation did not finish: %+v", got)
	}
}

func TestListenerCanStopWhileBrokerUnavailable(t *testing.T) {
	port, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := strconv.Itoa(port.Addr().(*net.TCPAddr).Port)
	port.Close()
	m, _ := New("live")
	m.mqttPort = address
	ctx, cancel := context.WithCancel(context.Background())
	m.Reconcile(ctx, []app.Printer{{ID: 1, IP: "127.0.0.1", Serial: "LOCAL", Enabled: true}})
	cancel()
	closed := make(chan struct{})
	go func() { m.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("unavailable MQTT broker prevented shutdown")
	}
}
