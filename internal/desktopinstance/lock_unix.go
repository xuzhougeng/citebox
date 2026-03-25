//go:build darwin || linux

package desktopinstance

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

type fileLock struct {
	file *os.File
}

func openFileLock(path string) (*fileLock, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open desktop instance lock: %w", err)
	}
	return &fileLock{file: file}, nil
}

func (l *fileLock) TryLock() (bool, error) {
	if err := syscall.Flock(int(l.file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return false, nil
		}
		return false, fmt.Errorf("acquire desktop instance lock: %w", err)
	}
	return true, nil
}

func (l *fileLock) Close() error {
	if l == nil || l.file == nil {
		return nil
	}

	unlockErr := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	closeErr := l.file.Close()
	l.file = nil

	if unlockErr != nil {
		return fmt.Errorf("release desktop instance lock: %w", unlockErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close desktop instance lock: %w", closeErr)
	}
	return nil
}
