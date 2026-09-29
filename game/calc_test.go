package game

import "testing"

func TestGetPossibleMovesWallTypes(t *testing.T) {
	tests := []struct {
		name     string
		wallType WallType
		want     MoveOption
	}{
		{name: "empty", wallType: WallEmpty, want: MoveOption{Possible: true, ThroughDoor: false, ThroughHiddenDoor: false}},
		{name: "door", wallType: WallDoor, want: MoveOption{Possible: true, ThroughDoor: true, ThroughHiddenDoor: false}},
		{name: "hidden door", wallType: WallHiddenDoor, want: MoveOption{Possible: false, ThroughDoor: false, ThroughHiddenDoor: true}},
		{name: "indestructible", wallType: WallIndestructible, want: MoveOption{Possible: false, ThroughDoor: false, ThroughHiddenDoor: false}},
		{name: "destructible", wallType: WallDestructible, want: MoveOption{Possible: false, ThroughDoor: false, ThroughHiddenDoor: false}},
		{name: "unknown", wallType: WallType("unknown"), want: MoveOption{Possible: false, ThroughDoor: false, ThroughHiddenDoor: false}},
		{name: "zero", wallType: "", want: MoveOption{Possible: false, ThroughDoor: false, ThroughHiddenDoor: false}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := Map{Rooms: map[Pos]*Room{{}: {NWall: Wall{Type: tt.wallType}}}}
			moves := m.GetPossibleMoves(Pos{})
			if moves.North != tt.want {
				t.Fatalf("North = %+v, want %+v", moves.North, tt.want)
			}
		})
	}
}

func TestGetPossibleMovesDirectionMapping(t *testing.T) {
	m := Map{Rooms: map[Pos]*Room{
		{X: 2, Y: 3}: {
			NWall: Wall{Type: WallDoor},
			SWall: Wall{Type: WallIndestructible},
			EWall: Wall{Type: WallEmpty},
			WWall: Wall{Type: WallHiddenDoor},
		},
	}}

	got := m.GetPossibleMoves(Pos{X: 2, Y: 3})
	want := PossibleMoves{
		North: MoveOption{Possible: true, ThroughDoor: true},
		East:  MoveOption{Possible: true},
		South: MoveOption{},
		West:  MoveOption{Possible: false, ThroughDoor: false, ThroughHiddenDoor: true},
	}
	if got != want {
		t.Fatalf("GetPossibleMoves() = %+v, want %+v", got, want)
	}
}

func TestGetPossibleMovesMissingRoom(t *testing.T) {
	tests := []struct {
		name string
		m    *Map
		pos  Pos
	}{
		{name: "missing position", m: &Map{Rooms: map[Pos]*Room{}}, pos: Pos{X: 1}},
		{name: "nil room", m: &Map{Rooms: map[Pos]*Room{{}: nil}}, pos: Pos{}},
		{name: "nil map", pos: Pos{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.m.GetPossibleMoves(tt.pos); got != (PossibleMoves{}) {
				t.Fatalf("GetPossibleMoves() = %+v, want zero value", got)
			}
		})
	}
}
