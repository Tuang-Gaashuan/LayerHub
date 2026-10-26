//go:build linux || darwin

package app

import (
	"fmt"
	"os"
	"sync"
	"syscall"
)

func lockDataDir(dir string) (func(), error) {
	file, err := os.OpenFile(dir+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		return nil, fmt.Errorf("data directory is already in use: %w", err)
	}
	var once sync.Once
	return func() { once.Do(func() { syscall.Flock(int(file.Fd()), syscall.LOCK_UN); file.Close() }) }, nil
}
