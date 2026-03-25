//go:build windows

package desktopinstance

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows"
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
	var overlapped windows.Overlapped
	err := windows.LockFileEx(
		windows.Handle(l.file.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0,
		1,
		0,
		&overlapped,
	)
	if err != nil {
		if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
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

	var overlapped windows.Overlapped
	unlockErr := windows.UnlockFileEx(windows.Handle(l.file.Fd()), 0, 1, 0, &overlapped)
	closeErr := l.file.Close()
	l.file = nil

	if unlockErr != nil && !errors.Is(unlockErr, windows.ERROR_NOT_LOCKED) {
		return fmt.Errorf("release desktop instance lock: %w", unlockErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close desktop instance lock: %w", closeErr)
	}
	return nil
}
