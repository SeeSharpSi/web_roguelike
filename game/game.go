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
	wall_types := []string{"empty", "door", "hidden_door", "indestructible", "destructible"}
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
			passage_wall_types := []string{"empty", "door", "destructible"}
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
}
