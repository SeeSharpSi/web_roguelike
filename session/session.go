package session

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"net/http"
	"sync"
	"time"
)

// Session identifies a browser session without storing gameplay state.
type Session struct {
	ID           string
	LastAccessed time.Time
}

// Manager handles the creation, storage, and retrieval of sessions.
type Manager struct {
	sessions map[string]*Session
	mutex    sync.Mutex
}

// NewManager creates a new session manager.
func NewManager() *Manager {
	return &Manager{
		sessions: make(map[string]*Session),
	}
}

// CreateSession creates a new session and returns its ID.
func (m *Manager) CreateSession() (string, error) {
	session, err := m.createSession()
	if err != nil {
		return "", err
	}
	return session.ID, nil
}

func (m *Manager) createSession() (*Session, error) {
	for {
		idBytes := make([]byte, 16)
		if _, err := io.ReadFull(rand.Reader, idBytes); err != nil {
			return nil, err
		}
		id := hex.EncodeToString(idBytes)
		now := time.Now()

		m.mutex.Lock()
		if _, exists := m.sessions[id]; exists {
			m.mutex.Unlock()
			continue
		}
		stored := &Session{ID: id, LastAccessed: now}
		m.sessions[id] = stored
		copy := *stored
		m.mutex.Unlock()
		return &copy, nil
	}
}

// GetSession retrieves a session by its ID.
func (m *Manager) GetSession(id string) *Session {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	session, ok := m.sessions[id]
	if !ok {
		return nil
	}
	session.LastAccessed = time.Now()
	copy := *session
	return &copy
}

// GetOrCreateSession retrieves an existing session or creates a new one.
func (m *Manager) GetOrCreateSession(r *http.Request) (*Session, http.Cookie, error) {
	secure := r != nil && r.TLS != nil
	if r != nil {
		cookie, err := r.Cookie("session_id")
		if err == nil {
			session := m.GetSession(cookie.Value)
			if session != nil {
				return session, sessionCookie(session.ID, time.Now(), secure), nil
			}
		}
	}

	session, err := m.createSession()
	if err != nil {
		return nil, http.Cookie{}, err
	}
	return session, sessionCookie(session.ID, time.Now(), secure), nil
}

func sessionCookie(id string, now time.Time, secure bool) http.Cookie {
	return http.Cookie{
		Name:     "session_id",
		Value:    id,
		Path:     "/",
		Expires:  now.Add(24 * time.Hour),
		MaxAge:   24 * 60 * 60,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	}
}

// Cleanup removes sessions that have been idle for at least maxIdle.
func (m *Manager) Cleanup(now time.Time, maxIdle time.Duration) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	for id, session := range m.sessions {
		if now.Sub(session.LastAccessed) >= maxIdle {
			delete(m.sessions, id)
		}
	}
}
