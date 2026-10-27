package updatekit

import (
	"errors"
	"fmt"
	"strings"
)

// activation separates service state transitions from data recovery. There is
// deliberately no database-restore callback: once the new reader starts, its
// matching program and any new writes must remain together for review.
type activation struct {
	before                                    []byte
	stop, start, backup, selectNew, selectOld func() error
	inspect                                   func() ([]byte, error)
}

func activate(a activation) (result error) {
	if e := a.stop(); e != nil {
		return errors.Join(e, a.start())
	}
	startedNew := false
	switchAttempted := false
	defer func() {
		if !startedNew {
			if switchAttempted {
				if e := a.selectOld(); e != nil {
					result = errors.Join(result, fmt.Errorf("无法确认旧程序指向，未继续启动: %w", e))
					return
				}
			}
			result = errors.Join(result, a.start())
		}
	}()
	after, e := a.inspect()
	if e != nil {
		return e
	}
	if strings.TrimSpace(string(a.before)) != strings.TrimSpace(string(after)) {
		return errors.New("停止前后任务状态变化，已取消切换")
	}
	if e = a.backup(); e != nil {
		return e
	}
	switchAttempted = true
	if e = a.selectNew(); e != nil {
		return e
	}
	startedNew = true
	return a.start()
}
