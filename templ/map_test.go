package templ_test

import (
	"bytes"
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"seesharpsi/web_roguelike/game"
	"seesharpsi/web_roguelike/templ"
)

type screenEdge struct {
	horizontal bool
	x, y       int
}

type screenVertex struct{ x, y int }

type renderedWall struct {
	start, end screenVertex
	wallType   game.WallType
	position   game.Pos
	color      string
	className  string
}

var (
	openingTagRE = regexp.MustCompile(`<([a-zA-Z][a-zA-Z0-9:-]*)\b([^>]*)>`)
	attributeRE  = regexp.MustCompile(`([a-zA-Z_:][a-zA-Z0-9_:.-]*)="([^"]*)"`)
	colorRE      = regexp.MustCompile(`border-(top|right|bottom|left)-color:\s*([^;]+);`)
)

func TestRenderedSpecialEdgesHaveCreamWallSupport(t *testing.T) {
	const normalWallColor = "#E8DEC3"
	const doorColor = "#4AA6C8"
	const destructibleColor = "#D85B52"
	cellCountRE := regexp.MustCompile(`grid-template-columns:\s*repeat\((\d+),`)
	rowCountRE := regexp.MustCompile(`grid-template-rows:\s*repeat\((\d+),`)
	normalColors := map[string]bool{normalWallColor: true}
	specialColors := map[string]bool{doorColor: true, destructibleColor: true}
	allowedColors := map[string]bool{"transparent": true, normalWallColor: true, doorColor: true, destructibleColor: true}
	seen := map[string]bool{}

	for seed := 1; seed <= 256; seed++ {
		m := game.Map{}
		m.Generate_map(rand.New(rand.NewPCG(uint64(seed), uint64(seed+1000))))
		output := renderMap(t, m, game.Player{})

		gridStyle := renderedMapGridStyle(t, output)
		columns := cellCountRE.FindStringSubmatch(gridStyle)
		if len(columns) != 2 {
			t.Fatalf("seed (%d, %d): map grid has no inline column count in %q", seed, seed+1000, gridStyle)
		}
		if got, err := strconv.Atoi(columns[1]); err != nil || got != m.Width {
			t.Fatalf("seed (%d, %d): grid column count %q, want %d", seed, seed+1000, columns[1], m.Width)
		}
		rows := rowCountRE.FindStringSubmatch(gridStyle)
		if len(rows) != 2 {
			t.Fatalf("seed (%d, %d): map grid has no inline row count in %q", seed, seed+1000, gridStyle)
		}
		if got, err := strconv.Atoi(rows[1]); err != nil || got != m.Length {
			t.Fatalf("seed (%d, %d): grid row count %q, want %d", seed, seed+1000, rows[1], m.Length)
		}

		cells := renderedRoomCells(output)
		if len(cells) != m.Width*m.Length {
			t.Fatalf("seed (%d, %d): got %d room cells, want %d", seed, seed+1000, len(cells), m.Width*m.Length)
		}
		edges := make(map[screenEdge]string)
		for i, cell := range cells {
			x, y := i%m.Width, i/m.Width
			worldPos := game.Pos{X: x, Y: m.Length - 1 - y}
			room := m.Rooms[worldPos]
			colors := make(map[string]string)
			for _, match := range colorRE.FindAllStringSubmatch(cell["style"], -1) {
				colors[match[1]] = strings.TrimSpace(match[2])
			}
			for _, side := range []struct {
				name     string
				wallType game.WallType
			}{
				{name: "top", wallType: room.NWall.Type},
				{name: "right", wallType: room.EWall.Type},
				{name: "bottom", wallType: room.SWall.Type},
				{name: "left", wallType: room.WWall.Type},
			} {
				if got, want := colors[side.name], wallPalette(side.wallType); got != want {
					t.Fatalf("seed (%d, %d): top-down cell (%d, %d) %s color = %q, want %q for wall type %q", seed, seed+1000, x, worldPos.Y, side.name, got, want, side.wallType)
				}
			}
			for _, side := range []struct {
				name string
				edge screenEdge
			}{
				{"top", screenEdge{horizontal: true, x: x, y: y}},
				{"right", screenEdge{x: x + 1, y: y}},
				{"bottom", screenEdge{horizontal: true, x: x, y: y + 1}},
				{"left", screenEdge{x: x, y: y}},
			} {
				color := colors[side.name]
				if color == "" {
					t.Fatalf("seed (%d, %d): room cell %d missing %s border color", seed, seed+1000, i, side.name)
				}
				if !allowedColors[color] {
					t.Fatalf("seed (%d, %d): room cell %d %s border has unexpected color %q", seed, seed+1000, i, side.name, color)
				}
				if previous, exists := edges[side.edge]; exists && previous != color {
					t.Fatalf("seed (%d, %d): edge %+v colors disagree: %q and %q", seed, seed+1000, side.edge, previous, color)
				}
				edges[side.edge] = color
			}
		}

		normalVertices := make(map[screenVertex]bool)
		specialVertices := make(map[screenVertex]bool)
		edgeList := make([]screenEdge, 0, len(edges))
		for edge := range edges {
			edgeList = append(edgeList, edge)
		}
		sort.Slice(edgeList, func(i, j int) bool {
			if edgeList[i].horizontal != edgeList[j].horizontal {
				return edgeList[i].horizontal
			}
			if edgeList[i].y != edgeList[j].y {
				return edgeList[i].y < edgeList[j].y
			}
			return edgeList[i].x < edgeList[j].x
		})
		for _, edge := range edgeList {
			color := edges[edge]
			first, second := edgeVertices(edge)
			if normalColors[color] {
				normalVertices[first], normalVertices[second] = true, true
			}
		}
		for _, edge := range edgeList {
			color := edges[edge]
			first, second := edgeVertices(edge)
			if !specialColors[color] {
				continue
			}
			seen[color] = true
			for _, vertex := range []screenVertex{first, second} {
				if specialVertices[vertex] {
					t.Fatalf("seed (%d, %d): special color %q edge %+v shares endpoint vertex %+v", seed, seed+1000, color, edge, vertex)
				}
				specialVertices[vertex] = true
				if !normalVertices[vertex] {
					t.Fatalf("seed (%d, %d): special color %q edge %+v unsupported by cream wall at vertex %+v", seed, seed+1000, color, edge, vertex)
				}
			}
		}
	}
	for _, color := range []string{doorColor, destructibleColor} {
		if !seen[color] {
			t.Fatalf("seed corpus did not render special wall color %q", color)
		}
	}
}

func TestMapWithPlayersRendersCanonicalWallOverlay(t *testing.T) {
	const cellSize = 38
	m := twoByTwoMap()
	viewerPosition := game.Pos{X: 0, Y: 1}
	viewer := game.Player{Alive: true, Position: viewerPosition}
	players := []templ.MapPlayer{{
		ID:       "viewer-id",
		Username: "Ada",
		Position: viewerPosition,
		IsViewer: true,
		Number:   2,
	}}
	var output bytes.Buffer
	if err := templ.MapWithPlayers(m, viewer, players).Render(context.Background(), &output); err != nil {
		t.Fatalf("render map with players: %v", err)
	}
	html := output.String()

	cells := renderedRoomCells(html)
	wantPositions := []game.Pos{{X: 0, Y: 1}, {X: 1, Y: 1}, {X: 0, Y: 0}, {X: 1, Y: 0}}
	if len(cells) != len(wantPositions) {
		t.Fatalf("got %d room cells, want %d", len(cells), len(wantPositions))
	}
	for i, cell := range cells {
		got := game.Pos{X: mustIntAttribute(t, cell, "data-x"), Y: mustIntAttribute(t, cell, "data-y")}
		if got != wantPositions[i] {
			t.Errorf("room cell %d coordinate = %+v, want top-down position %+v", i, got, wantPositions[i])
		}
	}
	markerAttrs, hasViewerMarker := findClassedElement(html, "span", "viewer-marker")
	if !hasViewerMarker || markerAttrs["data-player-id"] != "viewer-id" || !strings.Contains(html, "P2") {
		t.Fatal("viewer marker should identify viewer and display player number P2")
	}

	overlayAttrs, ok := findClassedElement(html, "svg", "map-walls")
	if !ok {
		t.Fatal("rendered map has no map-walls SVG overlay")
	}
	if got := overlayAttrs["viewBox"]; got != "0 0 76 76" {
		t.Errorf("map-walls viewBox = %q, want %q", got, "0 0 76 76")
	}
	assertThreePixelWallStroke(t)
	if got := borderColorMap(cells[0]["style"])["left"]; got != "transparent" {
		t.Errorf("empty wall cell border color = %q, want transparent", got)
	}

	walls := renderedOverlayWalls(t, html)
	wantTypes := fixtureWallTypes(cellSize)
	if len(walls) != len(wantTypes) {
		t.Fatalf("got %d overlay edges, want %d non-empty 2x2-grid edges", len(walls), len(wantTypes))
	}
	gotTypes := make(map[string]game.WallType, len(walls))
	for _, wall := range walls {
		key := geometryKey(wall.start, wall.end)
		if _, exists := gotTypes[key]; exists {
			t.Errorf("overlay draws shared edge %s more than once", key)
		}
		gotTypes[key] = wall.wallType
		if !hasClass(wall.className, "map-wall") {
			t.Errorf("edge %s has classes %q, want map-wall", key, wall.className)
		}
		if wantColor := wallPalette(wall.wallType); wall.color != wantColor {
			t.Errorf("edge %s (%s) stroke = %q, want %q", key, wall.wallType, wall.color, wantColor)
		}
		if wantPosition := overlayWallPosition(wall.start, wall.end, cellSize); wall.position != wantPosition {
			t.Errorf("edge %s data coordinate = %+v, want owning room %+v", key, wall.position, wantPosition)
		}
		if wall.start.x < 0 || wall.start.y < 0 || wall.end.x > 2*cellSize || wall.end.y > 2*cellSize {
			t.Errorf("edge %s lies outside 2x2 map bounds", key)
		}
		if wall.start.x != wall.end.x && wall.start.y != wall.end.y {
			t.Errorf("edge %s is not horizontal or vertical", key)
		}
		if abs(wall.start.x-wall.end.x)+abs(wall.start.y-wall.end.y) != cellSize {
			t.Errorf("edge %s length is not one %dpx cell", key, cellSize)
		}
	}
	if len(gotTypes) != len(wantTypes) {
		t.Fatalf("got %d distinct overlay edges, want %d", len(gotTypes), len(wantTypes))
	}
	for key, wantType := range wantTypes {
		if got, exists := gotTypes[key]; !exists || got != wantType {
			t.Errorf("edge %s type = %q (present %t), want %q", key, got, exists, wantType)
		}
	}
}

func renderMap(t *testing.T, m game.Map, player game.Player) string {
	t.Helper()
	var output bytes.Buffer
	if err := templ.Map(m, player).Render(context.Background(), &output); err != nil {
		t.Fatalf("render map: %v", err)
	}
	return output.String()
}

func renderedMapGridStyle(t *testing.T, html string) string {
	t.Helper()
	if attrs, ok := findClassedElement(html, "div", "map-grid"); ok {
		return attrs["style"]
	}
	t.Fatal("rendered map has no map-grid element")
	return ""
}

func renderedRoomCells(html string) []map[string]string {
	var cells []map[string]string
	for _, match := range openingTagRE.FindAllStringSubmatch(html, -1) {
		if match[1] != "div" {
			continue
		}
		attrs := parseAttributes(match[2])
		if hasClass(attrs["class"], "room-cell") {
			cells = append(cells, attrs)
		}
	}
	return cells
}

func renderedOverlayWalls(t *testing.T, html string) []renderedWall {
	t.Helper()
	var walls []renderedWall
	for _, match := range openingTagRE.FindAllStringSubmatch(html, -1) {
		if match[1] != "line" {
			continue
		}
		attrs := parseAttributes(match[2])
		wallType, ok := attrs["data-wall-type"]
		if !ok {
			continue
		}
		walls = append(walls, renderedWall{
			start:     screenVertex{x: mustParseInt(t, attrs["x1"]), y: mustParseInt(t, attrs["y1"])},
			end:       screenVertex{x: mustParseInt(t, attrs["x2"]), y: mustParseInt(t, attrs["y2"])},
			wallType:  game.WallType(wallType),
			position:  game.Pos{X: mustParseInt(t, attrs["data-x"]), Y: mustParseInt(t, attrs["data-y"])},
			color:     attrs["stroke"],
			className: attrs["class"],
		})
	}
	return walls
}

func findClassedElement(html, tag, className string) (map[string]string, bool) {
	for _, match := range openingTagRE.FindAllStringSubmatch(html, -1) {
		if match[1] != tag {
			continue
		}
		attrs := parseAttributes(match[2])
		if hasClass(attrs["class"], className) {
			return attrs, true
		}
	}
	return nil, false
}

func parseAttributes(raw string) map[string]string {
	attrs := make(map[string]string)
	for _, match := range attributeRE.FindAllStringSubmatch(raw, -1) {
		attrs[match[1]] = match[2]
	}
	return attrs
}

func hasClass(classList, target string) bool {
	for _, className := range strings.Fields(classList) {
		if className == target {
			return true
		}
	}
	return false
}

func mustIntAttribute(t *testing.T, attrs map[string]string, name string) int {
	t.Helper()
	value, err := strconv.Atoi(attrs[name])
	if err != nil {
		t.Fatalf("attribute %s=%q is not an integer", name, attrs[name])
	}
	return value
}

func mustParseInt(t *testing.T, value string) int {
	t.Helper()
	parsed, err := strconv.Atoi(value)
	if err != nil {
		t.Fatalf("expected integer SVG coordinate, got %q", value)
	}
	return parsed
}

func edgeVertices(edge screenEdge) (screenVertex, screenVertex) {
	first := screenVertex{edge.x, edge.y}
	second := first
	if edge.horizontal {
		second.x++
	} else {
		second.y++
	}
	return first, second
}

func twoByTwoMap() game.Map {
	positions := []game.Pos{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 0, Y: 1}, {X: 1, Y: 1}}
	m := game.Map{
		Width:    2,
		Length:   2,
		Rooms:    make(map[game.Pos]*game.Room, len(positions)),
		Explored: make(map[game.Pos]*game.Room),
	}
	for _, position := range positions {
		m.Rooms[position] = &game.Room{
			NWall: game.Wall{Type: game.WallIndestructible},
			SWall: game.Wall{Type: game.WallIndestructible},
			EWall: game.Wall{Type: game.WallIndestructible},
			WWall: game.Wall{Type: game.WallIndestructible},
		}
		m.Explored[position] = m.Rooms[position]
	}
	// Keep shared interior walls identical on both adjacent rooms.
	m.Rooms[game.Pos{X: 0, Y: 0}].NWall = game.Wall{Type: game.WallIndestructible}
	m.Rooms[game.Pos{X: 0, Y: 1}].SWall = game.Wall{Type: game.WallIndestructible}
	m.Rooms[game.Pos{X: 0, Y: 0}].EWall = game.Wall{Type: game.WallIndestructible}
	m.Rooms[game.Pos{X: 1, Y: 0}].WWall = game.Wall{Type: game.WallIndestructible}

	m.Rooms[game.Pos{X: 0, Y: 0}].WWall = game.Wall{Type: game.WallDoor}
	m.Rooms[game.Pos{X: 1, Y: 1}].EWall = game.Wall{Type: game.WallHiddenDoor}
	m.Rooms[game.Pos{X: 1, Y: 0}].SWall = game.Wall{Type: game.WallDestructible}
	m.Rooms[game.Pos{X: 0, Y: 1}].WWall = game.Wall{Type: game.WallEmpty}
	return m
}

func fixtureWallTypes(cellSize int) map[string]game.WallType {
	point := func(x, y int) screenVertex { return screenVertex{x: x * cellSize, y: y * cellSize} }
	walls := make(map[string]game.WallType)
	for _, edge := range [][2]screenVertex{
		{point(0, 0), point(1, 0)}, {point(1, 0), point(2, 0)},
		{point(0, 1), point(1, 1)}, {point(1, 1), point(2, 1)},
		{point(0, 2), point(1, 2)}, {point(1, 2), point(2, 2)},
		{point(0, 0), point(0, 1)}, {point(0, 1), point(0, 2)},
		{point(1, 0), point(1, 1)}, {point(1, 1), point(1, 2)},
		{point(2, 0), point(2, 1)}, {point(2, 1), point(2, 2)},
	} {
		walls[geometryKey(edge[0], edge[1])] = game.WallIndestructible
	}
	walls[geometryKey(point(0, 1), point(0, 2))] = game.WallDoor
	walls[geometryKey(point(2, 0), point(2, 1))] = game.WallHiddenDoor
	walls[geometryKey(point(1, 2), point(2, 2))] = game.WallDestructible
	delete(walls, geometryKey(point(0, 0), point(0, 1)))
	return walls
}

func geometryKey(first, second screenVertex) string {
	if first.x > second.x || first.x == second.x && first.y > second.y {
		first, second = second, first
	}
	return fmt.Sprintf("(%d,%d)-(%d,%d)", first.x, first.y, second.x, second.y)
}

func overlayWallPosition(start, end screenVertex, cellSize int) game.Pos {
	x := start.x
	if end.x < x {
		x = end.x
	}
	x /= cellSize
	if x > 1 {
		x = 1
	}
	y := start.y
	if end.y < y {
		y = end.y
	}
	y = 1 - y/cellSize
	if y < 0 {
		y = 0
	}
	return game.Pos{X: x, Y: y}
}

func wallPalette(wallType game.WallType) string {
	switch wallType {
	case game.WallEmpty:
		return "transparent"
	case game.WallDoor, game.WallHiddenDoor:
		return "#4AA6C8"
	case game.WallIndestructible:
		return "#E8DEC3"
	case game.WallDestructible:
		return "#D85B52"
	default:
		return ""
	}
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func assertThreePixelWallStroke(t *testing.T) {
	t.Helper()
	css, err := os.ReadFile("../static/map.css")
	if err != nil {
		t.Fatalf("read map stylesheet: %v", err)
	}
	strokeRule := regexp.MustCompile(`(?s)\.map-wall\s*\{[^}]*stroke-width:\s*3px\s*;`)
	if !strokeRule.Match(css) {
		t.Fatal("map-wall stylesheet rule must keep shared wall stroke at 3px")
	}
}

func borderColorMap(style string) map[string]string {
	colors := make(map[string]string)
	for _, match := range colorRE.FindAllStringSubmatch(style, -1) {
		colors[match[1]] = strings.TrimSpace(match[2])
	}
	return colors
}
