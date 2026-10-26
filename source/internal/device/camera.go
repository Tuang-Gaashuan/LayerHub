package device

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"image/jpeg"
	"io"
	"mime/multipart"
	"net"
	"net/url"
	"os/exec"
	"sync"
	"time"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/base"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/pion/rtp"
	"layerhub.local/server/internal/app"
)

type cameraSession struct {
	mu      sync.Mutex
	frame   app.CameraFrame
	changed chan struct{}
	done    chan struct{}
	lastUse time.Time
	failed  bool
}

func (m *Manager) CameraFrame(ctx context.Context, p app.Printer) (app.CameraFrame, error) {
	m.mu.Lock()
	item := m.items[p.ID]
	if m.mode != "live" || item == nil || !item.config.Enabled || !sameConfig(item.config, p) || !cameraReading(item.config, item.raw).Supported {
		m.mu.Unlock()
		return app.CameraFrame{}, errors.New("camera unavailable")
	}
	session := item.camera
	if session != nil {
		select {
		case <-session.done:
			session = nil
		default:
		}
	}
	if session == nil {
		session = &cameraSession{changed: make(chan struct{}), done: make(chan struct{}), lastUse: time.Now()}
		item.camera = session
		protocol := cameraReading(item.config, item.raw).Protocol
		m.wg.Add(1)
		go func() { defer m.wg.Done(); session.run(item.ctx, item.config, protocol) }()
	}
	m.mu.Unlock()
	return session.get(ctx)
}

func (s *cameraSession) get(ctx context.Context) (app.CameraFrame, error) {
	for {
		s.mu.Lock()
		s.lastUse = time.Now()
		frame, failed, changed := s.frame, s.failed, s.changed
		if !frame.Updated.IsZero() && time.Since(frame.Updated) <= 8*time.Second {
			frame.JPEG = bytes.Clone(frame.JPEG)
			s.mu.Unlock()
			return frame, nil
		}
		s.mu.Unlock()
		if failed {
			return app.CameraFrame{}, errors.New("camera temporarily unavailable")
		}
		select {
		case <-ctx.Done():
			return app.CameraFrame{}, ctx.Err()
		case <-s.done:
			return app.CameraFrame{}, errors.New("camera stopped")
		case <-changed:
		}
	}
}

func (s *cameraSession) update(frame []byte) error {
	if len(frame) == 0 || len(frame) > 2<<20 {
		return errors.New("invalid camera frame size")
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(frame))
	if err != nil || cfg.Width > 4096 || cfg.Height > 4096 || cfg.Width <= 0 || cfg.Height <= 0 {
		return errors.New("invalid camera JPEG")
	}
	s.mu.Lock()
	s.frame = app.CameraFrame{JPEG: frame, Updated: time.Now()}
	s.failed = false
	close(s.changed)
	s.changed = make(chan struct{})
	s.mu.Unlock()
	return nil
}

func (s *cameraSession) run(parent context.Context, p app.Printer, protocol string) {
	defer close(s.done)
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.mu.Lock()
				idle := time.Since(s.lastUse) > 20*time.Second
				s.mu.Unlock()
				if idle {
					cancel()
					return
				}
			}
		}
	}()
	for ctx.Err() == nil {
		if protocol == "rtsps" {
			_ = s.rtsp(ctx, p)
		} else {
			_ = s.legacy(ctx, p)
		}
		s.mu.Lock()
		s.failed = true
		s.frame = app.CameraFrame{}
		close(s.changed)
		s.changed = make(chan struct{})
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}
	}
}

func (s *cameraSession) legacy(ctx context.Context, p app.Printer) error {
	return s.legacyAt(ctx, p, "6000")
}

func (s *cameraSession) legacyAt(ctx context.Context, p app.Printer, port string) error {
	dialer := &tls.Dialer{NetDialer: &net.Dialer{Timeout: 6 * time.Second}, Config: tlsFor(p)}
	stream, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(p.IP, port))
	if err != nil {
		return err
	}
	defer stream.Close()
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			stream.Close()
		case <-done:
		}
	}()
	if len(p.Code) > 32 {
		return errors.New("unsupported camera credential length")
	}
	auth := make([]byte, 80)
	binary.LittleEndian.PutUint32(auth, 0x40)
	binary.LittleEndian.PutUint32(auth[4:], 0x3000)
	copy(auth[16:48], "bblp")
	copy(auth[48:], p.Code)
	stream.SetWriteDeadline(time.Now().Add(6 * time.Second))
	if _, err = stream.Write(auth); err != nil {
		return err
	}
	for ctx.Err() == nil {
		stream.SetReadDeadline(time.Now().Add(12 * time.Second))
		header := make([]byte, 16)
		if _, err = io.ReadFull(stream, header); err != nil {
			return err
		}
		size := binary.LittleEndian.Uint32(header)
		if size < 4 || size > 2<<20 || binary.LittleEndian.Uint32(header[4:]) != 0 || binary.LittleEndian.Uint32(header[8:]) != 1 {
			return errors.New("unsupported camera packet")
		}
		frame := make([]byte, size)
		if _, err = io.ReadFull(stream, frame); err != nil {
			return err
		}
		if err = s.update(frame); err != nil {
			return err
		}
	}
	return ctx.Err()
}

func (s *cameraSession) rtsp(ctx context.Context, p app.Printer) error {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return errors.New("video decoder not installed")
	}
	u, err := base.ParseURL("rtsps://" + net.JoinHostPort(p.IP, "322") + "/streaming/live/1")
	if err != nil {
		return err
	}
	u.User = url.UserPassword("bblp", p.Code)
	tcp := gortsplib.ProtocolTCP
	client := &gortsplib.Client{Scheme: u.Scheme, Host: u.Host, Protocol: &tcp, TLSConfig: tlsFor(p), ReadTimeout: 8 * time.Second, WriteTimeout: 6 * time.Second}
	expectedAddress := net.JoinHostPort(p.IP, "322")
	client.DialContext = func(dialCtx context.Context, network, address string) (net.Conn, error) {
		if network != "tcp" || address != expectedAddress {
			return nil, errors.New("camera redirect outside configured device refused")
		}
		return (&net.Dialer{Timeout: 6 * time.Second}).DialContext(dialCtx, network, address)
	}
	if err = client.Start(); err != nil {
		return err
	}
	defer client.Close()
	stopped := make(chan struct{})
	defer close(stopped)
	go func() {
		select {
		case <-ctx.Done():
			client.Close()
		case <-stopped:
		}
	}()
	desc, _, err := client.Describe(u)
	if err != nil {
		return err
	}
	var h264 *format.H264
	var h265 *format.H265
	var media *description.Media
	var video format.Format
	var decode func(*rtp.Packet) ([][]byte, error)
	var params [][]byte
	codec := "h264"
	if media = desc.FindFormat(&h264); media != nil {
		decoder, e := h264.CreateDecoder()
		if e != nil {
			return e
		}
		decode = decoder.Decode
		video = h264
		sps, pps := h264.SafeParams()
		params = [][]byte{sps, pps}
	} else if media = desc.FindFormat(&h265); media != nil {
		decoder, e := h265.CreateDecoder()
		if e != nil {
			return e
		}
		decode = decoder.Decode
		video = h265
		codec = "hevc"
		vps, sps, pps := h265.SafeParams()
		params = [][]byte{vps, sps, pps}
	} else {
		return errors.New("camera video codec unsupported")
	}
	if _, err = client.Setup(desc.BaseURL, media, 0, 0); err != nil {
		return err
	}
	workerCtx, workerCancel := context.WithCancel(ctx)
	defer workerCancel()
	// Only elementary video goes to FFmpeg stdin. Credentials never enter its
	// arguments, stderr, filesystem or a browser URL. TLS is verified above.
	cmd := exec.CommandContext(workerCtx, ffmpeg, "-hide_banner", "-loglevel", "quiet", "-nostdin", "-threads", "1", "-probesize", "32768", "-analyzeduration", "0", "-f", codec, "-i", "pipe:0", "-an", "-vf", "fps=8,scale=960:-2", "-threads", "1", "-c:v", "mjpeg", "-q:v", "5", "-f", "mpjpeg", "-boundary_tag", "frame", "pipe:1")
	input, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	defer output.Close()
	if err = cmd.Start(); err != nil {
		return err
	}
	defer func() { workerCancel(); _ = cmd.Wait() }()
	failures := make(chan error, 4)
	units := make(chan []byte, 16)
	sendUnits := func(au [][]byte) {
		size := 0
		for _, nalu := range au {
			size += len(nalu) + 4
		}
		if size > 2<<20 {
			select {
			case failures <- errors.New("oversized video unit"):
			default:
			}
			return
		}
		packet := make([]byte, 0, size)
		for _, nalu := range au {
			if len(nalu) > 0 {
				packet = append(packet, 0, 0, 0, 1)
				packet = append(packet, nalu...)
			}
		}
		if len(packet) == 0 {
			return
		}
		select {
		case units <- packet:
		default:
			select {
			case failures <- errors.New("video decoder overloaded"):
			default:
			}
		}
	}
	go func() {
		for {
			select {
			case <-workerCtx.Done():
				return
			case packet := <-units:
				if _, e := input.Write(packet); e != nil {
					select {
					case failures <- e:
					default:
					}
					return
				}
			}
		}
	}()
	sendUnits(params)
	client.OnPacketRTP(media, video, func(packet *rtp.Packet) {
		if au, e := decode(packet); e == nil {
			sendUnits(au)
		}
	})
	go func() {
		reader := multipart.NewReader(output, "frame")
		for {
			part, e := reader.NextPart()
			if e != nil {
				select {
				case failures <- e:
				default:
				}
				return
			}
			frame, e := io.ReadAll(io.LimitReader(part, (2<<20)+1))
			part.Close()
			if e == nil {
				e = s.update(frame)
			}
			if e != nil {
				select {
				case failures <- e:
				default:
				}
				return
			}
		}
	}()
	if _, err = client.Play(nil); err != nil {
		return err
	}
	go func() {
		e := client.Wait()
		select {
		case failures <- e:
		default:
		}
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err = <-failures:
		return err
	}
}
