package templ

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"seesharpsi/web_roguelike/game"
	"seesharpsi/web_roguelike/match"
)

type hudWall struct {
	Direction string
	Label     string
	Class     string
}

type hudItem struct {
	Name        string
	Glyph       string
	Class       string
	Description string
}

func playerNumber(snapshot match.Snapshot, playerID string) int {
	for index, player := range snapshot.Players {
		if player.ID == playerID {
			return index + 1
		}
	}
	return 0
}

func coordinateLabel(position game.Pos, length int) string {
	column := position.X + 1
	letters := ""
	for column > 0 {
		column--
		letters = string(rune('A'+column%26)) + letters
		column /= 26
	}
	return letters + strconv.Itoa(length-position.Y)
}

func mapProgressStyle(snapshot match.Snapshot) string {
	total := snapshot.World.Width * snapshot.World.Length
	if total < 1 {
		return "width: 0%"
	}
	progress := math.Min(100, float64(len(snapshot.World.Explored))*100/float64(total))
	return fmt.Sprintf("width: %.2f%%", progress)
}

func currentRoom(snapshot match.Snapshot) *game.Room {
	return snapshot.World.Rooms[snapshot.Viewer.Player.Position]
}

func currentRoomWalls(snapshot match.Snapshot) []hudWall {
	room := currentRoom(snapshot)
	if room == nil {
		return []hudWall{{Direction: "Current room", Label: "Room data unavailable", Class: "unknown"}}
	}
	return []hudWall{
		{Direction: "North", Label: wallDescription(room.NWall), Class: wallClass(room.NWall.Type)},
		{Direction: "East", Label: wallDescription(room.EWall), Class: wallClass(room.EWall.Type)},
		{Direction: "South", Label: wallDescription(room.SWall), Class: wallClass(room.SWall.Type)},
		{Direction: "West", Label: wallDescription(room.WWall), Class: wallClass(room.WWall.Type)},
	}
}

func roomDescription(snapshot match.Snapshot) string {
	walls := currentRoomWalls(snapshot)
	if len(walls) == 1 && walls[0].Class == "unknown" {
		return walls[0].Label + "."
	}
	descriptions := make([]string, 0, len(walls))
	for _, wall := range walls {
		descriptions = append(descriptions, wall.Direction+": "+wall.Label)
	}
	return strings.Join(descriptions, ". ") + "."
}

func wallDescription(wall game.Wall) string {
	switch wall.Type {
	case game.WallEmpty:
		return "Open passage"
	case game.WallDoor:
		return "Door"
	case game.WallHiddenDoor:
		return "Hidden door"
	case game.WallIndestructible:
		return "Standard wall"
	case game.WallDestructible:
		return fmt.Sprintf("Destructible wall · %d integrity", wall.Health)
	default:
		return "Unknown wall"
	}
}

func wallClass(wallType game.WallType) string {
	switch wallType {
	case game.WallEmpty:
		return "open"
	case game.WallDoor, game.WallHiddenDoor:
		return "door"
	case game.WallDestructible:
		return "damaged"
	case game.WallIndestructible:
		return "normal"
	default:
		return "unknown"
	}
}

func roomDamageCount(snapshot match.Snapshot) int {
	room := currentRoom(snapshot)
	if room == nil {
		return 0
	}
	count := 0
	for _, wall := range [...]game.Wall{room.NWall, room.EWall, room.SWall, room.WWall} {
		if wall.Type == game.WallDestructible {
			count++
		}
	}
	return count
}

func roomSignalClass(snapshot match.Snapshot) string {
	if roomDamageCount(snapshot) > 0 {
		return "room-signal room-signal-warning"
	}
	return "room-signal"
}

func roomSignalTitle(snapshot match.Snapshot) string {
	if roomDamageCount(snapshot) > 0 {
		return "LOCAL WARNING"
	}
	return "LOCAL SCAN"
}

func roomSignalCopy(snapshot match.Snapshot) string {
	if count := roomDamageCount(snapshot); count > 0 {
		if count == 1 {
			return "1 destructible wall detected in this room."
		}
		return fmt.Sprintf("%d destructible walls detected in this room.", count)
	}
	return "No destructible walls detected in this room."
}

func directiveCopy(snapshot match.Snapshot) string {
	if snapshot.Viewer.Player.Role != "" {
		return "Follow your assigned objective while exploring the shared sector."
	}
	return "No role assigned. Explore to reveal more of the shared sector."
}

func roleLabel(role game.Role) string {
	return strings.TrimSpace(string(role))
}

func playerRowClass(player match.ParticipantView, snapshot match.Snapshot) string {
	if player.ID == snapshot.Viewer.ID {
		return "player-row player-row-viewer"
	}
	return "player-row"
}

func playerBadgeClass(player match.ParticipantView, snapshot match.Snapshot) string {
	if player.ID == snapshot.Viewer.ID {
		return "crew-badge crew-badge-viewer"
	}
	return "crew-badge"
}

func vitalFillStyle(value int) string {
	value = max(0, min(100, value))
	return fmt.Sprintf("width: %d%%", value)
}

func playerVitalsStatus(player game.Player) string {
	if player.Alive {
		return "ALIVE"
	}
	return "INACTIVE"
}

func roomItems(snapshot match.Snapshot) []hudItem {
	room := currentRoom(snapshot)
	if room == nil || len(room.Items) == 0 {
		return nil
	}
	items := make([]hudItem, 0, min(len(room.Items), 6))
	for _, item := range room.Items {
		if len(items) == 6 {
			break
		}
		view, ok := describeItem(item.Type)
		if ok {
			items = append(items, view)
		}
	}
	return items
}

func describeItem(itemType game.ItemType) (hudItem, bool) {
	switch itemType {
	case game.ItemKeycard:
		return hudItem{
			Name:        "Keycard",
			Glyph:       "K",
			Class:       "keycard",
			Description: "An access card recovered from this room.",
		}, true
	case game.ItemMedGel:
		return hudItem{
			Name:        "Med gel",
			Glyph:       "+",
			Class:       "med-gel",
			Description: "Medical supplies discovered in the current room.",
		}, true
	case game.ItemCharge:
		return hudItem{
			Name:        "Charge",
			Glyph:       "II",
			Class:       "charge",
			Description: "A compact explosive charge.",
		}, true
	case game.ItemClapper:
		return hudItem{
			Name:        "Clapper",
			Glyph:       "O",
			Class:       "clapper",
			Description: "An acoustic decoy recovered from this room.",
		}, true
	default:
		return hudItem{}, false
	}
}

func quickSlotClass(slot int) string {
	if slot == 0 {
		return "quick-slot quick-slot-selected"
	}
	return "quick-slot"
}
