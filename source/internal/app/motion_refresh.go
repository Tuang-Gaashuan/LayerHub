package app

import (
	"context"
	"time"
)

type MotionRefresher interface {
	RefreshMotion(context.Context, Printer) error
}

func (s *Service) RefreshMotion(u User, id int64) error {
	if !u.Admin {
		return forbidden("手动移动仅管理员可操作")
	}
	p, e := s.devicePrinter(id)
	if e != nil {
		return e
	}
	if !p.Enabled {
		return conflict("设备已停用")
	}
	d, ok := s.Device.(MotionRefresher)
	if !ok || s.Config.DeviceMode != "live" {
		return conflict("设备驱动没有提供状态刷新")
	}
	ctx, cancel := context.WithTimeout(s.ctx, 8*time.Second)
	defer cancel()
	if e = d.RefreshMotion(ctx, p); e != nil {
		return conflict("未取得新的移动状态，请稍后重新读取")
	}
	return nil
}
