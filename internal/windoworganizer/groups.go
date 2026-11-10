package windoworganizer

import (
	"fmt"
	"slices"

	"deskctl/internal/winapi"
	"deskctl/internal/windowrules"
)

func geometryKey(rule windowrules.Rule) string {
	if rule.GroupID == "" {
		return rule.ID
	}
	zone := windowrules.Zone{Width: 1, Height: 1}
	if effective := rule.EffectiveZone(); effective != nil {
		zone = *effective
	}
	return fmt.Sprintf("%s|%s|%s|%g,%g,%g,%g", rule.GroupID, rule.DesktopID, rule.DisplayID,
		zone.X, zone.Y, zone.Width, zone.Height)
}

func overlaps(left, right []string) bool {
	for _, id := range left {
		if slices.Contains(right, id) {
			return true
		}
	}
	return false
}

func (engine *Engine) effectiveRules() []windowrules.Rule {
	rules := []windowrules.Rule{}
	used := map[string]bool{}
	for _, rule := range engine.rules.Rules {
		if manual, ok := engine.manualGroups[rule.GroupID]; ok {
			if rule.Enabled {
				used[rule.GroupID] = true
				manual.ID = rule.ID
				rules = append(rules, manual)
			}
		} else {
			rules = append(rules, rule)
		}
	}
	for _, group := range engine.rules.Groups {
		if manual, ok := engine.manualGroups[group.ID]; ok && !used[group.ID] {
			rules = append(rules, manual)
		}
	}
	return rules
}

// Invalidate preserves choices for display but makes stale execution impossible.
func (engine *Engine) Invalidate() {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	engine.current.ID = ""
}

func (engine *Engine) planGroups() []GroupState {
	groups := []GroupState{}
	for _, group := range engine.rules.Groups {
		state := GroupState{ID: group.ID, Name: group.Name, Members: []string{}, Status: "empty"}
		matcher := windowrules.Rule{Enabled: true, Layout: &group.Layout}
		for _, item := range engine.current.Items {
			if !matcher.Matches(item.Window.Application, item.Window.Title) {
				continue
			}
			state.Members = append(state.Members, item.Window.ID)
			if len(state.Members) == 1 {
				state.DisplayID, state.DesktopID = item.Window.DisplayID, item.Window.DesktopID
				if item.Window.Pinned {
					state.DesktopID = winapi.AllDesktopsID
				}
			}
			if item.Status == "blocked" || item.Status == "conflict" {
				state.Status, state.Explanation = "conflict", item.Explanation
			} else if state.Status != "conflict" {
				state.Status = "ready"
			}
		}
		display, found := findDisplay(engine.current.Displays, state.DisplayID)
		if !found {
			display, found = primaryDisplay(engine.current.Displays)
		}
		if found {
			state.DisplayID, state.Bounds = display.ID, display.Work
			if group.Zone != nil {
				state.Bounds = group.Zone.Rect(display.Work)
			}
		}
		for _, layout := range engine.current.Layouts {
			if layout.GroupID != group.ID {
				continue
			}
			state.DisplayID, state.DesktopID, state.Bounds = layout.DisplayID, layout.DesktopID, layout.Bounds
			if layout.Status == "conflict" {
				state.Status, state.Explanation = "conflict", layout.Explanation
			}
			break
		}
		if state.DesktopID == "" {
			state.DesktopID = engine.current.CurrentDesktopID
		}
		groups = append(groups, state)
	}
	return groups
}

func (engine *Engine) rebuildLayouts() {
	engine.current.Layouts = engine.planLayouts()
	for _, layout := range engine.current.Layouts {
		if layout.GroupID == "" || layout.Status != "conflict" {
			continue
		}
		for index := range engine.current.Items {
			item := &engine.current.Items[index]
			if overlaps(item.RuleIDs, layout.RuleIDs) && item.Status != "blocked" {
				item.Status, item.Explanation = "conflict", layout.Explanation
			}
		}
	}
	engine.current.Groups = engine.planGroups()
}

// AssignGroup prepares one coherent override; it never moves windows or saves rules.
func (engine *Engine) AssignGroup(planID, groupID, desktopID, displayID string, zone windowrules.Zone) (Plan, error) {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	if planID == "" || engine.current.ID != planID || engine.current.Mode != "arrange" {
		return Plan{}, fmt.Errorf("apercu obsolete : actualiser")
	}
	if !hasDestination(engine.current.Desktops, desktopID) {
		return Plan{}, fmt.Errorf("bureau cible absent")
	}
	if _, ok := findDisplay(engine.current.Displays, displayID); !ok {
		return Plan{}, fmt.Errorf("ecran cible absent")
	}
	if err := windowrules.ValidateZone(&zone); err != nil {
		return Plan{}, err
	}
	var group *windowrules.Group
	for index := range engine.rules.Groups {
		if engine.rules.Groups[index].ID == groupID {
			group = &engine.rules.Groups[index]
			break
		}
	}
	if group == nil {
		return Plan{}, fmt.Errorf("groupe absent : %s", groupID)
	}
	// A group override must not leave stale per-window desktop overrides behind.
	matcher := windowrules.Rule{Enabled: true, Layout: &group.Layout}
	for _, item := range engine.current.Items {
		if matcher.Matches(item.Window.Application, item.Window.Title) && item.Window.Blocked != "" {
			return Plan{}, fmt.Errorf("groupe non deplacable : %s", item.Window.Blocked)
		}
	}
	manual := windowrules.Rule{ID: "manual:" + group.ID, Name: group.Name, Enabled: true,
		GroupID: group.ID, DesktopID: desktopID, DisplayID: displayID, Zone: &zone}
	doc, err := windowrules.Normalize(windowrules.Document{Version: windowrules.Version,
		Groups: engine.rules.Groups, Rules: []windowrules.Rule{manual}})
	if err != nil {
		return Plan{}, err
	}
	revision, err := token()
	if err != nil {
		return Plan{}, err
	}
	engine.manualGroups[group.ID] = doc.Rules[0]
	for _, item := range engine.current.Items {
		if matcher.Matches(item.Window.Application, item.Window.Title) {
			delete(engine.manual, item.Window.ID)
		}
	}
	for index, item := range engine.current.Items {
		engine.current.Items[index] = engine.evaluate(item.Window)
	}
	engine.rebuildLayouts()
	engine.current.ID = revision
	return clonePlan(engine.current), nil
}
