package device

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"image"
	"image/jpeg"
	"io"
	"layerhub.local/server/internal/app"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

func testCameraJPEG(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if e := jpeg.Encode(&b, image.NewRGBA(image.Rect(0, 0, 16, 9)), nil); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}

func TestCameraCacheSharesFreshFramesWithoutSharingMutableBuffers(t *testing.T) {
	frame := testCameraJPEG(t)
	session := &cameraSession{changed: make(chan struct{}), done: make(chan struct{})}
	if e := session.update(frame); e != nil {
		t.Fatal(e)
	}
	p := app.Printer{ID: 1, IP: "192.0.2.1", ModelID: "x2d", Enabled: true}
	m, _ := New("live")
	m.items[1] = &listener{config: p, camera: session, raw: map[string]any{"ipcam": map[string]any{"rtsp_url": "rtsps://192.0.2.1:322/streaming/live/1"}}}
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f, e := m.CameraFrame(context.Background(), p)
			if e != nil || !bytes.Equal(f.JPEG, frame) {
				t.Error("shared frame unavailable")
			}
			if len(f.JPEG) > 0 {
				f.JPEG[0] = 0
			}
		}()
	}
	wg.Wait()
	if !bytes.Equal(session.frame.JPEG, frame) || session.frame.JPEG[0] != 255 {
		t.Fatal("one viewer corrupted another viewer")
	}
	session.frame.Updated = time.Now().Add(-time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, e := session.get(ctx); e == nil {
		t.Fatal("old image presented as live")
	}
	if e := session.update([]byte("invalid")); e == nil {
		t.Fatal("invalid frame cached")
	}
	p.Enabled = false
	if _, e := m.CameraFrame(context.Background(), p); e == nil {
		t.Fatal("changed/disabled device reused camera")
	}
}

func TestLegacyCameraPinnedTLSAndFragmentedPackets(t *testing.T) {
	cert, pin := testCertificate(t)
	server, e := tls.Listen("tcp", "127.0.0.1:0", cert)
	if e != nil {
		t.Fatal(e)
	}
	defer server.Close()
	_, port, _ := net.SplitHostPort(server.Addr().String())
	frame := testCameraJPEG(t)
	result := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go func() {
		conn, e := server.Accept()
		if e != nil {
			result <- e
			return
		}
		defer conn.Close()
		auth := make([]byte, 80)
		_, e = io.ReadFull(conn, auth)
		if e == nil && (binary.LittleEndian.Uint32(auth) != 0x40 || binary.LittleEndian.Uint32(auth[4:]) != 0x3000 || string(bytes.TrimRight(auth[48:], "\x00")) != "test-code") {
			e = io.ErrUnexpectedEOF
		}
		if e != nil {
			result <- e
			return
		}
		header := make([]byte, 16)
		binary.LittleEndian.PutUint32(header, uint32(len(frame)))
		binary.LittleEndian.PutUint32(header[8:], 1)
		packet := append(header, frame...)
		for len(packet) > 0 {
			size := 7
			if len(packet) < size {
				size = len(packet)
			}
			_, e = conn.Write(packet[:size])
			if e != nil {
				result <- e
				return
			}
			packet = packet[size:]
		}
		result <- nil
		<-ctx.Done()
	}()
	session := &cameraSession{changed: make(chan struct{}), done: make(chan struct{})}
	finished := make(chan error, 1)
	go func() {
		finished <- session.legacyAt(ctx, app.Printer{IP: "127.0.0.1", Code: "test-code", Fingerprint: pin}, port)
	}()
	got, e := session.get(ctx)
	if e != nil || !bytes.Equal(got.JPEG, frame) {
		t.Fatal("fragmented JPEG failed", e)
	}
	if e = <-result; e != nil {
		t.Fatal(e)
	}
	cancel()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("camera did not close on cancellation")
	}
}

func TestCameraRejectsWrongCertificateBeforeSendingAccessCode(t *testing.T) {
	cert, _ := testCertificate(t)
	server, e := tls.Listen("tcp", "127.0.0.1:0", cert)
	if e != nil {
		t.Fatal(e)
	}
	defer server.Close()
	_, port, _ := net.SplitHostPort(server.Addr().String())
	bytesRead := make(chan int, 1)
	go func() {
		c, e := server.Accept()
		if e != nil {
			bytesRead <- 0
			return
		}
		defer c.Close()
		c.SetReadDeadline(time.Now().Add(time.Second))
		b := make([]byte, 80)
		n, _ := c.Read(b)
		bytesRead <- n
	}()
	session := &cameraSession{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if e := session.legacyAt(ctx, app.Printer{IP: "127.0.0.1", Code: "do-not-send", Fingerprint: strings.Repeat("0", 64)}, port); e == nil {
		t.Fatal("untrusted camera accepted")
	}
	if n := <-bytesRead; n != 0 {
		t.Fatal("camera credential sent before certificate pin verification")
	}
}
