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
	doorCapSeen := false
	hiddenDoorCapSeen := false
	destructibleCapSeen := false
	doorSeen := false
	hiddenDoorSeen := false
	destructibleSeen := false

	for _, seed := range wallQuotaSeedPairs {
		m := Map{}
		m.Generate_map(rand.New(rand.NewPCG(seed.state1, seed.state2)))

		doors, hiddenDoors, destructibles := 0, 0, 0
		specialIndices := make([]int, 0, 7)
		interiorEdgeCount := (m.Width-1)*m.Length + m.Width*(m.Length-1)
		cooldown := interiorEdgeCount / 7
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
					case WallHiddenDoor:
						hiddenDoors++
						hiddenDoorSeen = true
						specialIndices = append(specialIndices, edgeIndex)
					case WallDestructible:
						destructibles++
						destructibleSeen = true
						specialIndices = append(specialIndices, edgeIndex)
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
					case WallHiddenDoor:
						hiddenDoors++
						hiddenDoorSeen = true
						specialIndices = append(specialIndices, edgeIndex)
					case WallDestructible:
						destructibles++
						destructibleSeen = true
						specialIndices = append(specialIndices, edgeIndex)
					}
					edgeIndex++
				}
			}
		}

		if doors > 2 || hiddenDoors > 2 || destructibles > 3 {
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
		if doors == 2 {
			doorCapSeen = true
		}
		if hiddenDoors == 2 {
			hiddenDoorCapSeen = true
		}
		if destructibles == 3 {
			destructibleCapSeen = true
		}
		assertAllRoomsReachable(t, m, seed)
	}

	if !doorSeen || !doorCapSeen {
		t.Fatalf("seed corpus did not exercise door quota: encountered=%t reached_cap=%t", doorSeen, doorCapSeen)
	}
	if !hiddenDoorSeen || !hiddenDoorCapSeen {
		t.Fatalf("seed corpus did not exercise hidden door quota: encountered=%t reached_cap=%t", hiddenDoorSeen, hiddenDoorCapSeen)
	}
	if !destructibleSeen || !destructibleCapSeen {
		t.Fatalf("seed corpus did not exercise destructible quota: encountered=%t reached_cap=%t", destructibleSeen, destructibleCapSeen)
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
