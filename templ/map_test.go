package templ_test

import (
	"bytes"
	"context"
	"math/rand/v2"
	"regexp"
	"sort"
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

func TestRenderedSpecialEdgesHaveBlackSupport(t *testing.T) {
	cellRE := regexp.MustCompile(`<div class="room-cell" style="([^"]*)">`)
	colorRE := regexp.MustCompile(`border-(top|right|bottom|left)-color:\s*([^;]+);`)
	special := map[string]bool{
		"saddlebrown": true,
		"peru":        true,
		"firebrick":   true,
	}
	seen := map[string]bool{}

	for seed := 1; seed <= 256; seed++ {
		m := game.Map{}
		m.Generate_map(rand.New(rand.NewPCG(uint64(seed), uint64(seed+1000))))
		var output bytes.Buffer
		if err := templ.Map(m, game.Player{}).Render(context.Background(), &output); err != nil {
			t.Fatalf("seed (%d, %d): render: %v", seed, seed+1000, err)
		}

		cells := cellRE.FindAllStringSubmatch(output.String(), -1)
		if len(cells) != m.Width*m.Length {
			t.Fatalf("seed (%d, %d): got %d room cells, want %d", seed, seed+1000, len(cells), m.Width*m.Length)
		}
		edges := make(map[screenEdge]string)
		for i, cell := range cells {
			x, y := i%m.Width, i/m.Width
			colors := make(map[string]string)
			for _, match := range colorRE.FindAllStringSubmatch(cell[1], -1) {
				colors[match[1]] = strings.TrimSpace(match[2])
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
				if previous, exists := edges[side.edge]; exists && previous != color {
					t.Fatalf("seed (%d, %d): edge %+v colors disagree: %q and %q", seed, seed+1000, side.edge, previous, color)
				}
				edges[side.edge] = color
			}
		}

		blackVertices := make(map[screenVertex]bool)
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
			first := screenVertex{edge.x, edge.y}
			second := first
			if edge.horizontal {
				second.x++
			} else {
				second.y++
			}
			if color == "black" {
				blackVertices[first], blackVertices[second] = true, true
			}
		}
		for _, edge := range edgeList {
			color := edges[edge]
			first := screenVertex{edge.x, edge.y}
			second := first
			if edge.horizontal {
				second.x++
			} else {
				second.y++
			}
			if !special[color] {
				continue
			}
			seen[color] = true
			for _, vertex := range []screenVertex{first, second} {
				if specialVertices[vertex] {
					t.Fatalf("seed (%d, %d): color %q edge %+v shares endpoint vertex %+v", seed, seed+1000, color, edge, vertex)
				}
				specialVertices[vertex] = true
				if !blackVertices[vertex] {
					t.Fatalf("seed (%d, %d): color %q edge %+v unsupported vertex %+v", seed, seed+1000, color, edge, vertex)
				}
			}
		}
	}
	for _, color := range []string{"saddlebrown", "peru", "firebrick"} {
		if !seen[color] {
			t.Fatalf("corpus did not render special color %q", color)
		}
	}
}
