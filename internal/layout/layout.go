// Package layout turns a tiling tree into screen rectangles.
package layout

import (
	"strconv"

	"deskctl/internal/winapi"
	"deskctl/internal/windowrules"
)

// Tile computes one rectangle per window. Leaves and groups without windows
// collapse and give their space to their siblings; several windows in one leaf
// share it equally along the parent axis. gap is the space between and around windows.
func Tile(root windowrules.Node, area winapi.Rect, slots map[string][]string, gap int32) map[string]winapi.Rect {
	result := map[string]winapi.Rect{}
	if gap < 0 {
		gap = 0
	}
	half := gap / 2
	inner := winapi.Rect{X: area.X + half, Y: area.Y + half, Width: area.Width - gap, Height: area.Height - gap}
	place(root, "", "h", inner, slots, result)
	for id, rect := range result {
		result[id] = winapi.Rect{X: rect.X + half, Y: rect.Y + half, Width: max(rect.Width-gap, 1), Height: max(rect.Height-gap, 1)}
	}
	return result
}

func occupied(node windowrules.Node, path string, slots map[string][]string) bool {
	if node.Kind == "window" {
		return len(slots[path]) > 0
	}
	for index, child := range node.Children {
		if occupied(child, join(path, index), slots) {
			return true
		}
	}
	return false
}

func join(parent string, index int) string {
	if parent == "" {
		return strconv.Itoa(index)
	}
	return parent + "." + strconv.Itoa(index)
}

func place(node windowrules.Node, path, parentAxis string, area winapi.Rect, slots map[string][]string, result map[string]winapi.Rect) {
	if node.Kind == "window" {
		ids := slots[path]
		weights := make([]float64, len(ids))
		for index := range weights {
			weights[index] = 1
		}
		for index, rect := range split(area, parentAxis, weights) {
			result[ids[index]] = rect
		}
		return
	}
	type entry struct {
		node windowrules.Node
		path string
	}
	present := []entry{}
	weights := []float64{}
	for index, child := range node.Children {
		childPath := join(path, index)
		if occupied(child, childPath, slots) {
			present = append(present, entry{child, childPath})
			weights = append(weights, child.Weight)
		}
	}
	for index, rect := range split(area, node.Axis, weights) {
		place(present[index].node, present[index].path, node.Axis, rect, slots, result)
	}
}

// split divides area along axis ("h" = side by side, "v" = stacked) using
// cumulative rounding so the parts cover the area exactly.
func split(area winapi.Rect, axis string, weights []float64) []winapi.Rect {
	total := 0.0
	for _, weight := range weights {
		total += weight
	}
	if total <= 0 {
		total = float64(len(weights))
		for index := range weights {
			weights[index] = 1
		}
	}
	length := area.Width
	if axis == "v" {
		length = area.Height
	}
	parts := make([]winapi.Rect, 0, len(weights))
	cumulative, start := 0.0, int32(0)
	for _, weight := range weights {
		cumulative += weight
		end := int32(float64(length)*cumulative/total + 0.5)
		if axis == "v" {
			parts = append(parts, winapi.Rect{X: area.X, Y: area.Y + start, Width: area.Width, Height: end - start})
		} else {
			parts = append(parts, winapi.Rect{X: area.X + start, Y: area.Y, Width: end - start, Height: area.Height})
		}
		start = end
	}
	return parts
}
