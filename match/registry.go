package match

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"seesharpsi/web_roguelike/game"
)

type Status string

const (
	Lobby    Status = "lobby"
	Active   Status = "active"
	Finished Status = "finished"
	Closed   Status = "closed"

	MaxPlayers = 8
)

var (
	ErrRoomNotFound    = errors.New("room not found")
	ErrNotMember       = errors.New("session is not an active room member")
	ErrInvalidUsername = errors.New("username must contain 1 to 32 non-control characters")
	ErrRoomFull        = errors.New("room is full")
	ErrGameStarted     = errors.New("game has already started; new players cannot join")
	ErrNotHost         = errors.New("only the host can perform this action")
	ErrNotActive       = errors.New("game is not active")
	ErrInvalidMove     = errors.New("invalid or blocked move")
	ErrClosed          = errors.New("room is closed")
)

type ParticipantView struct {
	ID       string
	Username string
	Player   game.Player
}

type Snapshot struct {
	Code         string
	Status       Status
	HostPlayerID string
	Players      []ParticipantView
	Viewer       ParticipantView
	World        game.Map
	Revision     uint64
	Moves        game.PossibleMoves
}

type Registry struct {
	mutex sync.Mutex
	rooms map[string]*match
}

type match struct {
	mutex         sync.Mutex
	code          string
	status        Status
	hostPlayerID  string
	players       map[string]*participant
	usernameIndex map[string]string
	world         game.Map
	revision      uint64
	lastActivity  time.Time
}

type participant struct {
	id              string
	username        string
	usernameKey     string
	player          game.Player
	activeSessionID string
}

const codeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// NewRegistry creates an empty in-memory match registry.
func NewRegistry() *Registry {
	return &Registry{rooms: make(map[string]*match)}
}

// NormalizeCode trims surrounding whitespace and uppercases a room code.
func NormalizeCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

func normalizeUsername(username string) (key, display string, err error) {
	display = strings.TrimSpace(username)
	runeCount := 0
	for _, r := range display {
		runeCount++
		if unicode.IsControl(r) {
			return "", "", ErrInvalidUsername
		}
	}
	if runeCount < 1 || runeCount > 32 {
		return "", "", ErrInvalidUsername
	}
	return strings.ToLower(display), display, nil
}

func newPlayerID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return "p_" + hex.EncodeToString(bytes), nil
}

func newRoomCode() (string, error) {
	var code [6]byte
	alphabetSize := big.NewInt(int64(len(codeAlphabet)))
	for i := range code {
		index, err := rand.Int(rand.Reader, alphabetSize)
		if err != nil {
			return "", err
		}
		code[i] = codeAlphabet[index.Int64()]
	}
	return string(code[:]), nil
}

func (r *Registry) room(code string) (*match, error) {
	code = NormalizeCode(code)
	r.mutex.Lock()
	room := r.rooms[code]
	r.mutex.Unlock()
	if room == nil {
		return nil, ErrRoomNotFound
	}
	return room, nil
}

func (r *Registry) removeIfSame(code string, room *match) {
	r.mutex.Lock()
	if r.rooms[code] == room {
		delete(r.rooms, code)
	}
	r.mutex.Unlock()
}

// Create creates a lobby and adds its creator as host.
func (r *Registry) Create(sessionID, username string) (Snapshot, error) {
	if sessionID == "" {
		return Snapshot{}, ErrNotMember
	}
	usernameKey, displayName, err := normalizeUsername(username)
	if err != nil {
		return Snapshot{}, err
	}

	world := game.Map{}
	world.Generate_map()
	player := game.Player{}
	player.Generate_player()
	player.Position = world.StartPos
	playerID, err := newPlayerID()
	if err != nil {
		return Snapshot{}, fmt.Errorf("generate player ID: %w", err)
	}
	now := time.Now()
	member := &participant{
		id:              playerID,
		username:        displayName,
		usernameKey:     usernameKey,
		player:          player,
		activeSessionID: sessionID,
	}

	for {
		code, err := newRoomCode()
		if err != nil {
			return Snapshot{}, fmt.Errorf("generate room code: %w", err)
		}
		room := &match{
			code:          code,
			status:        Lobby,
			hostPlayerID:  playerID,
			players:       map[string]*participant{playerID: member},
			usernameIndex: map[string]string{usernameKey: playerID},
			world:         world,
			revision:      1,
			lastActivity:  now,
		}

		r.mutex.Lock()
		if _, collision := r.rooms[code]; collision {
			r.mutex.Unlock()
			continue
		}
		r.rooms[code] = room
		r.mutex.Unlock()

		room.mutex.Lock()
		snapshot := room.snapshotLocked(member)
		room.mutex.Unlock()
		return snapshot, nil
	}
}

func (room *match) participantForSessionLocked(sessionID string) *participant {
	if sessionID == "" {
		return nil
	}
	for _, member := range room.players {
		if member.activeSessionID == sessionID {
			return member
		}
	}
	return nil
}

func (room *match) snapshotLocked(viewer *participant) Snapshot {
	players := make([]ParticipantView, 0, len(room.players))
	ordered := make([]*participant, 0, len(room.players))
	for _, member := range room.players {
		ordered = append(ordered, member)
	}
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].usernameKey < ordered[j].usernameKey
	})
	for _, member := range ordered {
		players = append(players, ParticipantView{
			ID:       member.id,
			Username: member.username,
			Player:   member.player,
		})
	}
	return Snapshot{
		Code:         room.code,
		Status:       room.status,
		HostPlayerID: room.hostPlayerID,
		Players:      players,
		Viewer: ParticipantView{
			ID:       viewer.id,
			Username: viewer.username,
			Player:   viewer.player,
		},
		World:    cloneMap(room.world),
		Revision: room.revision,
		Moves:    room.world.GetPossibleMoves(viewer.player.Position),
	}
}

func cloneMap(source game.Map) game.Map {
	clones := make(map[*game.Room]*game.Room)
	cloneRoom := func(source *game.Room) *game.Room {
		if source == nil {
			return nil
		}
		if existing, ok := clones[source]; ok {
			return existing
		}
		copied := *source
		if source.Items != nil {
			copied.Items = make([]game.Item, len(source.Items))
			copy(copied.Items, source.Items)
		}
		clones[source] = &copied
		return &copied
	}

	cloned := game.Map{
		Width:    source.Width,
		Length:   source.Length,
		StartPos: source.StartPos,
	}
	if source.Rooms != nil {
		cloned.Rooms = make(map[game.Pos]*game.Room, len(source.Rooms))
		for pos, room := range source.Rooms {
			cloned.Rooms[pos] = cloneRoom(room)
		}
	}
	if source.Explored != nil {
		cloned.Explored = make(map[game.Pos]*game.Room, len(source.Explored))
		for pos, room := range source.Explored {
			cloned.Explored[pos] = cloneRoom(room)
		}
	}
	return cloned
}

// Cleanup closes and removes matches idle for at least maxIdle.
func (r *Registry) Cleanup(now time.Time, maxIdle time.Duration) {
	type entry struct {
		code string
		room *match
	}
	r.mutex.Lock()
	entries := make([]entry, 0, len(r.rooms))
	for code, room := range r.rooms {
		entries = append(entries, entry{code: code, room: room})
	}
	r.mutex.Unlock()

	for _, entry := range entries {
		entry.room.mutex.Lock()
		idle := entry.room.status != Closed && now.Sub(entry.room.lastActivity) >= maxIdle
		if idle {
			entry.room.status = Closed
			entry.room.revision++
		}
		entry.room.mutex.Unlock()
		if idle {
			r.removeIfSame(entry.code, entry.room)
		}
	}
}
