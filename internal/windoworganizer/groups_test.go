package windoworganizer

import (
	"testing"

	"deskctl/internal/winapi"
	"deskctl/internal/windowrules"
)

func reusableGroup() windowrules.Document {
	return windowrules.Document{Version: windowrules.Version, Groups: []windowrules.Group{{
		ID: "dev", Name: "Dev", Layout: windowrules.Node{Kind: "group", Axis: "h", Weight: 100, Children: []windowrules.Node{
			{Kind: "window", Application: "^Code$", Weight: 100},
		}},
	}}, Rules: []windowrules.Rule{}}
}

func groupSetup() (*Engine, *fakeBackend) {
	engine, backend, _ := setup()
	backend.inventory.Displays = []winapi.DisplayInfo{
		{ID: "D1", Primary: true, Work: winapi.Rect{Width: 1600, Height: 900}},
		{ID: "D2", Work: winapi.Rect{X: -2000, Width: 2000, Height: 1000}},
	}
	backend.inventory.CurrentDesktopID = a
	for index := range backend.inventory.Windows {
		backend.inventory.Windows[index].DisplayID = "D1"
		backend.inventory.Windows[index].Bounds = winapi.Rect{X: int32(index * 800), Width: 800, Height: 900}
	}
	return engine, backend
}

func TestGroupManualOverridePreparesOneRevisionAndPreservesRules(t *testing.T) {
	engine, backend := groupSetup()
	doc := reusableGroup()
	doc.Rules = []windowrules.Rule{{ID: "r", Name: "R", GroupID: "dev", Enabled: true, DesktopID: a, DisplayID: "D1"}}
	plan, err := engine.Prepare(doc)
	if err != nil {
		t.Fatal(err)
	}
	before := plan.ID
	zone := windowrules.Zone{X: .25, Width: .5, Height: 1}
	plan, err = engine.AssignGroup(plan.ID, "dev", b, "D2", zone)
	if err != nil || plan.ID == before || backend.moves != 0 {
		t.Fatal("must prepare one revision, no movement", plan, err)
	}
	for _, item := range plan.Items {
		if item.TargetID != b || item.Status != "pending" {
			t.Fatal("whole group must move", item)
		}
	}
	if engine.rules.Rules[0].DesktopID != a || plan.Groups[0].Bounds.X != -1500 ||
		plan.Groups[0].Bounds.Width != 1000 || plan.Layouts[0].GroupID != "dev" {
		t.Fatal("zone or original rules changed", plan)
	}
	if _, err := engine.AssignGroup(before, "dev", a, "D1", zone); err == nil {
		t.Fatal("old preview accepted")
	}
}

func TestEquivalentGroupRulesDeduplicateButContradictionsBlock(t *testing.T) {
	engine, _ := groupSetup()
	doc := reusableGroup()
	doc.Rules = []windowrules.Rule{
		{ID: "r1", Name: "One", GroupID: "dev", Enabled: true, DesktopID: b, DisplayID: "D1"},
		{ID: "r2", Name: "Two", GroupID: "dev", Enabled: true, DesktopID: b, DisplayID: "D1"},
	}
	plan, err := engine.Prepare(doc)
	if err != nil || len(plan.Layouts) != 1 || len(plan.Layouts[0].RuleIDs) != 2 || plan.Items[0].Status != "pending" {
		t.Fatal("equivalent references must be one job", err, plan)
	}
	report, err := engine.Apply(plan.ID, "r2")
	if err != nil || len(report.Layouts) != 1 {
		t.Fatal("alias rule must select group job", report, err)
	}
	doc.Rules[1].DesktopID = c
	plan, err = engine.Prepare(doc)
	if err != nil || plan.Items[0].Status != "conflict" || plan.Groups[0].Status != "conflict" {
		t.Fatal("contradictory group rules must conflict", err, plan)
	}
}

func TestGroupWithoutRuleCanBeMovedAndPinned(t *testing.T) {
	engine, _ := groupSetup()
	plan, err := engine.Prepare(reusableGroup())
	if err != nil || len(plan.Groups) != 1 || len(plan.Groups[0].Members) != 2 || len(plan.Layouts) != 0 {
		t.Fatal("independent groups must be visible but not applied", err, plan)
	}
	plan, err = engine.AssignGroup(plan.ID, "dev", winapi.AllDesktopsID, "D1", windowrules.Zone{Width: 1, Height: 1})
	if err != nil || len(plan.Layouts) != 1 || !plan.Layouts[0].Pinned {
		t.Fatal("manual group pin", err, plan)
	}
	report, err := engine.Apply(plan.ID)
	if err != nil || report.Results[0].Status != "success" {
		t.Fatal(report, err)
	}
}

func TestInvalidGroupPlacementBlocksEveryMember(t *testing.T) {
	for _, issue := range []string{"missing screen", "tiny zone", "blocked peer"} {
		t.Run(issue, func(t *testing.T) {
			engine, backend := groupSetup()
			doc := reusableGroup()
			doc.Rules = []windowrules.Rule{{ID: "r", Name: "Work", GroupID: "dev", Enabled: true,
				DesktopID: b, DisplayID: "D1"}}
			switch issue {
			case "missing screen":
				doc.Rules[0].DisplayID = "missing"
			case "tiny zone":
				doc.Rules[0].Zone = &windowrules.Zone{Width: .001, Height: .001}
			case "blocked peer":
				backend.inventory.Windows[1].Blocked = "access denied"
			}
			plan, err := engine.Prepare(doc)
			if err != nil || plan.Items[0].Status != "conflict" || plan.Groups[0].Status != "conflict" {
				t.Fatal("invalid group must not prepare a partial desktop move", plan, err)
			}
			_, _ = engine.Apply(plan.ID)
			if backend.moves != 0 {
				t.Fatal("invalid group moved a member", backend.moves)
			}
		})
	}
}
