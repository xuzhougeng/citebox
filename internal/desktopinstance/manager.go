package desktopinstance

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	instanceLockFilename = "desktop-instance.lock"
	instanceInfoFilename = "desktop-instance.json"
	activationPath       = "/activate"
	tokenHeader          = "X-CiteBox-Instance-Token"
	signalTimeout        = 3 * time.Second
	signalInterval       = 100 * time.Millisecond
)

var ErrAlreadyRunning = errors.New("desktop instance already running")

type AlreadyRunningError struct {
	SignalErr error
}

func (e *AlreadyRunningError) Error() string {
	if e == nil || e.SignalErr == nil {
		return ErrAlreadyRunning.Error()
	}
	return fmt.Sprintf("%s: %v", ErrAlreadyRunning.Error(), e.SignalErr)
}

func (e *AlreadyRunningError) Unwrap() error {
	return ErrAlreadyRunning
}

type Manager struct {
	lock     *fileLock
	infoPath string
	token    string
	listener net.Listener
	server   *http.Server

	mu                sync.Mutex
	activate          func()
	pendingActivation bool
	closeOnce         sync.Once
}

type instanceInfo struct {
	Addr  string `json:"addr"`
	Token string `json:"token"`
}

func Acquire(appName string) (*Manager, error) {
	baseDir, err := instanceDir(appName)
	if err != nil {
		return nil, err
	}

	lock, err := openFileLock(filepath.Join(baseDir, instanceLockFilename))
	if err != nil {
		return nil, err
	}

	locked, err := lock.TryLock()
	if err != nil {
		_ = lock.Close()
		return nil, err
	}
	if !locked {
		signalErr := signalExistingInstance(filepath.Join(baseDir, instanceInfoFilename))
		_ = lock.Close()
		return nil, &AlreadyRunningError{SignalErr: signalErr}
	}

	manager := &Manager{
		lock:     lock,
		infoPath: filepath.Join(baseDir, instanceInfoFilename),
	}
	if err := manager.start(); err != nil {
		_ = manager.Close()
		return nil, err
	}

	return manager, nil
}

func (m *Manager) SetActivateHandler(fn func()) {
	if fn == nil {
		return
	}

	m.mu.Lock()
	m.activate = fn
	pending := m.pendingActivation
	m.pendingActivation = false
	m.mu.Unlock()

	if pending {
		fn()
	}
}

func (m *Manager) Close() error {
	var closeErr error
	m.closeOnce.Do(func() {
		if m.server != nil {
			if err := m.server.Close(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				closeErr = err
			}
		}
		if m.listener != nil {
			if err := m.listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) && closeErr == nil {
				closeErr = err
			}
		}

		if err := os.Remove(m.infoPath); err != nil && !errors.Is(err, os.ErrNotExist) && closeErr == nil {
			closeErr = err
		}

		if m.lock != nil && closeErr == nil {
			closeErr = m.lock.Close()
		} else if m.lock != nil {
			_ = m.lock.Close()
		}
	})
	return closeErr
}

func (m *Manager) start() error {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("listen for desktop activation: %w", err)
	}

	token, err := randomToken()
	if err != nil {
		_ = listener.Close()
		return fmt.Errorf("generate desktop activation token: %w", err)
	}

	server := &http.Server{
		Handler:           http.HandlerFunc(m.handleActivate),
		ReadHeaderTimeout: time.Second,
	}

	m.listener = listener
	m.server = server
	m.token = token

	if err := writeInstanceInfo(m.infoPath, instanceInfo{
		Addr:  listener.Addr().String(),
		Token: token,
	}); err != nil {
		_ = server.Close()
		return err
	}

	go func() {
		_ = server.Serve(listener)
	}()

	return nil
}

func (m *Manager) handleActivate(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != activationPath {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if subtle.ConstantTimeCompare([]byte(r.Header.Get(tokenHeader)), []byte(m.token)) != 1 {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	_, _ = io.Copy(io.Discard, r.Body)
	_ = r.Body.Close()

	m.requestActivation()
	w.WriteHeader(http.StatusNoContent)
}

func (m *Manager) requestActivation() {
	m.mu.Lock()
	handler := m.activate
	if handler == nil {
		m.pendingActivation = true
		m.mu.Unlock()
		return
	}
	m.pendingActivation = false
	m.mu.Unlock()

	handler()
}

func instanceDir(appName string) (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve desktop config dir: %w", err)
	}

	baseDir := filepath.Join(configDir, appName)
	if err := os.MkdirAll(baseDir, 0o700); err != nil {
		return "", fmt.Errorf("create desktop config dir: %w", err)
	}
	return baseDir, nil
}

func writeInstanceInfo(path string, info instanceInfo) error {
	data, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("marshal desktop instance info: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write desktop instance info: %w", err)
	}
	return nil
}

func readInstanceInfo(path string) (instanceInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return instanceInfo{}, err
	}

	var info instanceInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return instanceInfo{}, err
	}
	if info.Addr == "" || info.Token == "" {
		return instanceInfo{}, errors.New("incomplete desktop instance info")
	}

	return info, nil
}

func signalExistingInstance(infoPath string) error {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	deadline := time.Now().Add(signalTimeout)
	var lastErr error

	for {
		info, err := readInstanceInfo(infoPath)
		if err == nil {
			lastErr = signalActivate(client, info)
			if lastErr == nil {
				return nil
			}
		} else {
			lastErr = err
		}

		if time.Now().After(deadline) {
			break
		}
		time.Sleep(signalInterval)
	}

	return lastErr
}

func signalActivate(client *http.Client, info instanceInfo) error {
	request, err := http.NewRequest(http.MethodPost, "http://"+info.Addr+activationPath, nil)
	if err != nil {
		return err
	}
	request.Header.Set(tokenHeader, info.Token)

	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)

	if response.StatusCode != http.StatusNoContent {
		return fmt.Errorf("unexpected activation response status: %s", response.Status)
	}
	return nil
}

func randomToken() (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}
