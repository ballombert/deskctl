package gui

import (
	"cmp"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strings"

	"deskctl/internal/tiling"
	"deskctl/internal/winapi"
	"deskctl/internal/windoworganizer"
	"deskctl/internal/windowrules"
	"deskctl/internal/windowstore"
)

// staticBackend replays an inventory so a rule can be previewed without touching windows.
type staticBackend struct{ inventory windoworganizer.Inventory }

func (backend staticBackend) Inventory() (windoworganizer.Inventory, error) {
	return backend.inventory, nil
}
func (staticBackend) Move(windoworganizer.Window, string) error {
	return fmt.Errorf("apercu : aucun mouvement")
}

type noJournal struct{}

func (noJournal) LoadJournal() (windoworganizer.Journal, error) {
	return windoworganizer.Journal{}, nil
}
func (noJournal) SaveJournal(windoworganizer.Journal) error { return fmt.Errorf("apercu") }

// RulePreview tells which windows a rule matches and where they would go.
type RulePreview struct {
	Matches []string                    `json:"matches"`
	Layout  *windoworganizer.LayoutPlan `json:"layout,omitempty"`
}

func inventoryOf(plan windoworganizer.Plan) windoworganizer.Inventory {
	inventory := windoworganizer.Inventory{Desktops: plan.Desktops, Displays: plan.Displays, CurrentDesktopID: plan.CurrentDesktopID}
	for _, item := range plan.Items {
		inventory.Windows = append(inventory.Windows, item.Window)
	}
	return inventory
}

func hasRuleAlias(selected, aliases []string) bool {
	for _, id := range selected {
		if slices.Contains(aliases, id) {
			return true
		}
	}
	return false
}

func previewRule(plan windoworganizer.Plan, rule windowrules.Rule, gap int32, groups ...windowrules.Group) (RulePreview, error) {
	rule.ID, rule.Enabled = "preview", true
	if strings.TrimSpace(rule.Name) == "" {
		rule.Name = "Apercu"
	}
	engine := windoworganizer.New(staticBackend{inventoryOf(plan)}, noJournal{})
	engine.SetGap(gap)
	result, err := engine.Prepare(windowrules.Document{Version: windowrules.Version, Groups: groups, Rules: []windowrules.Rule{rule}})
	if err != nil {
		return RulePreview{}, err
	}
	preview := RulePreview{Matches: []string{}}
	for _, item := range result.Items {
		if slices.Contains(item.RuleIDs, rule.ID) {
			preview.Matches = append(preview.Matches, item.Window.ID)
		}
	}
	if len(result.Layouts) != 0 {
		preview.Layout = &result.Layouts[0]
	}
	return preview, nil
}

// captureRule builds a rule from the current position of the selected windows:
// the order and the percentages come from their rectangles.
func captureRule(plan windoworganizer.Plan, windowIDs []string, axis, id string) (windowrules.Rule, error) {
	if axis != "h" && axis != "v" {
		return windowrules.Rule{}, fmt.Errorf("axe inconnu : %s", axis)
	}
	windows := []windoworganizer.Window{}
	for _, item := range plan.Items {
		if slices.Contains(windowIDs, item.Window.ID) {
			windows = append(windows, item.Window)
		}
	}
	if len(windows) < 1 || len(windows) > 12 {
		return windowrules.Rule{}, fmt.Errorf("selectionner entre 1 et 12 fenetres")
	}
	start := func(window windoworganizer.Window) int32 {
		if axis == "v" {
			return window.Bounds.Y
		}
		return window.Bounds.X
	}
	size := func(window windoworganizer.Window) float64 {
		if axis == "v" {
			return float64(max(window.Bounds.Height, 1))
		}
		return float64(max(window.Bounds.Width, 1))
	}
	slices.SortStableFunc(windows, func(left, right windoworganizer.Window) int { return cmp.Compare(start(left), start(right)) })
	total := 0.0
	for _, window := range windows {
		total += size(window)
	}
	applications := map[string]int{}
	for _, window := range windows {
		applications[strings.ToLower(window.Application)]++
	}
	root := windowrules.Node{Kind: "group", Axis: axis, Weight: 100}
	names := []string{}
	for _, window := range windows {
		leaf := windowrules.Node{Kind: "window", Weight: math.Round(size(window)/total*10000) / 100,
			Application: "^" + regexp.QuoteMeta(window.Application) + "$"}
		if applications[strings.ToLower(window.Application)] > 1 {
			title := []rune(window.Title)
			leaf.Title = regexp.QuoteMeta(string(title[:min(len(title), 40)]))
		}
		root.Children = append(root.Children, leaf)
		names = append(names, strings.TrimSuffix(window.Application, ".exe"))
	}
	desktop := windows[0].DesktopID
	if windows[0].Pinned {
		desktop = winapi.AllDesktopsID
	}
	rule := windowrules.Rule{ID: id, Name: strings.Join(names, " + "), Enabled: true, DesktopID: desktop,
		DisplayID: windows[0].DisplayID, Layout: &root}
	doc, err := windowrules.Normalize(windowrules.Document{Version: windowrules.Version, Rules: []windowrules.Rule{rule}})
	if err != nil {
		return windowrules.Rule{}, err
	}
	captured := doc.Rules[0]
	captured.Layout = captured.EffectiveLayout()
	captured.GroupID = ""
	return captured, nil
}

// layoutJobs keeps the layouts to tile, without windows whose desktop move failed.
func layoutJobs(plan windoworganizer.Plan, report windoworganizer.Report, indices map[string]int) []tiling.Job {
	failed := map[string]bool{}
	for _, result := range report.Results {
		if result.Status == "uncertain" || result.Status == "failed" || strings.Contains(result.Message, "obsolete") {
			failed[result.ID] = true
		}
	}
	jobs := []tiling.Job{}
	for _, layout := range report.Layouts {
		if layout.Status != "pending" {
			continue
		}
		display := winapi.DisplayInfo{}
		for _, candidate := range plan.Displays {
			if candidate.ID == layout.DisplayID {
				display = candidate
			}
		}
		slots := []windoworganizer.Slot{}
		for _, slot := range layout.Slots {
			if !failed[slot.WindowID] {
				slots = append(slots, slot)
			}
		}
		if len(slots) == 0 {
			continue
		}
		layout.Slots = slots
		jobs = append(jobs, tiling.Job{Layout: layout, Display: display.Work, FancyIndex: indices[layout.DisplayID],
			OnCurrentDesktop: layout.Pinned || layout.DesktopID == plan.CurrentDesktopID})
	}
	return jobs
}

func geometryUndo(jobs []tiling.Job, plan windoworganizer.Plan) windowstore.GeometryUndo {
	undo := windowstore.GeometryUndo{Version: 2, InProgress: true, Records: []windowstore.GeometryRecord{}}
	for _, job := range jobs {
		for _, slot := range job.Layout.Slots {
			for _, item := range plan.Items {
				if item.Window.ID == slot.WindowID {
					after := slot.Rect
					undo.Records = append(undo.Records, windowstore.GeometryRecord{WindowID: slot.WindowID,
						ProcessID: item.Window.ProcessID, Started: item.Window.Started, Title: item.Window.Title, Rect: slot.Before,
						After: &after, Application: item.Window.Application, Class: item.Window.Class,
						DesktopID: job.Layout.DesktopID, Pinned: job.Layout.Pinned})
				}
			}
		}
	}
	return undo
}
