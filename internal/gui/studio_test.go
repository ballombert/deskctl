package gui

import (
	"testing"

	"deskctl/internal/winapi"
	"deskctl/internal/windoworganizer"
)

const desk = "b8c5a6e2-7f10-4d3a-9c2b-1234567890ab"

func studioPlan() windoworganizer.Plan {
	window := func(id, app, title string, x, width int32) windoworganizer.Item {
		return windoworganizer.Item{Window: windoworganizer.Window{ID: id, ProcessID: 1, Application: app, Title: title, Started: "s",
			DesktopID: desk, DisplayID: "D1", Bounds: winapi.Rect{X: x, Width: width, Height: 900}}}
	}
	return windoworganizer.Plan{Mode: "arrange", CurrentDesktopID: desk,
		Desktops: []windoworganizer.Desktop{{ID: desk, Name: "Dev"}},
		Displays: []winapi.DisplayInfo{{ID: "D1", Name: "D1", Width: 1600, Height: 900, Primary: true, Work: winapi.Rect{Width: 1600, Height: 900}}},
		Items: []windoworganizer.Item{
			window("3", "WindowsTerminal.exe", "pwsh", 1200, 400),
			window("1", "Code.exe", "deskctl", 0, 800),
			window("2", "Code.exe", "notes", 800, 400),
		}}
}

func TestCaptureRuleOrdersWindowsAndKeepsPercentages(t *testing.T) {
	rule, err := captureRule(studioPlan(), []string{"1", "2", "3"}, "h", "r-1")
	if err != nil {
		t.Fatal(err)
	}
	children := rule.Layout.Children
	if rule.DisplayID != "D1" || rule.DesktopID != desk || len(children) != 3 || children[0].Weight != 50 || children[2].Weight != 25 {
		t.Fatal("captured layout must follow screen order and sizes", rule)
	}
	if children[0].Application != `^Code\.exe$` || children[0].Title != "deskctl" || children[2].Title != "" {
		t.Fatal("titles are only used to tell apart windows of the same application", children)
	}
	preview, err := previewRule(studioPlan(), rule, 0)
	if err != nil || len(preview.Matches) != 3 || preview.Layout == nil || preview.Layout.Status != "unchanged" {
		t.Fatal("a captured layout must match its own windows in place", err, preview)
	}
}

func TestLayoutJobsSkipWindowsWhoseMoveFailed(t *testing.T) {
	plan := studioPlan()
	layout := windoworganizer.LayoutPlan{RuleID: "g", Status: "pending", DisplayID: "D1", DesktopID: desk,
		Slots: []windoworganizer.Slot{{WindowID: "1"}, {WindowID: "2"}}}
	report := windoworganizer.Report{Layouts: []windoworganizer.LayoutPlan{layout, {Status: "unchanged"}},
		Results: []windoworganizer.Result{{ID: "2", Status: "uncertain"}}}
	jobs := layoutJobs(plan, report, map[string]int{"D1": 2})
	if len(jobs) != 1 || len(jobs[0].Layout.Slots) != 1 || jobs[0].FancyIndex != 2 || !jobs[0].OnCurrentDesktop || jobs[0].Display.Width != 1600 {
		t.Fatal("unexpected jobs", jobs)
	}
	if undo := geometryUndo(jobs, plan); len(undo.Records) != 1 || undo.Records[0].Started != "s" {
		t.Fatal("geometry undo must keep window identity", undo)
	}
}
