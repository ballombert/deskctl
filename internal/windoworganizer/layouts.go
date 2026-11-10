package windoworganizer

import (
	"fmt"
	"slices"

	"deskctl/internal/layout"
	"deskctl/internal/winapi"
	"deskctl/internal/windowrules"
)

// Slot is the planned position of one window.
type Slot struct {
	WindowID    string      `json:"windowId"`
	Application string      `json:"application"`
	Title       string      `json:"title"`
	Path        string      `json:"path,omitempty"`
	Rect        winapi.Rect `json:"rect"`
	Before      winapi.Rect `json:"before"`
	Elevated    bool        `json:"elevated,omitempty"`
}

// LayoutPlan is the geometry part of a rule: which screen, which rectangles.
type LayoutPlan struct {
	RuleID      string            `json:"ruleId"`
	RuleName    string            `json:"ruleName"`
	DisplayID   string            `json:"displayId"`
	DesktopID   string            `json:"desktopId"`
	Pinned      bool              `json:"pinned"`
	Layout      *windowrules.Node `json:"layout,omitempty"`
	Slots       []Slot            `json:"slots"`
	Status      string            `json:"status"`
	Explanation string            `json:"explanation"`
	GroupID     string            `json:"groupId,omitempty"`
	RuleIDs     []string          `json:"ruleIds"`
	Bounds      winapi.Rect       `json:"bounds"`
}

func findDisplay(displays []winapi.DisplayInfo, id string) (winapi.DisplayInfo, bool) {
	for _, display := range displays {
		if display.ID == id {
			return display, true
		}
	}
	return winapi.DisplayInfo{}, false
}

func primaryDisplay(displays []winapi.DisplayInfo) (winapi.DisplayInfo, bool) {
	for _, display := range displays {
		if display.Primary {
			return display, true
		}
	}
	if len(displays) == 0 {
		return winapi.DisplayInfo{}, false
	}
	return displays[0], true
}

// Rescale keeps the relative position of a window when it changes screen.
func Rescale(bounds winapi.Rect, from, to winapi.Rect) winapi.Rect { return rescale(bounds, from, to) }

func rescale(bounds winapi.Rect, from, to winapi.Rect) winapi.Rect {
	if from.Width <= 0 || from.Height <= 0 {
		width, height := min(bounds.Width, to.Width), min(bounds.Height, to.Height)
		return winapi.Rect{X: to.X + (to.Width-width)/2, Y: to.Y + (to.Height-height)/2, Width: width, Height: height}
	}
	scale := func(value, fromStart, fromSize, toStart, toSize int32) int32 {
		return toStart + int32(float64(value-fromStart)*float64(toSize)/float64(fromSize)+0.5)
	}
	rect := winapi.Rect{
		X: scale(bounds.X, from.X, from.Width, to.X, to.Width), Y: scale(bounds.Y, from.Y, from.Height, to.Y, to.Height),
		Width:  int32(float64(bounds.Width)*float64(to.Width)/float64(from.Width) + 0.5),
		Height: int32(float64(bounds.Height)*float64(to.Height)/float64(from.Height) + 0.5),
	}
	rect.Width, rect.Height = min(max(rect.Width, 1), to.Width), min(max(rect.Height, 1), to.Height)
	rect.X = min(max(rect.X, to.X), to.X+to.Width-rect.Width)
	rect.Y = min(max(rect.Y, to.Y), to.Y+to.Height-rect.Height)
	return rect
}

func near(left, right winapi.Rect) bool {
	abs := func(value int32) int32 { return max(value, -value) }
	return abs(left.X-right.X) <= 4 && abs(left.Y-right.Y) <= 4 && abs(left.Width-right.Width) <= 4 && abs(left.Height-right.Height) <= 4
}

// planLayouts must be called with engine.mu held.
func (engine *Engine) planLayouts() []LayoutPlan {
	plans := []LayoutPlan{}
	rules := engine.effectiveRules()
	seen := map[string]bool{}
	for _, rule := range rules {
		if !rule.Enabled || !rule.HasGeometry() {
			continue
		}
		key := geometryKey(rule)
		if seen[key] {
			continue
		}
		seen[key] = true
		plan := LayoutPlan{RuleID: rule.ID, RuleName: rule.Name, DesktopID: rule.DesktopID,
			Pinned: rule.DesktopID == winapi.AllDesktopsID, Layout: rule.EffectiveLayout(), Slots: []Slot{},
			GroupID: rule.GroupID, RuleIDs: []string{rule.ID}}
		for _, other := range rules {
			if other.Enabled && other.ID != rule.ID && geometryKey(other) == key {
				plan.RuleIDs = append(plan.RuleIDs, other.ID)
			}
		}
		members := []Window{}
		conflict := ""
		for _, item := range engine.current.Items {
			if rule.GroupID != "" && item.Window.Blocked != "" {
				matcher := rule
				matcher.Enabled = true
				if matcher.Matches(item.Window.Application, item.Window.Title) {
					conflict = "Groupe incomplet : " + item.Window.Blocked
				}
			}
			if slices.Contains(item.RuleIDs, rule.ID) && item.Status == "conflict" {
				conflict = item.Explanation
			}
			if slices.Contains(item.RuleIDs, rule.ID) && item.Status != "blocked" && item.Status != "conflict" {
				members = append(members, item.Window)
			}
			if conflict != "" {
				plan.Status, plan.Explanation = "conflict", conflict
				plans = append(plans, plan)
				continue
			}
		}
		display, found := findDisplay(engine.current.Displays, rule.DisplayID)
		if rule.DisplayID != "" && !found {
			plan.Status, plan.Explanation = "conflict", "Ecran absent : "+rule.DisplayID
			plans = append(plans, plan)
			continue
		}
		if !found && len(members) != 0 {
			display, found = findDisplay(engine.current.Displays, members[0].DisplayID)
		}
		if !found {
			display, found = primaryDisplay(engine.current.Displays)
		}
		if !found {
			plan.Status, plan.Explanation = "conflict", "Aucun ecran detecte"
			plans = append(plans, plan)
			continue
		}
		plan.DisplayID = display.ID
		plan.Bounds = display.Work
		if zone := rule.EffectiveZone(); zone != nil {
			plan.Bounds = zone.Rect(display.Work)
		}
		if len(members) == 0 {
			plan.Status, plan.Explanation = "empty", "Aucune fenetre ouverte ne correspond"
			plans = append(plans, plan)
			continue
		}
		byID := map[string]Window{}
		for _, window := range members {
			byID[window.ID] = window
		}
		add := func(window Window, path string, rect winapi.Rect) {
			plan.Slots = append(plan.Slots, Slot{WindowID: window.ID, Application: window.Application, Title: window.Title,
				Path: path, Rect: rect, Before: window.Bounds, Elevated: window.Elevated})
		}
		if rule.EffectiveLayout() != nil {
			candidates := make([]windowrules.Candidate, 0, len(members))
			for _, window := range members {
				candidates = append(candidates, windowrules.Candidate{ID: window.ID, Application: window.Application, Title: window.Title})
			}
			slots := rule.Slots(candidates)
			rects := layout.Tile(*rule.EffectiveLayout(), plan.Bounds, slots, engine.gap)
			for _, window := range members {
				for path, ids := range slots {
					if slices.Contains(ids, window.ID) {
						add(window, path, rects[window.ID])
					}
				}
			}
		} else {
			for _, window := range members {
				source, ok := findDisplay(engine.current.Displays, window.DisplayID)
				from := winapi.Rect{}
				if ok {
					from = source.Work
				}
				if ok && source.ID == display.ID {
					add(window, "", window.Bounds)
				} else {
					add(window, "", rescale(byID[window.ID].Bounds, from, display.Work))
				}
			}
		}
		plan.Status, plan.Explanation = "unchanged", "Deja en place"
		elevated := 0
		for _, slot := range plan.Slots {
			if rule.GroupID != "" && (slot.Rect.X < plan.Bounds.X || slot.Rect.Y < plan.Bounds.Y ||
				slot.Rect.X+slot.Rect.Width > plan.Bounds.X+plan.Bounds.Width ||
				slot.Rect.Y+slot.Rect.Height > plan.Bounds.Y+plan.Bounds.Height) {
				plan.Status, plan.Explanation = "conflict", "Zone trop petite pour les tuiles et les marges"
				break
			}
			if slot.Elevated {
				elevated++
			}
			if !near(slot.Rect, slot.Before) {
				plan.Status, plan.Explanation = "pending", fmt.Sprintf("%d fenetre(s) a placer sur %s", len(plan.Slots), display.Name)
			}
		}
		if elevated != 0 {
			plan.Explanation += fmt.Sprintf(" ; %d fenetre(s) administrateur : lancer deskctl en administrateur pour les redimensionner", elevated)
		}
		plans = append(plans, plan)
	}
	return plans
}
