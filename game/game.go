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
	Rooms    map[Pos]Room
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

// Current types are "empty", "door", "hidden_door", "indestructible", "destructible"
type Wall struct {
	Type   string
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

// Also fixed the random wall type selection to include all types.
func (r *Room) Generate_room() {
	// Use more reasonable wall types for initial generation
	// Avoid "indestructible" as it creates dead ends
	wall_types := []string{"empty", "door", "hidden_door", "destructible"}
	r.NWall.Type = wall_types[rand.N(len(wall_types))]
	r.NWall.Health = 50 + rand.N(51)
	r.SWall.Type = wall_types[rand.N(len(wall_types))]
	r.SWall.Health = 50 + rand.N(51)
	r.EWall.Type = wall_types[rand.N(len(wall_types))]
	r.EWall.Health = 50 + rand.N(51)
	r.WWall.Type = wall_types[rand.N(len(wall_types))]
	r.WWall.Health = 50 + rand.N(51)
}

// generate_map creates a series of connected rooms using a randomized DFS algorithm with backtracking.
func (m *Map) Generate_map() {
	// Initialize map dimensions and the Rooms map
	m.Width = 7 + rand.N(4)  // Width will be between 7 and 10
	m.Length = 7 + rand.N(4) // Length will be between 7 and 10
	m.Rooms = make(map[Pos]Room)
	m.Explored = make(map[Pos]*Room)

	// A stack to keep track of the path for backtracking
	stack := []Pos{}

	// 1. Start at a random location.
	start_pos := Pos{
		X: rand.N(m.Width),
		Y: rand.N(m.Length),
	}

	// 2. Generate the first room, add it to the map (marking it as visited), and push to stack.
	var firstRoom Room
	firstRoom.Generate_room()
	m.StartPos = start_pos
	m.Rooms[start_pos] = firstRoom
	stack = append(stack, start_pos)

	// 3. This loop continues until we've backtracked all the way to the start.
	for len(stack) > 0 {
		// Get the current position from the top of the stack (without removing it).
		current_pos := stack[len(stack)-1]

		// Find all valid, unvisited neighbors.
		unvisited_neighbors := []Pos{}
		potential_moves := []Pos{
			{X: current_pos.X, Y: current_pos.Y + 1}, // North
			{X: current_pos.X + 1, Y: current_pos.Y}, // East
			{X: current_pos.X, Y: current_pos.Y - 1}, // South
			{X: current_pos.X - 1, Y: current_pos.Y}, // West
		}

		// Shuffle the potential moves to ensure random path generation.
		rand.Shuffle(len(potential_moves), func(i, j int) {
			potential_moves[i], potential_moves[j] = potential_moves[j], potential_moves[i]
		})

		for _, move := range potential_moves {
			// Check if the move is within the map bounds.
			isInBounds := move.X >= 0 && move.X < m.Width && move.Y >= 0 && move.Y < m.Length
			if !isInBounds {
				continue
			}

			// Check if a room already exists at this position.
			if _, exists := m.Rooms[move]; !exists {
				unvisited_neighbors = append(unvisited_neighbors, move)
			}
		}

		// If there are unvisited neighbors to move to...
		if len(unvisited_neighbors) > 0 {
			// ...choose the first one from our shuffled list.
			next_pos := unvisited_neighbors[0]

			// Retrieve the current room from the map.
			// Since structs in maps are values, we operate on a copy and must put it back.
			current_room := m.Rooms[current_pos]

			// Generate a new room at the next position.
			var newRoom Room
			newRoom.Generate_room()

			// Define the types of walls that allow passage.
			// passage_wall_types := []string{"empty", "door", "destructible"}
			passage_wall_types := []string{"empty"}
			passage_type := passage_wall_types[rand.N(len(passage_wall_types))]

			// Create a shared wall that represents the new passage.
			passage_wall := Wall{Type: passage_type}
			if passage_type == "destructible" {
				passage_wall.Health = 50 + rand.N(51)
			}

			// Determine direction of movement and "break down" the walls between the rooms.
			if next_pos.Y > current_pos.Y { // Moved North
				current_room.NWall = passage_wall
				newRoom.SWall = passage_wall
			} else if next_pos.Y < current_pos.Y { // Moved South
				current_room.SWall = passage_wall
				newRoom.NWall = passage_wall
			} else if next_pos.X > current_pos.X { // Moved East
				current_room.EWall = passage_wall
				newRoom.WWall = passage_wall
			} else if next_pos.X < current_pos.X { // Moved West
				current_room.WWall = passage_wall
				newRoom.EWall = passage_wall
			}

			// Place the modified current room and the new room back into the map.
			m.Rooms[current_pos] = current_room
			m.Rooms[next_pos] = newRoom
			// --- END of NEW LOGIC ---

			// Add the new position to the stack to continue the path.
			stack = append(stack, next_pos)

		} else {
			// If we're stuck (no unvisited neighbors), pop from the stack to backtrack.
			stack = stack[:len(stack)-1]
		}
	}

	// After generating all rooms, synchronize wall types between adjacent rooms
	m.synchronizeAdjacentWalls()

	// Make boundary walls indestructible
	m.makeBoundaryWallsIndestructible()
}

// synchronizeAdjacentWalls ensures that bordering rooms have walls of the same type
func (m *Map) synchronizeAdjacentWalls() {
	for pos, room := range m.Rooms {
		// Check each direction for adjacent rooms
		directions := []struct {
			delta        Pos
			currentWall  *Wall
			adjacentWall *Wall
		}{
			{Pos{X: 0, Y: 1}, &room.NWall, nil},  // North
			{Pos{X: 0, Y: -1}, &room.SWall, nil}, // South
			{Pos{X: 1, Y: 0}, &room.EWall, nil},  // East
			{Pos{X: -1, Y: 0}, &room.WWall, nil}, // West
		}

		for _, dir := range directions {
			adjacentPos := Pos{X: pos.X + dir.delta.X, Y: pos.Y + dir.delta.Y}
			if adjacentRoom, exists := m.Rooms[adjacentPos]; exists {
				// Determine which wall of the adjacent room faces this room
				var adjacentWall *Wall
				if dir.delta.Y == 1 { // This room is south of adjacent
					adjacentWall = &adjacentRoom.SWall
				} else if dir.delta.Y == -1 { // This room is north of adjacent
					adjacentWall = &adjacentRoom.NWall
				} else if dir.delta.X == 1 { // This room is west of adjacent
					adjacentWall = &adjacentRoom.WWall
				} else if dir.delta.X == -1 { // This room is east of adjacent
					adjacentWall = &adjacentRoom.EWall
				}

				// Synchronize the wall types, but preserve passage walls
				if adjacentWall != nil {
					// Don't override passage walls (empty, door, destructible) unless both walls are passage types
					currentIsPassage := dir.currentWall.Type == "empty" || dir.currentWall.Type == "door" || dir.currentWall.Type == "destructible"
					adjacentIsPassage := adjacentWall.Type == "empty" || adjacentWall.Type == "door" || adjacentWall.Type == "destructible"

					if currentIsPassage && adjacentIsPassage {
						// Both are passage walls, synchronize to the more restrictive type
						if dir.currentWall.Type == "empty" && adjacentWall.Type != "empty" {
							dir.currentWall.Type = adjacentWall.Type
							if adjacentWall.Type == "destructible" {
								dir.currentWall.Health = adjacentWall.Health
							}
						} else if adjacentWall.Type == "empty" && dir.currentWall.Type != "empty" {
							adjacentWall.Type = dir.currentWall.Type
							if dir.currentWall.Type == "destructible" {
								adjacentWall.Health = dir.currentWall.Health
							}
						} else if dir.currentWall.Type == "door" && adjacentWall.Type == "destructible" {
							dir.currentWall.Type = "destructible"
							adjacentWall.Type = "destructible"
							// Keep higher health
							if dir.currentWall.Health > adjacentWall.Health {
								adjacentWall.Health = dir.currentWall.Health
							} else {
								dir.currentWall.Health = adjacentWall.Health
							}
						} else if adjacentWall.Type == "door" && dir.currentWall.Type == "destructible" {
							adjacentWall.Type = "destructible"
							dir.currentWall.Type = "destructible"
							// Keep higher health
							if adjacentWall.Health > dir.currentWall.Health {
								dir.currentWall.Health = adjacentWall.Health
							} else {
								adjacentWall.Health = dir.currentWall.Health
							}
						}
					} else if currentIsPassage && !adjacentIsPassage {
						// Current is passage, adjacent is blocking - don't change passage
						// Keep current wall as passage
					} else if !currentIsPassage && adjacentIsPassage {
						// Adjacent is passage, current is blocking - don't change passage
						// Keep adjacent wall as passage
					} else {
						// Both are blocking walls, synchronize to more restrictive
						if dir.currentWall.Type == "indestructible" || adjacentWall.Type == "indestructible" {
							dir.currentWall.Type = "indestructible"
							adjacentWall.Type = "indestructible"
						} else if dir.currentWall.Type == "hidden_door" || adjacentWall.Type == "hidden_door" {
							dir.currentWall.Type = "hidden_door"
							adjacentWall.Type = "hidden_door"
						}
					}
				}
			}
		}

		// Update the room in the map
		m.Rooms[pos] = room
	}
}

// makeBoundaryWallsIndestructible sets walls at map boundaries to indestructible
func (m *Map) makeBoundaryWallsIndestructible() {
	for pos, room := range m.Rooms {
		// Check if this room is at any boundary and set the appropriate wall to indestructible

		// North boundary (Y == Length-1)
		if pos.Y == m.Length-1 {
			room.NWall.Type = "indestructible"
		}

		// South boundary (Y == 0)
		if pos.Y == 0 {
			room.SWall.Type = "indestructible"
		}

		// East boundary (X == Width-1)
		if pos.X == m.Width-1 {
			room.EWall.Type = "indestructible"
		}

		// West boundary (X == 0)
		if pos.X == 0 {
			room.WWall.Type = "indestructible"
		}

		// Update the room in the map
		m.Rooms[pos] = room
	}
}

// MovePlayer attempts to move the player in the specified direction.
// Returns true if the move was successful, false otherwise.
func (m *Map) MovePlayer(p *Player, direction string) bool {
	currentPos := p.Position
	var newPos Pos

	// Calculate new position based on direction
	switch direction {
	case "north":
		newPos = Pos{X: currentPos.X, Y: currentPos.Y + 1}
	case "south":
		newPos = Pos{X: currentPos.X, Y: currentPos.Y - 1}
	case "east":
		newPos = Pos{X: currentPos.X + 1, Y: currentPos.Y}
	case "west":
		newPos = Pos{X: currentPos.X - 1, Y: currentPos.Y}
	default:
		return false // Invalid direction
	}

	// Check if new position is within map bounds
	if newPos.X < 0 || newPos.X >= m.Width || newPos.Y < 0 || newPos.Y >= m.Length {
		return false
	}

	// Check if there's a room at the current position
	currentRoom, exists := m.Rooms[currentPos]
	if !exists {
		return false
	}

	// Check if the wall in the direction of movement allows passage
	var wallType string
	switch direction {
	case "north":
		wallType = currentRoom.NWall.Type
	case "south":
		wallType = currentRoom.SWall.Type
	case "east":
		wallType = currentRoom.EWall.Type
	case "west":
		wallType = currentRoom.WWall.Type
	}

	// Only allow movement through "empty" walls (and "door" for now, as doors should be passable)
	if wallType != "empty" && wallType != "door" {
		return false
	}

	// Move is valid, update player position
	p.Position = newPos

	// Explore the new room if it hasn't been explored yet
	if _, explored := m.Explored[newPos]; !explored {
		if room, exists := m.Rooms[newPos]; exists {
			m.Explored[newPos] = &room
		}
	}

	return true
}
