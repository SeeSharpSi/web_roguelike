package match

import (
	"fmt"
	"strings"
	"time"

	"seesharpsi/web_roguelike/game"
)

// Join adds a lobby participant or reconnects an existing username.
func (r *Registry) Join(code, sessionID, username string) (Snapshot, error) {
	if sessionID == "" {
		return Snapshot{}, ErrNotMember
	}
	usernameKey, displayName, err := normalizeUsername(username)
	if err != nil {
		return Snapshot{}, err
	}
	room, err := r.room(code)
	if err != nil {
		return Snapshot{}, err
	}
	room.mutex.Lock()
	defer room.mutex.Unlock()
	if room.status == Closed {
		return Snapshot{}, ErrClosed
	}

	if playerID, exists := room.usernameIndex[usernameKey]; exists {
		member := room.players[playerID]
		previous := room.participantForSessionLocked(sessionID)
		changed := false
		if previous != nil && previous.id != member.id {
			previous.activeSessionID = ""
			changed = true
		}
		if member.activeSessionID != sessionID {
			member.activeSessionID = sessionID
			changed = true
		}
		if changed {
			room.revision++
		}
		room.lastActivity = time.Now()
		return room.snapshotLocked(member), nil
	}
	if room.status != Lobby {
		return Snapshot{}, ErrGameStarted
	}
	if len(room.players) >= MaxPlayers {
		return Snapshot{}, ErrRoomFull
	}

	playerID, err := newPlayerID()
	if err != nil {
		return Snapshot{}, fmt.Errorf("generate player ID: %w", err)
	}
	player := game.Player{}
	player.Generate_player()
	player.Position = room.world.StartPos
	member := &participant{
		id:              playerID,
		username:        displayName,
		usernameKey:     usernameKey,
		player:          player,
		activeSessionID: sessionID,
	}
	if previous := room.participantForSessionLocked(sessionID); previous != nil {
		previous.activeSessionID = ""
	}
	room.players[playerID] = member
	room.usernameIndex[usernameKey] = playerID
	room.revision++
	room.lastActivity = time.Now()
	return room.snapshotLocked(member), nil
}

// Snapshot returns an authorized, deeply independent view of a match.
func (r *Registry) Snapshot(code, sessionID string) (Snapshot, error) {
	room, err := r.room(code)
	if err != nil {
		return Snapshot{}, err
	}
	room.mutex.Lock()
	defer room.mutex.Unlock()
	if room.status == Closed {
		return Snapshot{}, ErrClosed
	}
	viewer := room.participantForSessionLocked(sessionID)
	if viewer == nil {
		return Snapshot{}, ErrNotMember
	}
	room.lastActivity = time.Now()
	return room.snapshotLocked(viewer), nil
}

// Start transitions a host-owned lobby to active play.
func (r *Registry) Start(code, sessionID string) (Snapshot, error) {
	room, err := r.room(code)
	if err != nil {
		return Snapshot{}, err
	}
	room.mutex.Lock()
	defer room.mutex.Unlock()
	if room.status == Closed {
		return Snapshot{}, ErrClosed
	}
	viewer := room.participantForSessionLocked(sessionID)
	if viewer == nil {
		return Snapshot{}, ErrNotMember
	}
	if viewer.id != room.hostPlayerID {
		return Snapshot{}, ErrNotHost
	}
	if room.status == Active {
		room.lastActivity = time.Now()
		return room.snapshotLocked(viewer), nil
	}
	if room.status != Lobby {
		return Snapshot{}, ErrGameStarted
	}
	room.status = Active
	room.revision++
	room.lastActivity = time.Now()
	return room.snapshotLocked(viewer), nil
}

// Move moves the session-bound participant through a traversable adjacent wall.
func (r *Registry) Move(code, sessionID, direction string) (Snapshot, error) {
	room, err := r.room(code)
	if err != nil {
		return Snapshot{}, err
	}
	room.mutex.Lock()
	defer room.mutex.Unlock()
	if room.status == Closed {
		return Snapshot{}, ErrClosed
	}
	viewer := room.participantForSessionLocked(sessionID)
	if viewer == nil {
		return Snapshot{}, ErrNotMember
	}
	if room.status != Active {
		return Snapshot{}, ErrNotActive
	}

	direction = strings.ToLower(strings.TrimSpace(direction))
	position := viewer.player.Position
	currentRoom := room.world.Rooms[position]
	if currentRoom == nil {
		return Snapshot{}, ErrInvalidMove
	}
	possibleMoves := room.world.GetPossibleMoves(position)
	var option game.MoveOption
	target := position
	switch direction {
	case "north":
		option = possibleMoves.North
		target.Y++
	case "south":
		option = possibleMoves.South
		target.Y--
	case "east":
		option = possibleMoves.East
		target.X++
	case "west":
		option = possibleMoves.West
		target.X--
	default:
		return Snapshot{}, ErrInvalidMove
	}
	if !option.Possible || room.world.Rooms[target] == nil {
		return Snapshot{}, ErrInvalidMove
	}

	viewer.player.Position = target
	room.world.Explored[target] = room.world.Rooms[target]
	room.revision++
	room.lastActivity = time.Now()
	return room.snapshotLocked(viewer), nil
}

// Finish marks active play finished. Only the host may finish a match.
func (r *Registry) Finish(code, sessionID string) (Snapshot, error) {
	room, err := r.room(code)
	if err != nil {
		return Snapshot{}, err
	}
	room.mutex.Lock()
	defer room.mutex.Unlock()
	if room.status == Closed {
		return Snapshot{}, ErrClosed
	}
	viewer := room.participantForSessionLocked(sessionID)
	if viewer == nil {
		return Snapshot{}, ErrNotMember
	}
	if viewer.id != room.hostPlayerID {
		return Snapshot{}, ErrNotHost
	}
	if room.status != Active {
		return Snapshot{}, ErrNotActive
	}
	room.status = Finished
	room.revision++
	room.lastActivity = time.Now()
	return room.snapshotLocked(viewer), nil
}

// Leave explicitly abandons the participant bound to sessionID.
func (r *Registry) Leave(code, sessionID string) error {
	room, err := r.room(code)
	if err != nil {
		return err
	}
	room.mutex.Lock()
	if room.status == Closed {
		room.mutex.Unlock()
		return ErrClosed
	}
	member := room.participantForSessionLocked(sessionID)
	if member == nil {
		room.mutex.Unlock()
		return ErrNotMember
	}
	delete(room.players, member.id)
	delete(room.usernameIndex, member.usernameKey)
	room.revision++
	room.lastActivity = time.Now()

	closed := false
	if member.id == room.hostPlayerID {
		if len(room.players) == 0 {
			room.status = Closed
			room.hostPlayerID = ""
			closed = true
		} else {
			room.hostPlayerID = room.firstPlayerIDLocked()
		}
	} else if len(room.players) == 0 {
		room.status = Closed
		room.hostPlayerID = ""
		closed = true
	}
	room.mutex.Unlock()
	if closed {
		r.removeIfSame(room.code, room)
	}
	return nil
}

func (room *match) firstPlayerIDLocked() string {
	var first *participant
	for _, member := range room.players {
		if first == nil || member.usernameKey < first.usernameKey {
			first = member
		}
	}
	if first == nil {
		return ""
	}
	return first.id
}
