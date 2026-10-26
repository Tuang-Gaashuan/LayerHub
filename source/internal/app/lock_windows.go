package app

import (
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"sync"
)

func lockDataDir(dir string) (func(), error) {
	file, err := os.OpenFile(dir+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	var overlapped windows.Overlapped
	if err = windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &overlapped); err != nil {
		file.Close()
		return nil, fmt.Errorf("data directory is already in use: %w", err)
	}
	var once sync.Once
	return func() {
		once.Do(func() { windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, &overlapped); file.Close() })
	}, nil
}
