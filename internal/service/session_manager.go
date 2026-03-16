package service

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"
)

const SessionCookieName = "citebox_session"

type Session struct {
	ID        string
	Username  string
	ExpiresAt time.Time
}

type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]Session
	ttl      time.Duration
}

func NewSessionManager(ttl time.Duration) *SessionManager {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}

	return &SessionManager{
		sessions: make(map[string]Session),
		ttl:      ttl,
	}
}

func (m *SessionManager) TTL() time.Duration {
	return m.ttl
}

func (m *SessionManager) Create(username string) (Session, error) {
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		return Session{}, err
	}

	session := Session{
		ID:        base64.RawURLEncoding.EncodeToString(token),
		Username:  username,
		ExpiresAt: time.Now().Add(m.ttl),
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.deleteExpiredLocked(time.Now())
	m.sessions[session.ID] = session
	return session, nil
}

func (m *SessionManager) Validate(sessionID string) (Session, bool) {
	if sessionID == "" {
		return Session{}, false
	}

	m.mu.RLock()
	session, ok := m.sessions[sessionID]
	m.mu.RUnlock()
	if !ok {
		return Session{}, false
	}

	if time.Now().After(session.ExpiresAt) {
		m.Delete(sessionID)
		return Session{}, false
	}

	return session, true
}

func (m *SessionManager) Delete(sessionID string) {
	if sessionID == "" {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, sessionID)
}

func (m *SessionManager) DeleteAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions = make(map[string]Session)
}

func (m *SessionManager) deleteExpiredLocked(now time.Time) {
	for sessionID, session := range m.sessions {
		if now.After(session.ExpiresAt) {
			delete(m.sessions, sessionID)
		}
	}
}
