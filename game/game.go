package game

import "math/rand/v2"

// A struct that contains player stats
type Player struct {
	Alive    bool
	Health   int
	Strength float64 // Stength is a multiplier
	Stamina  int
	Position Pos
}

type Map struct {
	Width    int
	Length   int
	Rooms    map[Pos]*Room
	StartPos Pos
	Explored map[Pos]*Room
}

type Pos struct {
	X int
	Y int
}

type Room struct {
	NWall Wall
	SWall Wall
	EWall Wall
	WWall Wall
	Items []Item
}

type WallType = string

const (
	WallEmpty          WallType = "empty"
	WallDoor           WallType = "door"
	WallHiddenDoor     WallType = "hidden_door"
	WallIndestructible WallType = "indestructible"
	WallDestructible   WallType = "destructible"
)

type Wall struct {
	Type   WallType
	Health int
}

type Item struct {
}

func (p *Player) Generate_player() {
	p.Alive = true
	p.Health = 83 + rand.N(18)
	p.Strength = float64(80+rand.N(21)) / 100
	p.Stamina = 50 + rand.N(51)
}

func generationRand(rngs ...*rand.Rand) *rand.Rand {
	if len(rngs) > 0 && rngs[0] != nil {
		return rngs[0]
	}
	return rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
}

func randomWall(rng *rand.Rand) Wall {
	types := []WallType{WallEmpty, WallDoor, WallHiddenDoor, WallIndestructible, WallDestructible}
	return newWall(types[rng.IntN(len(types))], rng)
}

func newWall(wallType WallType, rng *rand.Rand) Wall {
	wall := Wall{Type: wallType}
	if wallType == WallDestructible {
		wall.Health = 50 + rng.IntN(51)
	}
	return wall
}

func (r *Room) Generate_room(rngs ...*rand.Rand) {
	rng := generationRand(rngs...)
	r.NWall = randomWall(rng)
	r.SWall = randomWall(rng)
	r.EWall = randomWall(rng)
	r.WWall = randomWall(rng)
}

func treeEdge(parent map[Pos]Pos, first Pos, second Pos) bool {
	parentOfFirst, firstHasParent := parent[first]
	if firstHasParent && parentOfFirst == second {
		return true
	}
	parentOfSecond, secondHasParent := parent[second]
	return secondHasParent && parentOfSecond == first
}

// Generate_map creates every room, then assigns each shared edge once.
func (m *Map) Generate_map(rngs ...*rand.Rand) {
	rng := generationRand(rngs...)
	m.Width = 0
	m.Length = 0
	m.Rooms = nil
	m.Explored = nil
	m.StartPos = Pos{}
	m.Width = 7 + rng.IntN(4)
	m.Length = 7 + rng.IntN(4)
	m.Rooms = make(map[Pos]*Room)
	m.Explored = make(map[Pos]*Room)

	for y := 0; y < m.Length; y++ {
		for x := 0; x < m.Width; x++ {
			m.Rooms[Pos{X: x, Y: y}] = &Room{}
		}
	}

	m.StartPos = Pos{X: rng.IntN(m.Width), Y: rng.IntN(m.Length)}
	parent := make(map[Pos]Pos)
	visited := map[Pos]bool{m.StartPos: true}
	stack := []Pos{m.StartPos}
	for len(stack) > 0 {
		current := stack[len(stack)-1]
		neighbors := []Pos{
			{X: current.X, Y: current.Y + 1},
			{X: current.X + 1, Y: current.Y},
			{X: current.X, Y: current.Y - 1},
			{X: current.X - 1, Y: current.Y},
		}
		for i := len(neighbors) - 1; i > 0; i-- {
			j := rng.IntN(i + 1)
			neighbors[i], neighbors[j] = neighbors[j], neighbors[i]
		}
		advanced := false
		for _, next := range neighbors {
			if next.X < 0 || next.X >= m.Width || next.Y < 0 || next.Y >= m.Length || visited[next] {
				continue
			}
			visited[next] = true
			parent[next] = current
			stack = append(stack, next)
			advanced = true
			break
		}
		if !advanced {
			stack = stack[:len(stack)-1]
		}
	}

	passages := []WallType{WallEmpty, WallDoor, WallDestructible}
	wallTypes := []WallType{WallEmpty, WallDoor, WallHiddenDoor, WallIndestructible, WallDestructible}
	for y := 0; y < m.Length; y++ {
		for x := 0; x < m.Width; x++ {
			pos := Pos{X: x, Y: y}
			room := m.Rooms[pos]
			if y == m.Length-1 {
				room.NWall = newWall(WallIndestructible, rng)
			}
			if x == m.Width-1 {
				room.EWall = newWall(WallIndestructible, rng)
			}
			if x+1 < m.Width {
				next := Pos{X: x + 1, Y: y}
				var wallType WallType
				if treeEdge(parent, pos, next) {
					wallType = passages[rng.IntN(len(passages))]
				} else {
					wallType = wallTypes[rng.IntN(len(wallTypes))]
				}
				wall := newWall(wallType, rng)
				room.EWall = wall
				m.Rooms[next].WWall = wall
			}
			if y+1 < m.Length {
				next := Pos{X: x, Y: y + 1}
				var wallType WallType
				if treeEdge(parent, pos, next) {
					wallType = passages[rng.IntN(len(passages))]
				} else {
					wallType = wallTypes[rng.IntN(len(wallTypes))]
				}
				wall := newWall(wallType, rng)
				room.NWall = wall
				m.Rooms[next].SWall = wall
			}
			if y == 0 {
				room.SWall = newWall(WallIndestructible, rng)
			}
			if x == 0 {
				room.WWall = newWall(WallIndestructible, rng)
			}
		}
	}
	m.Explored[m.StartPos] = m.Rooms[m.StartPos]
}
