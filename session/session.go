package session

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"seesharpsi/web_roguelike/game"
	"sync"
	"time"
)

// Session holds the state for a single user's game.
type Session struct {
	ID           string
	LastAccessed time.Time
	Player       game.Player
	Map          game.Map
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
func (m *Manager) CreateSession() string {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Generate a random, secure session ID.
	b := make([]byte, 16)
	rand.Read(b)
	id := hex.EncodeToString(b)

	new_player := game.Player{}
	new_player.Generate_player()
	new_map := game.Map{}
	new_map.Generate_map()
	new_player.Position = new_map.StartPos

	m.sessions[id] = &Session{
		ID: id,
		//GameState:    &story.GameState{},
		//StoryHistory: []story.StoryPage{},
		LastAccessed: time.Now(),
		Player:       new_player,
		Map:          new_map,
	}
	return id
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
	return session
}

// GetOrCreateSession retrieves an existing session or creates a new one.
func (m *Manager) GetOrCreateSession(r *http.Request) (*Session, http.Cookie) {
	cookie, err := r.Cookie("session_id")
	if err == nil {
		session := m.GetSession(cookie.Value)
		if session != nil {
			return session, *cookie
		}
	}

	// If no valid session is found, create a new one.
	id := m.CreateSession()
	newCookie := http.Cookie{
		Name:     "session_id",
		Value:    id,
		Expires:  time.Now().Add(24 * time.Hour),
		HttpOnly: true,
		Path:     "/",
	}
	return m.GetSession(id), newCookie
}
