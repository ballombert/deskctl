// Package tiling places groups of windows with FancyWM or with native Win32 calls.
package tiling

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"deskctl/internal/winapi"
	"deskctl/internal/windoworganizer"
	"deskctl/internal/windowrules"
)

// Desktop abstracts the Win32 calls so engines can be tested.
type Desktop interface {
	Rect(windowID string) (winapi.Rect, error)
	SetRect(windowID string, rect winapi.Rect) error
	Focus(windowID string) error
	Wait(time.Duration)
}

// Fancy abstracts the FancyWM command line.
type Fancy interface {
	Running() bool
	Action(action string) error
}

// Job is one rule layout to apply.
type Job struct {
	Layout windoworganizer.LayoutPlan
	// Display is the target screen work area; FancyIndex its FancyWM index (0 = unknown).
	Display    winapi.Rect
	FancyIndex int
	// OnCurrentDesktop is true when every window is visible on the current desktop,
	// which FancyWM requires because its actions target the focused window.
	OnCurrentDesktop bool
}

type Outcome struct {
	WindowID string `json:"windowId"`
	Title    string `json:"title"`
	Status   string `json:"status"`
	Message  string `json:"message"`
	Engine   string `json:"engine"`
}

const settle = 250 * time.Millisecond

func fits(actual, target winapi.Rect, tolerance float64) bool {
	off := func(value, want int32, size int32) bool {
		return math.Abs(float64(value-want)) > math.Max(12, tolerance*float64(size))
	}
	return !off(actual.X, target.X, target.Width) && !off(actual.Y, target.Y, target.Height) &&
		!off(actual.Width, target.Width, target.Width) && !off(actual.Height, target.Height, target.Height)
}

func adminHint(slot windoworganizer.Slot, message string) string {
	if slot.Elevated {
		return message + " ; application administrateur : lancer deskctl en administrateur"
	}
	return message
}

// Native sets exact rectangles with SetWindowPos. When FancyWM keeps re-tiling a
// window, the window is switched to FancyWM floating mode and placed again.
func Native(desktop Desktop, fancy Fancy, job Job) []Outcome {
	outcomes := make([]Outcome, 0, len(job.Layout.Slots))
	for _, slot := range job.Layout.Slots {
		outcome := Outcome{WindowID: slot.WindowID, Title: slot.Title, Engine: "native", Status: "success", Message: "Position verifiee"}
		if err := desktop.SetRect(slot.WindowID, slot.Rect); err != nil {
			outcome.Status, outcome.Message = "failed", adminHint(slot, err.Error())
		}
		outcomes = append(outcomes, outcome)
	}
	desktop.Wait(settle)
	for index, slot := range job.Layout.Slots {
		if outcomes[index].Status != "success" {
			continue
		}
		actual, err := desktop.Rect(slot.WindowID)
		if err == nil && fits(actual, slot.Rect, 0) {
			continue
		}
		if fancy != nil && fancy.Running() && desktop.Focus(slot.WindowID) == nil && fancy.Action("ToggleFloatingMode") == nil {
			desktop.Wait(settle)
			if desktop.SetRect(slot.WindowID, slot.Rect) == nil {
				desktop.Wait(settle)
				if actual, err = desktop.Rect(slot.WindowID); err == nil && fits(actual, slot.Rect, 0) {
					outcomes[index].Message = "Position verifiee (fenetre passee en flottant dans FancyWM)"
					continue
				}
			}
			_ = desktop.Focus(slot.WindowID)
			_ = fancy.Action("ToggleFloatingMode")
		}
		outcomes[index].Status, outcomes[index].Message = "uncertain", adminHint(slot, "Position non confirmee : la fenetre impose sa taille ou est geree par un autre outil")
	}
	return outcomes
}

type member struct {
	path string
	id   string
}

func ordered(slots []windoworganizer.Slot) []member {
	members := make([]member, 0, len(slots))
	for _, slot := range slots {
		members = append(members, member{slot.Path, slot.WindowID})
	}
	sort.SliceStable(members, func(left, right int) bool { return pathLess(members[left].path, members[right].path) })
	return members
}

func pathLess(left, right string) bool {
	a, b := strings.Split(left, "."), strings.Split(right, ".")
	for index := 0; index < len(a) && index < len(b); index++ {
		if len(a[index]) != len(b[index]) {
			return len(a[index]) < len(b[index])
		}
		if a[index] != b[index] {
			return a[index] < b[index]
		}
	}
	return len(a) < len(b)
}

func within(path, prefix string) bool {
	return prefix == "" || path == prefix || strings.HasPrefix(path, prefix+".")
}

// Fancy asks FancyWM to build the panels. FancyWM only resizes by 1/12 steps,
// so the result is checked with a 25 % tolerance; an error means "use Native".
func FancyWM(desktop Desktop, fancy Fancy, job Job) ([]Outcome, error) {
	switch {
	case fancy == nil || !fancy.Running():
		return nil, fmt.Errorf("FancyWM n'est pas lance")
	case job.FancyIndex == 0:
		return nil, fmt.Errorf("ecran sans indice FancyWM")
	case !job.OnCurrentDesktop:
		return nil, fmt.Errorf("fenetres hors du bureau courant")
	}
	for _, slot := range job.Layout.Slots {
		if slot.Elevated {
			return nil, fmt.Errorf("fenetre administrateur dans le groupe")
		}
	}
	members := ordered(job.Layout.Slots)
	run := func(id, action string) error {
		if err := desktop.Focus(id); err != nil {
			return err
		}
		if err := fancy.Action(action); err != nil {
			return err
		}
		desktop.Wait(150 * time.Millisecond)
		return nil
	}
	for _, item := range members {
		if err := run(item.id, fmt.Sprintf("MoveToDisplay%d", job.FancyIndex)); err != nil {
			return nil, err
		}
	}
	if job.Layout.Layout != nil {
		if err := buildGroup(run, *job.Layout.Layout, "", members); err != nil {
			return nil, err
		}
	}
	desktop.Wait(settle)
	outcomes := []Outcome{}
	for _, slot := range job.Layout.Slots {
		actual, err := desktop.Rect(slot.WindowID)
		target := slot.Rect
		if job.Layout.Layout == nil {
			target = job.Display
		}
		inside := err == nil && actual.X+actual.Width/2 >= job.Display.X && actual.X+actual.Width/2 <= job.Display.X+job.Display.Width &&
			actual.Y+actual.Height/2 >= job.Display.Y && actual.Y+actual.Height/2 <= job.Display.Y+job.Display.Height
		if !inside || (job.Layout.Layout != nil && !fits(actual, target, 0.25)) {
			return nil, fmt.Errorf("disposition FancyWM differente de la cible")
		}
		outcomes = append(outcomes, Outcome{WindowID: slot.WindowID, Title: slot.Title, Engine: "fancywm", Status: "success", Message: "Tuile par FancyWM (tailles approchees au 1/12)"})
	}
	return outcomes, nil
}

func buildGroup(run func(string, string) error, node windowrules.Node, path string, members []member) error {
	first := func(prefix string) string {
		for _, item := range members {
			if within(item.path, prefix) {
				return item.id
			}
		}
		return ""
	}
	anchor := first(path)
	if anchor == "" || node.Kind != "group" {
		return nil
	}
	panel, toward, grow, shrink := "CreateHorizontalPanel", "MoveLeft", "IncreaseWidth", "DecreaseWidth"
	if node.Axis == "v" {
		panel, toward, grow, shrink = "CreateVerticalPanel", "MoveUp", "IncreaseHeight", "DecreaseHeight"
	}
	if err := run(anchor, panel); err != nil {
		return err
	}
	present := []int{}
	for index := range node.Children {
		if first(join(path, index)) != "" {
			present = append(present, index)
		}
	}
	for _, index := range present {
		childPath := join(path, index)
		for _, item := range members {
			if within(item.path, childPath) && item.id != anchor {
				if err := run(item.id, toward); err != nil {
					return err
				}
			}
		}
	}
	total := 0.0
	for _, index := range present {
		total += node.Children[index].Weight
	}
	for _, index := range present[:max(len(present)-1, 0)] {
		share := node.Children[index].Weight / total
		steps := int(math.Round((share - 1/float64(len(present))) * 12))
		action := grow
		if steps < 0 {
			action, steps = shrink, -steps
		}
		for range steps {
			if err := run(first(join(path, index)), action); err != nil {
				return err
			}
		}
	}
	for _, index := range present {
		if err := buildGroup(run, node.Children[index], join(path, index), members); err != nil {
			return err
		}
	}
	return nil
}

func join(parent string, index int) string {
	if parent == "" {
		return fmt.Sprint(index)
	}
	return fmt.Sprintf("%s.%d", parent, index)
}

// Apply runs the configured engine: "native", "fancywm" (strict) or "auto"
// (FancyWM when possible, native otherwise or when FancyWM's result is off).
func Apply(engine string, desktop Desktop, fancy Fancy, job Job) []Outcome {
	zone := job.Layout.Bounds
	partial := zone.Width > 0 && zone.Height > 0 && zone != job.Display
	if engine == "native" || partial {
		outcomes := Native(desktop, fancy, job)
		if partial && engine != "native" {
			for index := range outcomes {
				outcomes[index].Message += " ; moteur natif requis pour la zone du groupe"
			}
		}
		return outcomes
	}
	outcomes, err := FancyWM(desktop, fancy, job)
	if err == nil {
		return outcomes
	}
	if engine == "fancywm" {
		failed := []Outcome{}
		for _, slot := range job.Layout.Slots {
			failed = append(failed, Outcome{WindowID: slot.WindowID, Title: slot.Title, Engine: "fancywm", Status: "failed", Message: "FancyWM : " + err.Error()})
		}
		return failed
	}
	outcomes = Native(desktop, fancy, job)
	for index := range outcomes {
		outcomes[index].Message += " (FancyWM ignore : " + err.Error() + ")"
	}
	return outcomes
}
