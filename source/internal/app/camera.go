package app

import (
	"bytes"
	"context"
	"fmt"
	"image/jpeg"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strconv"
	"time"
)

// The enclosing /api/ handler checks the session before this image endpoint.
func (s *Service) cameraHTTP(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.URL.Query().Get("printerId"), 10, 64)
	if err != nil || id <= 0 {
		fail(w, invalid("请选择有效的打印机"))
		return
	}
	p, err := s.devicePrinter(id)
	if err != nil {
		fail(w, missing("打印机不存在"))
		return
	}
	if !p.Enabled || s.Config.DeviceMode != "live" {
		fail(w, conflict("摄像头需要已启用的真实设备连接"))
		return
	}
	provider, ok := s.Device.(CameraProvider)
	if !ok {
		fail(w, conflict("当前驱动没有提供摄像头"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 18*time.Second)
	defer cancel()
	after := time.Time{}
	if r.URL.Query().Get("capture") == "1" {
		after = time.Now()
	}
	frame, err := cameraFrameAfter(ctx, provider, p, after)
	if err != nil {
		fail(w, &apiError{503, "摄像头暂时不可用，请检查设备视频设置及 NAS 视频组件后重试"})
		return
	}
	if r.URL.Query().Get("stream") == "1" {
		s.cameraStreamHTTP(w, r, provider, p, frame)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "no-store, private")
	w.Header().Set("X-Frame-Updated", strconv.FormatInt(frame.Updated.UnixMilli(), 10))
	w.Header().Set("Content-Length", strconv.Itoa(len(frame.JPEG)))
	if r.URL.Query().Get("capture") == "1" {
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="LayerHub_%d_%s.jpg"`, p.ID, frame.Updated.UTC().Format("20060102_150405")))
	}
	w.Write(frame.JPEG)
}

func cameraFrameAfter(ctx context.Context, provider CameraProvider, p Printer, after time.Time) (CameraFrame, error) {
	for {
		frame, err := provider.CameraFrame(ctx, p)
		if err != nil {
			return CameraFrame{}, err
		}
		if len(frame.JPEG) > 2<<20 || frame.Updated.IsZero() || time.Since(frame.Updated) > 8*time.Second || time.Until(frame.Updated) > time.Second {
			return CameraFrame{}, fmt.Errorf("expired camera frame")
		}
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(frame.JPEG))
		if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 4096 || cfg.Height > 4096 {
			return CameraFrame{}, fmt.Errorf("invalid camera image")
		}
		if frame.Updated.After(after) {
			return frame, nil
		}
		select {
		case <-ctx.Done():
			return CameraFrame{}, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// Viewers share the driver's single camera/decoder. Each connection only writes
// the latest frame; a slow client cannot grow a queue of old video frames.
func (s *Service) cameraStreamHTTP(w http.ResponseWriter, r *http.Request, provider CameraProvider, p Printer, frame CameraFrame) {
	writer := multipart.NewWriter(w)
	_ = writer.SetBoundary("layerhub-frame")
	w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary="+writer.Boundary())
	w.Header().Set("Cache-Control", "no-store, private")
	w.Header().Set("X-Accel-Buffering", "no")
	control := http.NewResponseController(w)
	defer control.SetWriteDeadline(time.Time{})
	checked := time.Now()
	for {
		if time.Since(checked) >= 2*time.Second {
			if _, _, err := s.session(r); err != nil {
				return
			}
			checked = time.Now()
		}
		if err := control.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
			return
		}
		header := textproto.MIMEHeader{}
		header.Set("Content-Type", "image/jpeg")
		header.Set("Content-Length", strconv.Itoa(len(frame.JPEG)))
		header.Set("X-Frame-Updated", strconv.FormatInt(frame.Updated.UnixMilli(), 10))
		part, err := writer.CreatePart(header)
		if err != nil {
			return
		}
		if _, err = part.Write(frame.JPEG); err != nil {
			return
		}
		if err = control.Flush(); err != nil {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		next, err := cameraFrameAfter(ctx, provider, p, frame.Updated)
		cancel()
		if err != nil {
			return
		}
		frame = next
	}
}
