package game

import (
	"math/rand/v2"
	"testing"
)

type mapSeedPair struct {
	state1 uint64
	state2 uint64
}

var wallQuotaSeedPairs = []mapSeedPair{
	{1, 2},
	{3, 5},
	{8, 13},
	{21, 34},
	{55, 89},
	{144, 233},
	{377, 610},
	{987, 1597},
	{2584, 4181},
	{6765, 10946},
	{17711, 28657},
	{46368, 75025},
	{121393, 196418},
	{317811, 514229},
	{832040, 1346269},
	{2178309, 3524578},
	{5702887, 9227465},
	{14930352, 24157817},
	{39088169, 63245986},
	{102334155, 165580141},
	{267914296, 433494437},
	{701408733, 1134903170},
	{1836311903, 2971215073},
	{4807526976, 7778742049},
	{12586269025, 20365011074},
	{32951280099, 53316291173},
	{86267571272, 139583862445},
	{225851433717, 365435296162},
	{591286729879, 956722026041},
	{1548008755920, 2504730781961},
	{4052739537881, 6557470319842},
	{10610209857723, 17167680177565},
}

func TestGenerateMapWallQuotasAndConnectivity(t *testing.T) {
	doorSeen := false
	hiddenDoorSeen := false
	destructibleSeen := false

	for _, seed := range wallQuotaSeedPairs {
		m := Map{}
		m.Generate_map(rand.New(rand.NewPCG(seed.state1, seed.state2)))

		doors, hiddenDoors, destructibles := 0, 0, 0
		specialIndices := make([]int, 0, 9)
		interiorEdgeCount := (m.Width-1)*m.Length + m.Width*(m.Length-1)
		cooldown := interiorEdgeCount / 9
		if cooldown < 1 {
			cooldown = 1
		}
		edgeIndex := 0
		for y := 0; y < m.Length; y++ {
			for x := 0; x < m.Width; x++ {
				pos := Pos{X: x, Y: y}
				room := m.Rooms[pos]
				if y == m.Length-1 && room.NWall.Type != WallIndestructible {
					t.Fatalf("seed (%d, %d): north perimeter at %+v is %q", seed.state1, seed.state2, pos, room.NWall.Type)
				}
				if y == 0 && room.SWall.Type != WallIndestructible {
					t.Fatalf("seed (%d, %d): south perimeter at %+v is %q", seed.state1, seed.state2, pos, room.SWall.Type)
				}
				if x == m.Width-1 && room.EWall.Type != WallIndestructible {
					t.Fatalf("seed (%d, %d): east perimeter at %+v is %q", seed.state1, seed.state2, pos, room.EWall.Type)
				}
				if x == 0 && room.WWall.Type != WallIndestructible {
					t.Fatalf("seed (%d, %d): west perimeter at %+v is %q", seed.state1, seed.state2, pos, room.WWall.Type)
				}

				if x+1 < m.Width {
					next := Pos{X: x + 1, Y: y}
					if room.EWall != m.Rooms[next].WWall {
						t.Fatalf("seed (%d, %d): east/west walls disagree between %+v and %+v", seed.state1, seed.state2, pos, next)
					}
					switch room.EWall.Type {
					case WallDoor:
						doors++
						doorSeen = true
						specialIndices = append(specialIndices, edgeIndex)
						if !m.wallIsSupported(mapEdge{x: x + 1, y: y}) {
							t.Fatalf("seed (%d, %d): unsupported %s at east edge (%d, %d)", seed.state1, seed.state2, room.EWall.Type, x+1, y)
						}
					case WallHiddenDoor:
						hiddenDoors++
						hiddenDoorSeen = true
						specialIndices = append(specialIndices, edgeIndex)
						if !m.wallIsSupported(mapEdge{x: x + 1, y: y}) {
							t.Fatalf("seed (%d, %d): unsupported hidden door at east edge (%d, %d)", seed.state1, seed.state2, x+1, y)
						}
					case WallDestructible:
						destructibles++
						destructibleSeen = true
						specialIndices = append(specialIndices, edgeIndex)
						if !m.wallIsSupported(mapEdge{x: x + 1, y: y}) {
							t.Fatalf("seed (%d, %d): unsupported destructible at east edge (%d, %d)", seed.state1, seed.state2, x+1, y)
						}
					}
					edgeIndex++
				}
				if y+1 < m.Length {
					next := Pos{X: x, Y: y + 1}
					if room.NWall != m.Rooms[next].SWall {
						t.Fatalf("seed (%d, %d): north/south walls disagree between %+v and %+v", seed.state1, seed.state2, pos, next)
					}
					switch room.NWall.Type {
					case WallDoor:
						doors++
						doorSeen = true
						specialIndices = append(specialIndices, edgeIndex)
						if !m.wallIsSupported(mapEdge{horizontal: true, x: x, y: y + 1}) {
							t.Fatalf("seed (%d, %d): unsupported %s at north edge (%d, %d)", seed.state1, seed.state2, room.NWall.Type, x, y+1)
						}
					case WallHiddenDoor:
						hiddenDoors++
						hiddenDoorSeen = true
						specialIndices = append(specialIndices, edgeIndex)
						if !m.wallIsSupported(mapEdge{horizontal: true, x: x, y: y + 1}) {
							t.Fatalf("seed (%d, %d): unsupported hidden door at north edge (%d, %d)", seed.state1, seed.state2, x, y+1)
						}
					case WallDestructible:
						destructibles++
						destructibleSeen = true
						specialIndices = append(specialIndices, edgeIndex)
						if !m.wallIsSupported(mapEdge{horizontal: true, x: x, y: y + 1}) {
							t.Fatalf("seed (%d, %d): unsupported destructible at north edge (%d, %d)", seed.state1, seed.state2, x, y+1)
						}
					}
					edgeIndex++
				}
			}
		}

		if doors > 2 || hiddenDoors > 2 || destructibles > 5 {
			t.Fatalf("seed (%d, %d): wall quotas exceeded: doors=%d hidden_doors=%d destructibles=%d", seed.state1, seed.state2, doors, hiddenDoors, destructibles)
		}
		if len(specialIndices) > 0 && specialIndices[0] < cooldown/2 {
			t.Fatalf("seed (%d, %d): first special wall at edge %d, before cooldown start %d", seed.state1, seed.state2, specialIndices[0], cooldown/2)
		}
		for i := 1; i < len(specialIndices); i++ {
			if difference := specialIndices[i] - specialIndices[i-1]; difference < cooldown {
				t.Fatalf("seed (%d, %d): special walls at edges %d and %d are only %d edges apart, cooldown=%d", seed.state1, seed.state2, specialIndices[i-1], specialIndices[i], difference, cooldown)
			}
		}
		assertAllRoomsReachable(t, m, seed)
	}

	// Strict support validation can remove valid quota candidates; maxima remain upper bounds.
	if !doorSeen {
		t.Fatalf("seed corpus did not exercise door wall")
	}
	if !hiddenDoorSeen {
		t.Fatalf("seed corpus did not exercise hidden door wall")
	}
	if !destructibleSeen {
		t.Fatalf("seed corpus did not exercise destructible wall")
	}
}

func TestSpecialWallTypesObeyInteriorEdgePlacementAttemptCooldownGate(t *testing.T) {
	for _, wallType := range []WallType{WallDoor, WallHiddenDoor, WallDestructible} {
		t.Run(string(wallType), func(t *testing.T) {
			if !isSpecialWall(wallType) {
				t.Fatalf("%q is not classified as special wall", wallType)
			}
			counts := make(map[WallType]int)
			limits := map[WallType]int{wallType: 1}
			if got := pickWallType(rand.New(rand.NewPCG(1, 2)), []WallType{wallType}, counts, limits, false); got != WallEmpty {
				t.Fatalf("cooldown should block %q during this interior-edge placement attempt, got %q", wallType, got)
			}
		})
	}
}

func TestValidateSpecialWalls(t *testing.T) {
	baseCases := []struct {
		name     string
		supports []mapEdge
		keep     bool
	}{
		{"straight", []mapEdge{{horizontal: true, x: 0, y: 1}, {horizontal: true, x: 2, y: 1}}, true},
		{"same-turn corner", []mapEdge{{x: 1, y: 0}, {x: 2, y: 0}}, true},
		{"opposite-turn corner", []mapEdge{{x: 1, y: 0}, {x: 2, y: 1}}, true},
		{"mixed orientation", []mapEdge{{horizontal: true, x: 0, y: 1}, {x: 2, y: 1}}, true},
		{"missing first endpoint", []mapEdge{{horizontal: true, x: 2, y: 1}}, false},
		{"missing second endpoint", []mapEdge{{horizontal: true, x: 0, y: 1}}, false},
		{"no support", nil, false},
	}

	for _, wallType := range []WallType{WallDoor, WallHiddenDoor, WallDestructible} {
		for _, horizontal := range []bool{true, false} {
			for _, test := range baseCases {
				t.Run(string(wallType)+"/"+orientationName(horizontal)+"/"+test.name, func(t *testing.T) {
					m := fixtureMap(4, 4)
					edge := mapEdge{horizontal: true, x: 1, y: 1}
					if !horizontal {
						edge = transposeFixtureEdge(edge)
					}
					health := 0
					if wallType == WallDestructible {
						health = 73
					}
					setFixtureWallWithHealth(&m, edge, wallType, health)
					for _, support := range test.supports {
						if !horizontal {
							support = transposeFixtureEdge(support)
						}
						setFixtureWall(&m, support, WallIndestructible)
					}
					m.validateSpecialWalls()

					wall, _ := m.wallAtEdge(edge)
					want := Wall{Type: WallEmpty}
					if test.keep {
						want = Wall{Type: wallType, Health: health}
					}
					if wall != want {
						t.Fatalf("edge became {%q, %d}, want {%q, %d}", wall.Type, wall.Health, want.Type, want.Health)
					}
					assertFixtureMirror(t, m, edge)
				})
			}
		}
	}
}

func orientationName(horizontal bool) string {
	if horizontal {
		return "horizontal"
	}
	return "vertical"
}

func transposeFixtureEdge(edge mapEdge) mapEdge {
	return mapEdge{horizontal: !edge.horizontal, x: edge.y, y: edge.x}
}

func TestValidateSpecialWallsRejectsDanglingVerticalDestructible(t *testing.T) {
	m := fixtureMap(4, 4)
	edge := mapEdge{x: 2, y: 1}
	setFixtureWallWithHealth(&m, edge, WallDestructible, 73)
	setFixtureWall(&m, mapEdge{horizontal: true, x: 2, y: 2}, WallIndestructible)
	m.validateSpecialWalls()

	wall, _ := m.wallAtEdge(edge)
	if wall != (Wall{Type: WallEmpty}) {
		t.Fatalf("dangling vertical destructible became {%q, %d}", wall.Type, wall.Health)
	}
	assertFixtureMirror(t, m, edge)
}

func TestValidateSpecialWallsRejectsSharedEndpoints(t *testing.T) {
	tests := []struct {
		name                  string
		first, second         mapEdge
		firstType, secondType WallType
		supports              []mapEdge
	}{
		{
			name:  "perpendicular",
			first: mapEdge{x: 2, y: 1}, second: mapEdge{horizontal: true, x: 1, y: 2},
			firstType: WallDoor, secondType: WallHiddenDoor,
			supports: []mapEdge{{horizontal: true, x: 1, y: 1}, {horizontal: true, x: 2, y: 2}, {x: 1, y: 1}},
		},
		{
			name:  "collinear adjacency",
			first: mapEdge{x: 2, y: 1}, second: mapEdge{x: 2, y: 2},
			firstType: WallHiddenDoor, secondType: WallDoor,
			supports: []mapEdge{{horizontal: true, x: 1, y: 1}, {horizontal: true, x: 1, y: 2}, {horizontal: true, x: 1, y: 3}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := fixtureMap(4, 4)
			setFixtureWall(&m, test.first, test.firstType)
			setFixtureWall(&m, test.second, test.secondType)
			for _, support := range test.supports {
				setFixtureWall(&m, support, WallIndestructible)
			}
			m.validateSpecialWalls()

			first, _ := m.wallAtEdge(test.first)
			second, _ := m.wallAtEdge(test.second)
			if first.Type != test.firstType || second.Type != WallEmpty {
				t.Fatalf("got first=%q second=%q, want first=%q second=%q", first.Type, second.Type, test.firstType, WallEmpty)
			}
			assertFixtureMirror(t, m, test.first)
			assertFixtureMirror(t, m, test.second)
		})
	}
}

func TestGeneratedMapsHaveUniqueSpecialWallEndpoints(t *testing.T) {
	for _, seed := range wallQuotaSeedPairs {
		m := Map{}
		m.Generate_map(rand.New(rand.NewPCG(seed.state1, seed.state2)))
		occupied := make(map[Pos]mapEdge)
		for y := 0; y < m.Length; y++ {
			for x := 0; x < m.Width; x++ {
				for _, edge := range []mapEdge{{x: x + 1, y: y}, {horizontal: true, x: x, y: y + 1}} {
					wall, exists := m.wallAtEdge(edge)
					if !exists || !isSpecialWall(wall.Type) {
						continue
					}
					if !m.wallIsSupported(edge) {
						t.Fatalf("seed (%d, %d): unsupported %s at %+v", seed.state1, seed.state2, wall.Type, edge)
					}
					first, second := edge.endpoints()
					for _, vertex := range []Pos{first, second} {
						if prior, exists := occupied[vertex]; exists {
							priorWall, _ := m.wallAtEdge(prior)
							t.Fatalf("seed (%d, %d): %s %+v conflicts with prior %s %+v at vertex %+v", seed.state1, seed.state2, wall.Type, edge, priorWall.Type, prior, vertex)
						}
						occupied[vertex] = edge
					}
				}
			}
		}
	}
}

func TestValidateSpecialWallsRejectsMixedSharedEndpoints(t *testing.T) {
	tests := []struct {
		name                  string
		firstType, secondType WallType
		supports              []mapEdge
	}{
		{
			name:      "destructible perpendicular",
			firstType: WallDestructible, secondType: WallDestructible,
			supports: []mapEdge{{horizontal: true, x: 1, y: 1}, {horizontal: true, x: 2, y: 2}, {x: 1, y: 1}},
		},
		{
			name:      "door perpendicular",
			firstType: WallDoor, secondType: WallDestructible,
			supports: []mapEdge{{horizontal: true, x: 1, y: 1}, {horizontal: true, x: 2, y: 2}, {x: 1, y: 1}},
		},
		{
			name:      "hidden door perpendicular",
			firstType: WallHiddenDoor, secondType: WallDestructible,
			supports: []mapEdge{{horizontal: true, x: 1, y: 1}, {horizontal: true, x: 2, y: 2}, {x: 1, y: 1}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := fixtureMap(4, 4)
			first := mapEdge{x: 2, y: 1}
			second := mapEdge{horizontal: true, x: 1, y: 2}
			setFixtureWall(&m, first, test.firstType)
			setFixtureWall(&m, second, test.secondType)
			for _, support := range test.supports {
				setFixtureWall(&m, support, WallIndestructible)
			}
			if !m.wallIsSupported(first) || !m.wallIsSupported(second) {
				t.Fatal("collision fixture must support both special walls before validation")
			}
			m.validateSpecialWalls()

			firstWall, _ := m.wallAtEdge(first)
			secondWall, _ := m.wallAtEdge(second)
			if firstWall.Type != test.firstType || secondWall.Type != WallEmpty {
				t.Fatalf("got first=%q second=%q, want first=%q second=%q", firstWall.Type, secondWall.Type, test.firstType, WallEmpty)
			}
			assertFixtureMirror(t, m, first)
			assertFixtureMirror(t, m, second)
		})
	}
}

func TestValidateSpecialWallsUnsupportedWallDoesNotReserveEndpoint(t *testing.T) {
	tests := []struct {
		name       string
		firstType  WallType
		secondType WallType
		supports   []mapEdge
	}{
		{name: "destructible first", firstType: WallDestructible, secondType: WallDestructible, supports: []mapEdge{{horizontal: true, x: 0, y: 2}, {horizontal: true, x: 2, y: 2}}},
		{name: "door first", firstType: WallDoor, secondType: WallDoor, supports: []mapEdge{{horizontal: true, x: 0, y: 2}, {horizontal: true, x: 2, y: 2}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := fixtureMap(4, 4)
			first := mapEdge{x: 2, y: 1}
			second := mapEdge{horizontal: true, x: 1, y: 2}
			setFixtureWall(&m, first, test.firstType)
			setFixtureWall(&m, second, test.secondType)
			for _, support := range test.supports {
				setFixtureWall(&m, support, WallIndestructible)
			}
			m.validateSpecialWalls()

			firstWall, _ := m.wallAtEdge(first)
			secondWall, _ := m.wallAtEdge(second)
			if firstWall.Type != WallEmpty || secondWall.Type != test.secondType {
				t.Fatalf("got first=%q second=%q, want first=%q second=%q", firstWall.Type, secondWall.Type, WallEmpty, test.secondType)
			}
		})
	}
}

func fixtureMap(width, length int) Map {
	m := Map{Width: width, Length: length, Rooms: make(map[Pos]*Room)}
	for y := 0; y < length; y++ {
		for x := 0; x < width; x++ {
			m.Rooms[Pos{X: x, Y: y}] = &Room{}
		}
	}
	return m
}

func setFixtureWall(m *Map, edge mapEdge, wallType WallType) {
	setFixtureWallWithHealth(m, edge, wallType, 0)
}

func setFixtureWallWithHealth(m *Map, edge mapEdge, wallType WallType, health int) {
	wall := Wall{Type: wallType, Health: health}
	if edge.horizontal {
		m.Rooms[Pos{X: edge.x, Y: edge.y - 1}].NWall = wall
		m.Rooms[Pos{X: edge.x, Y: edge.y}].SWall = wall
		return
	}
	m.Rooms[Pos{X: edge.x - 1, Y: edge.y}].EWall = wall
	m.Rooms[Pos{X: edge.x, Y: edge.y}].WWall = wall
}

func assertFixtureMirror(t *testing.T, m Map, edge mapEdge) {
	t.Helper()
	if edge.horizontal {
		if m.Rooms[Pos{X: edge.x, Y: edge.y - 1}].NWall != m.Rooms[Pos{X: edge.x, Y: edge.y}].SWall {
			t.Fatal("north/south special walls disagree")
		}
		return
	}
	if m.Rooms[Pos{X: edge.x - 1, Y: edge.y}].EWall != m.Rooms[Pos{X: edge.x, Y: edge.y}].WWall {
		t.Fatal("east/west special walls disagree")
	}
}

func assertAllRoomsReachable(t *testing.T, m Map, seed mapSeedPair) {
	t.Helper()
	visited := map[Pos]bool{m.StartPos: true}
	queue := []Pos{m.StartPos}
	for len(queue) > 0 {
		pos := queue[0]
		queue = queue[1:]
		room := m.Rooms[pos]
		neighbors := []struct {
			pos  Pos
			wall Wall
		}{
			{Pos{X: pos.X + 1, Y: pos.Y}, room.EWall},
			{Pos{X: pos.X - 1, Y: pos.Y}, room.WWall},
			{Pos{X: pos.X, Y: pos.Y + 1}, room.NWall},
			{Pos{X: pos.X, Y: pos.Y - 1}, room.SWall},
		}
		for _, neighbor := range neighbors {
			if !isPassage(neighbor.wall.Type) || visited[neighbor.pos] {
				continue
			}
			if _, exists := m.Rooms[neighbor.pos]; !exists {
				continue
			}
			visited[neighbor.pos] = true
			queue = append(queue, neighbor.pos)
		}
	}

	if len(visited) != len(m.Rooms) {
		t.Fatalf("seed (%d, %d): reached %d of %d rooms", seed.state1, seed.state2, len(visited), len(m.Rooms))
	}
}

func isPassage(wallType WallType) bool {
	return wallType == WallEmpty || wallType == WallDoor || wallType == WallDestructible
}
