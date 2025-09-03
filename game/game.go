package game

import "math/rand/v2"

// A struct that contains player stats
type Player struct {
	Alive    bool
	Health   int
	Strength float64 // Strength is a multiplier
	Stamina  int
	Position Pos
	Items    []Item
}

type Enemy struct {
	Alive       bool
	Name        string
	Description string
	Health      int
	Turns_alive int
	Strength    float64 // Strength is a multiplier
	Position    Pos
}

type Map struct {
	Width       int
	Length      int
	Rooms       map[Pos]Room
	StartPos    Pos
	Explored    map[Pos]*Room
	Enemies     []Enemy
	Bombs       []Pos // Positions of bomb items
	PlacedBombs []PlacedBomb
}

type PlacedBomb struct {
	Position  Pos
	TurnsLeft int
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

	Dropped_Items []Item // Player dropped things
}

// Current types are "empty", "door", "hidden_door", "indestructible", "destructible"
type Wall struct {
	Type   string
	Health int
}

type Item struct {
	Name        string
	Description string
	Turns_alive int
}

func (p *Player) Generate_player() {
	p.Alive = true
	p.Health = 30
	p.Strength = float64(80+rand.N(21)) / 100
	p.Stamina = 50 + rand.N(51)
}

func (e *Enemy) Generate_enemy() {
	e.Alive = true
	e.Health = 3 + rand.N(2)
	e.Strength = float64(80+rand.N(21)) / 100
}

// Also fixed the random wall type selection to include all types.
func (r *Room) Generate_room() {
	// Use more reasonable wall types for initial generation
	// Avoid "indestructible" as it creates dead ends
	wall_types := []string{"empty", "door", "door", "door", "hidden_door", "destructible", "indestructible", "indestructible", "indestructible"}
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
	m.Width = 9 + rand.N(5)  // Width will be between 7 and 10
	m.Length = 9 + rand.N(5) // Length will be between 7 and 10
	m.Rooms = make(map[Pos]Room)
	m.Explored = make(map[Pos]*Room)
	m.Enemies = make([]Enemy, 0)          // Initialize enemies slice
	m.Bombs = make([]Pos, 0)              // Initialize bombs slice
	m.PlacedBombs = make([]PlacedBomb, 0) // Initialize placed bombs slice

	// Room counter for enemy and bomb generation
	roomCount := 0

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

	// Increment room count and check for enemy and bomb generation
	roomCount++
	if roomCount%15 == 0 {
		var enemy Enemy
		enemy.Generate_enemy()
		enemy.Position = start_pos
		enemy.Name = "Goblin"
		enemy.Description = "A nasty little goblin"
		m.Enemies = append(m.Enemies, enemy)
	}
	if roomCount%7 == 0 {
		// Place a bomb at this room position
		m.Bombs = append(m.Bombs, start_pos)
	}

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

			// Increment room count and check for enemy and bomb generation
			roomCount++
			if roomCount%15 == 0 {
				var enemy Enemy
				enemy.Generate_enemy()
				enemy.Position = next_pos
				enemy.Name = "Goblin"
				enemy.Description = "A nasty little goblin"
				m.Enemies = append(m.Enemies, enemy)
			}
			if roomCount%7 == 0 {
				// Place a bomb at this room position
				m.Bombs = append(m.Bombs, next_pos)
			}

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

// MovePlayer attempts to move the player in the specified direction or place a bomb.
// Returns true if the action was successful, false otherwise.
func (m *Map) MovePlayer(p *Player, action string) bool {
	currentPos := p.Position

	// Handle bomb placement
	if action == "place_bomb" {
		if m.PlaceBomb(currentPos, p) {
			// Move enemies first, then update bombs so enemies take damage if they move into blast radius
			m.MoveEnemies(currentPos)
			m.UpdateBombs(p)
			return true
		}
		return false
	}

	// Handle movement
	var newPos Pos
	direction := action

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

	// Check for enemy collision and apply damage
	crossingDetected := false
	for i := range m.Enemies {
		enemy := &m.Enemies[i]
		if enemy.Alive && enemy.Position.X == newPos.X && enemy.Position.Y == newPos.Y {
			// Check if enemy is adjacent to player (potential crossing scenario)
			dx := enemy.Position.X - currentPos.X
			dy := enemy.Position.Y - currentPos.Y

			// If enemy is exactly one space away (adjacent), it's a crossing scenario
			if (dx == 0 && (dy == 1 || dy == -1)) || (dy == 0 && (dx == 1 || dx == -1)) {
				// Crossing scenario: neither moves, player takes damage
				damage := enemy.Health
				p.Health -= damage
				if p.Health < 0 {
					p.Health = 0
				}
				crossingDetected = true
			} else {
				// Normal collision: player moves into enemy, takes damage
				damage := enemy.Health
				p.Health -= damage
				if p.Health < 0 {
					p.Health = 0
				}
			}
			break
		}
	}

	// Only move player if it's not a crossing scenario
	if !crossingDetected {
		// Move is valid, update player position
		p.Position = newPos

		// Check for bombs at the new position and collect them
		for i := len(m.Bombs) - 1; i >= 0; i-- {
			bombPos := m.Bombs[i]
			if bombPos.X == newPos.X && bombPos.Y == newPos.Y {
				// Add bomb to player's inventory
				bombItem := Item{
					Name:        "Bomb",
					Description: "A powerful explosive device",
					Turns_alive: 0, // Bombs don't decay
				}
				p.Items = append(p.Items, bombItem)

				// Remove bomb from map
				m.Bombs = append(m.Bombs[:i], m.Bombs[i+1:]...)
			}
		}
	}

	// Only explore and move enemies if player actually moved
	if !crossingDetected {
		// Explore the new room if it hasn't been explored yet
		if _, explored := m.Explored[newPos]; !explored {
			if room, exists := m.Rooms[newPos]; exists {
				m.Explored[newPos] = &room
			}
		}

		// Explore rooms visible from the new position
		m.ExploreVisibleRooms(newPos)

		// Move enemies first, then update bombs so enemies take damage if they move into blast radius
		m.MoveEnemies(newPos)
		m.UpdateBombs(p)
	} else {
		// Move enemies first, then update bombs so enemies take damage if they move into blast radius
		m.MoveEnemies(currentPos)
		m.UpdateBombs(p)
	}

	return true
}

// UpdateBombs decrements bomb timers and explodes bombs when they reach 0
func (m *Map) UpdateBombs(player *Player) {
	for i := len(m.PlacedBombs) - 1; i >= 0; i-- {
		bomb := &m.PlacedBombs[i]
		bomb.TurnsLeft--

		if bomb.TurnsLeft <= 0 {
			// Bomb explodes - destroy adjacent destructible walls and doors
			m.ExplodeBomb(bomb.Position, player)
			// Remove the bomb
			m.PlacedBombs = append(m.PlacedBombs[:i], m.PlacedBombs[i+1:]...)
		}
	}
}

// ExplodeBomb destroys adjacent destructible walls and doors and damages nearby entities
func (m *Map) ExplodeBomb(pos Pos, player *Player) {
	directions := []struct {
		delta        Pos
		bombWall     func(*Room) *Wall
		adjacentWall func(*Room) *Wall
	}{
		{Pos{X: 0, Y: 1}, func(r *Room) *Wall { return &r.NWall }, func(r *Room) *Wall { return &r.SWall }},  // North
		{Pos{X: 0, Y: -1}, func(r *Room) *Wall { return &r.SWall }, func(r *Room) *Wall { return &r.NWall }}, // South
		{Pos{X: 1, Y: 0}, func(r *Room) *Wall { return &r.EWall }, func(r *Room) *Wall { return &r.WWall }},  // East
		{Pos{X: -1, Y: 0}, func(r *Room) *Wall { return &r.WWall }, func(r *Room) *Wall { return &r.EWall }}, // West
	}

	// Check for damage to player and enemies in affected areas
	affectedPositions := []Pos{pos} // Include the bomb position
	for _, dir := range directions {
		adjacentPos := Pos{X: pos.X + dir.delta.X, Y: pos.Y + dir.delta.Y}
		if adjacentPos.X >= 0 && adjacentPos.X < m.Width && adjacentPos.Y >= 0 && adjacentPos.Y < m.Length {
			affectedPositions = append(affectedPositions, adjacentPos)
		}
	}

	// Damage player if they're in an affected area
	if player.Position.X == pos.X && player.Position.Y == pos.Y {
		player.Health -= 5
		if player.Health < 0 {
			player.Health = 0
		}
	} else {
		for _, adjPos := range affectedPositions[1:] { // Skip the bomb position since we already checked it
			if player.Position.X == adjPos.X && player.Position.Y == adjPos.Y {
				player.Health -= 5
				if player.Health < 0 {
					player.Health = 0
				}
				break
			}
		}
	}

	// Damage enemies in affected areas
	for i := range m.Enemies {
		enemy := &m.Enemies[i]
		if !enemy.Alive {
			continue
		}

		// Check if enemy is in bomb position
		if enemy.Position.X == pos.X && enemy.Position.Y == pos.Y {
			enemy.Health -= 2
			if enemy.Health <= 0 {
				enemy.Alive = false
			}
			continue
		}

		// Check if enemy is in adjacent positions
		for _, adjPos := range affectedPositions[1:] {
			if enemy.Position.X == adjPos.X && enemy.Position.Y == adjPos.Y {
				enemy.Health -= 2
				if enemy.Health <= 0 {
					enemy.Alive = false
				}
				break
			}
		}
	}

	for _, dir := range directions {
		adjacentPos := Pos{X: pos.X + dir.delta.X, Y: pos.Y + dir.delta.Y}

		// Check if adjacent position is valid
		if adjacentPos.X < 0 || adjacentPos.X >= m.Width || adjacentPos.Y < 0 || adjacentPos.Y >= m.Length {
			continue
		}

		bombRoom, bombExists := m.Rooms[pos]
		adjacentRoom, adjacentExists := m.Rooms[adjacentPos]

		if !bombExists || !adjacentExists {
			continue
		}

		// Get the walls that need to be destroyed
		bombWall := dir.bombWall(&bombRoom)
		adjacentWall := dir.adjacentWall(&adjacentRoom)

		// Destroy destructible walls and doors on both sides
		if bombWall != nil && (bombWall.Type == "destructible" || bombWall.Type == "door") {
			bombWall.Type = "empty"
			bombWall.Health = 0
		}
		if adjacentWall != nil && (adjacentWall.Type == "destructible" || adjacentWall.Type == "door") {
			adjacentWall.Type = "empty"
			adjacentWall.Health = 0
		}

		// Update both rooms in the map
		m.Rooms[pos] = bombRoom
		m.Rooms[adjacentPos] = adjacentRoom

		// Update the explored map
		// Only update explored rooms that the player can currently see
		if _, bombExplored := m.Explored[pos]; bombExplored {
			m.Explored[pos] = &bombRoom
			// Only update adjacent room if it's already explored
			if _, adjacentExplored := m.Explored[adjacentPos]; adjacentExplored {
				m.Explored[adjacentPos] = &adjacentRoom
			}
			// Don't add new rooms to explored - let ExploreVisibleRooms handle that
		} else {
			// If bomb room is not explored, only update adjacent room if it's already explored
			if _, adjacentExplored := m.Explored[adjacentPos]; adjacentExplored {
				m.Explored[adjacentPos] = &adjacentRoom
			}
		}
	}
}

// GetValidEnemyMoves returns a list of valid directions an enemy can move from its current position.
// An enemy can only move if both walls between current and target rooms are empty.
func (m *Map) GetValidEnemyMoves(enemyPos Pos) []string {
	var validMoves []string
	directions := []string{"north", "south", "east", "west"}

	for _, direction := range directions {
		var newPos Pos
		var currentWallType, targetWallType string

		// Calculate new position
		switch direction {
		case "north":
			newPos = Pos{X: enemyPos.X, Y: enemyPos.Y + 1}
		case "south":
			newPos = Pos{X: enemyPos.X, Y: enemyPos.Y - 1}
		case "east":
			newPos = Pos{X: enemyPos.X + 1, Y: enemyPos.Y}
		case "west":
			newPos = Pos{X: enemyPos.X - 1, Y: enemyPos.Y}
		}

		// Check if new position is within map bounds
		if newPos.X < 0 || newPos.X >= m.Width || newPos.Y < 0 || newPos.Y >= m.Length {
			continue
		}

		// Get current room
		currentRoom, currentExists := m.Rooms[enemyPos]
		if !currentExists {
			continue
		}

		// Get target room
		targetRoom, targetExists := m.Rooms[newPos]
		if !targetExists {
			continue
		}

		// Get wall types for both rooms
		switch direction {
		case "north":
			currentWallType = currentRoom.NWall.Type
			targetWallType = targetRoom.SWall.Type
		case "south":
			currentWallType = currentRoom.SWall.Type
			targetWallType = targetRoom.NWall.Type
		case "east":
			currentWallType = currentRoom.EWall.Type
			targetWallType = targetRoom.WWall.Type
		case "west":
			currentWallType = currentRoom.WWall.Type
			targetWallType = targetRoom.EWall.Type
		}

		// Both walls must be empty for enemy to move
		if currentWallType == "empty" && targetWallType == "empty" {
			validMoves = append(validMoves, direction)
		}
	}

	return validMoves
}

// PlaceBomb places a bomb at the player's current position if they have bombs
func (m *Map) PlaceBomb(playerPos Pos, player *Player) bool {
	// Check if player has bombs
	if len(player.Items) == 0 {
		return false // No bombs to place
	}

	// Remove one bomb from inventory
	player.Items = player.Items[:len(player.Items)-1]

	// Place bomb at player's position
	bomb := PlacedBomb{
		Position:  playerPos,
		TurnsLeft: 4,
	}
	m.PlacedBombs = append(m.PlacedBombs, bomb)

	return true
}

// MoveEnemies moves all living enemies in random valid directions
func (m *Map) MoveEnemies(playerPos Pos) {
	for i := range m.Enemies {
		enemy := &m.Enemies[i]
		if !enemy.Alive {
			continue
		}

		// Get valid moves for this enemy
		validMoves := m.GetValidEnemyMoves(enemy.Position)

		// If no valid moves, enemy stays put
		if len(validMoves) == 0 {
			continue
		}

		// Choose a random valid direction
		randomIndex := rand.N(len(validMoves))
		chosenDirection := validMoves[randomIndex]

		// Calculate new position
		var newPos Pos
		switch chosenDirection {
		case "north":
			newPos = Pos{X: enemy.Position.X, Y: enemy.Position.Y + 1}
		case "south":
			newPos = Pos{X: enemy.Position.X, Y: enemy.Position.Y - 1}
		case "east":
			newPos = Pos{X: enemy.Position.X + 1, Y: enemy.Position.Y}
		case "west":
			newPos = Pos{X: enemy.Position.X - 1, Y: enemy.Position.Y}
		}

		// Check if the new position would be occupied by the player
		if newPos.X == playerPos.X && newPos.Y == playerPos.Y {
			// Enemy cannot move into player's position
			continue
		}

		// Update enemy position
		enemy.Position = newPos
	}
}

// ExploreVisibleRooms marks rooms visible from the player's current position as explored.
// The player can see up to 2 rooms away, but only through empty walls.
func (m *Map) ExploreVisibleRooms(playerPos Pos) {
	// Directions and their corresponding walls
	directions := []struct {
		delta        Pos
		playerWall   func(Room) *Wall
		adjacentWall func(Room) *Wall
	}{
		{Pos{X: 0, Y: 1}, func(r Room) *Wall { return &r.NWall }, func(r Room) *Wall { return &r.SWall }},  // North
		{Pos{X: 0, Y: -1}, func(r Room) *Wall { return &r.SWall }, func(r Room) *Wall { return &r.NWall }}, // South
		{Pos{X: 1, Y: 0}, func(r Room) *Wall { return &r.EWall }, func(r Room) *Wall { return &r.WWall }},  // East
		{Pos{X: -1, Y: 0}, func(r Room) *Wall { return &r.WWall }, func(r Room) *Wall { return &r.EWall }}, // West
	}

	// Check each direction
	for _, dir := range directions {
		// Check first adjacent room
		adjacentPos := Pos{X: playerPos.X + dir.delta.X, Y: playerPos.Y + dir.delta.Y}

		// Check if adjacent position is within bounds
		if adjacentPos.X < 0 || adjacentPos.X >= m.Width || adjacentPos.Y < 0 || adjacentPos.Y >= m.Length {
			continue
		}

		playerRoom, playerExists := m.Rooms[playerPos]
		adjacentRoom, adjacentExists := m.Rooms[adjacentPos]

		if !playerExists || !adjacentExists {
			continue
		}

		// Check if the wall from player's room allows visibility (must be empty)
		playerWall := dir.playerWall(playerRoom)
		if playerWall.Type != "empty" {
			continue
		}

		// Check if the facing wall from adjacent room allows visibility (must be empty)
		adjacentWall := dir.adjacentWall(adjacentRoom)
		if adjacentWall.Type != "empty" {
			continue
		}

		// First adjacent room is visible, mark it as explored
		if _, explored := m.Explored[adjacentPos]; !explored {
			m.Explored[adjacentPos] = &adjacentRoom
		}

		// Now check the second room in this direction
		secondPos := Pos{X: adjacentPos.X + dir.delta.X, Y: adjacentPos.Y + dir.delta.Y}

		// Check if second position is within bounds
		if secondPos.X < 0 || secondPos.X >= m.Width || secondPos.Y < 0 || secondPos.Y >= m.Length {
			continue
		}

		secondRoom, secondExists := m.Rooms[secondPos]
		if !secondExists {
			continue
		}

		// Check if the wall from adjacent room to second room allows visibility
		adjacentToSecondWall := dir.playerWall(adjacentRoom)
		if adjacentToSecondWall.Type != "empty" {
			continue
		}

		// Check if the facing wall from second room allows visibility
		secondWall := dir.adjacentWall(secondRoom)
		if secondWall.Type != "empty" {
			continue
		}

		// Second room is visible, mark it as explored
		if _, explored := m.Explored[secondPos]; !explored {
			m.Explored[secondPos] = &secondRoom
		}
	}
}
