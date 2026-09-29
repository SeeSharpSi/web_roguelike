package game

// MoveOption reports whether movement is possible and identifies doors on the route.
type MoveOption struct {
	Possible          bool
	ThroughDoor       bool
	ThroughHiddenDoor bool
}

// PossibleMoves describes movement options in each direction from a room.
type PossibleMoves struct {
	North MoveOption
	South MoveOption
	East  MoveOption
	West  MoveOption
}

// GetPossibleMoves returns movement options from pos.
func (m *Map) GetPossibleMoves(pos Pos) PossibleMoves {
	if m == nil {
		return PossibleMoves{}
	}

	room, exists := m.Rooms[pos]
	if !exists || room == nil {
		return PossibleMoves{}
	}

	return PossibleMoves{
		North: moveOption(room.NWall),
		South: moveOption(room.SWall),
		East:  moveOption(room.EWall),
		West:  moveOption(room.WWall),
	}
}

func moveOption(wall Wall) MoveOption {
	switch wall.Type {
	case WallEmpty:
		return MoveOption{Possible: true}
	case WallDoor:
		return MoveOption{Possible: true, ThroughDoor: true}
	case WallHiddenDoor:
		return MoveOption{ThroughHiddenDoor: true}
	default:
		return MoveOption{}
	}
}
