package layout

import (
	"testing"

	"deskctl/internal/winapi"
	"deskctl/internal/windowrules"
)

func tree() windowrules.Node {
	return windowrules.Node{Kind: "group", Axis: "h", Children: []windowrules.Node{
		{Kind: "window", Weight: 60, Application: "a"},
		{Kind: "group", Axis: "v", Weight: 40, Children: []windowrules.Node{
			{Kind: "window", Weight: 50, Application: "b"},
			{Kind: "window", Weight: 50, Application: "c"},
		}},
	}}
}

func TestNestedGroupsUseSavedPercentages(t *testing.T) {
	area := winapi.Rect{X: -1000, Y: 0, Width: 1000, Height: 800}
	rects := Tile(tree(), area, map[string][]string{"0": {"A"}, "1.0": {"B"}, "1.1": {"C"}}, 0)
	want := map[string]winapi.Rect{
		"A": {X: -1000, Y: 0, Width: 600, Height: 800},
		"B": {X: -400, Y: 0, Width: 400, Height: 400},
		"C": {X: -400, Y: 400, Width: 400, Height: 400},
	}
	for id, rect := range want {
		if rects[id] != rect {
			t.Fatalf("%s: got %+v want %+v", id, rects[id], rect)
		}
	}
}

func TestEmptySlotsCollapseAndSharedLeafSplits(t *testing.T) {
	area := winapi.Rect{Width: 900, Height: 600}
	rects := Tile(tree(), area, map[string][]string{"0": {"A1", "A2"}, "1.1": {"C"}}, 0)
	if rects["A1"].Width != 270 || rects["A2"].X != 270 || rects["C"].Height != 600 || rects["C"].X != 540 {
		t.Fatal("empty leaf must collapse and shared leaf must split", rects)
	}
}

func TestGapIsUniform(t *testing.T) {
	area := winapi.Rect{Width: 1000, Height: 500}
	rects := Tile(windowrules.Node{Kind: "group", Axis: "h", Children: []windowrules.Node{
		{Kind: "window", Weight: 50, Application: "a"}, {Kind: "window", Weight: 50, Application: "b"},
	}}, area, map[string][]string{"0": {"A"}, "1": {"B"}}, 10)
	if rects["A"].X != 10 || rects["B"].X-(rects["A"].X+rects["A"].Width) != 10 || rects["B"].X+rects["B"].Width != 990 || rects["A"].Y != 10 {
		t.Fatal("gaps must be uniform", rects)
	}
}
