package windoworganizer

import (
	"errors"
	"os"
	"testing"

	"deskctl/internal/winapi"
	"deskctl/internal/windowrules"
)

const (
	a = "b8c5a6e2-7f10-4d3a-9c2b-1234567890ab"
	b = "12345678-1234-1234-1234-1234567890ab"
	c = "87654321-4321-4321-4321-ba0987654321"
)

type fakeBackend struct {
	inventory Inventory
	moves     int
	fail      bool
}

func (backend *fakeBackend) Inventory() (Inventory, error) { return backend.inventory, nil }
func (backend *fakeBackend) Move(window Window, target string) error {
	backend.moves++
	if backend.fail {
		return errors.New("HRESULT refused")
	}
	for index := range backend.inventory.Windows {
		if backend.inventory.Windows[index].ID == window.ID {
			backend.inventory.Windows[index].DesktopID = target
			backend.inventory.Windows[index].Pinned = target == winapi.AllDesktopsID
		}
	}
	return nil
}

type memoryJournal struct {
	journal Journal
	saves   int
	failAt  int
}

func (store *memoryJournal) LoadJournal() (Journal, error) {
	if store.journal.Version == 0 {
		return Journal{}, os.ErrNotExist
	}
	return store.journal, nil
}
func (store *memoryJournal) SaveJournal(journal Journal) error {
	store.saves++
	if store.failAt == store.saves {
		return errors.New("disk full")
	}
	store.journal = journal
	store.journal.Records = append([]Record{}, journal.Records...)
	return nil
}

func setup() (*Engine, *fakeBackend, *memoryJournal) {
	backend := &fakeBackend{inventory: Inventory{
		Desktops: []Desktop{{a, "Base"}, {b, "Dev"}, {c, "Web"}},
		Windows: []Window{
			{ID: "10", ProcessID: 1, Application: "Code", Title: "deskctl", Started: "start", Class: "editor", DesktopID: a},
			{ID: "11", ProcessID: 1, Application: "Code", Title: "Autre", Started: "start", Class: "editor", DesktopID: a},
		},
	}}
	store := &memoryJournal{}
	return New(backend, store), backend, store
}

func doc() windowrules.Document {
	return windowrules.Document{Version: 1, Rules: []windowrules.Rule{{
		ID: "r", Name: "Projet", Enabled: true, Application: "Code", Title: "deskctl", DesktopID: b,
	}}}
}

func TestPerWindowManualOverridesAndUndo(t *testing.T) {
	engine, backend, store := setup()
	plan, err := engine.Prepare(doc())
	if err != nil {
		t.Fatal(err)
	}
	if backend.moves != 0 || plan.Items[0].TargetID != b || plan.Items[1].Status != "unmatched" {
		t.Fatal("prepare must classify per window without moving")
	}
	staleID := plan.ID
	plan, err = engine.Assign(plan.ID, "11", c)
	if err != nil || plan.ID == staleID {
		t.Fatal("manual choice must produce new revision", err)
	}
	if _, err := engine.Apply(staleID); err == nil || backend.moves != 0 {
		t.Fatal("stale plan must not move")
	}
	plan, err = engine.SetRules(doc())
	if err != nil || plan.Items[1].TargetID != c || plan.Items[1].Source != "manual" {
		t.Fatal("rule edits must preserve manual choice", err)
	}
	report, err := engine.Apply(plan.ID)
	if err != nil || backend.moves != 2 || len(report.Results) != 2 || store.journal.Records[0].State != "applied" {
		t.Fatal("distinct windows of same process should move independently", err, report)
	}
	if _, err := engine.Apply(plan.ID); err == nil || backend.moves != 2 {
		t.Fatal("duplicate apply must be refused")
	}
	backend.inventory.Windows[1].DesktopID = b
	undo, err := engine.PreviewUndo()
	if err != nil || undo.Items[0].Status != "pending" || undo.Items[1].Status != "blocked" {
		t.Fatal("manual change after batch must prevent restoration", err, undo)
	}
	if _, err := engine.Apply(undo.ID); err != nil || backend.moves != 3 || backend.inventory.Windows[0].DesktopID != a || backend.inventory.Windows[1].DesktopID != b {
		t.Fatal("undo must restore only unchanged window", err)
	}
}

func TestConflictsAndMissingTargets(t *testing.T) {
	engine, _, _ := setup()
	rules := doc()
	second := rules.Rules[0]
	second.ID, second.Name, second.DesktopID = "r2", "Second", c
	rules.Rules = append(rules.Rules, second)
	plan, _ := engine.Prepare(rules)
	if plan.Items[0].Status != "conflict" || plan.Items[0].TargetID != "" {
		t.Fatal("conflicting destinations must block the window")
	}
	rules.Rules[1].DesktopID = b
	plan, _ = engine.Prepare(rules)
	if plan.Items[0].Status != "pending" {
		t.Fatal("same destination must not conflict")
	}
	rules.Rules[1].DesktopID = "00000000-0000-0000-0000-000000000001"
	plan, _ = engine.Prepare(rules)
	if plan.Items[0].Status != "conflict" {
		t.Fatal("missing desktop must be visible")
	}
}

func TestPinAndUnpinWindowWithUndo(t *testing.T) {
	engine, backend, _ := setup()
	plan, err := engine.Prepare(windowrules.Empty())
	if err != nil {
		t.Fatal(err)
	}
	plan, err = engine.Assign(plan.ID, "10", winapi.AllDesktopsID)
	if err != nil || plan.Items[0].Status != "pending" {
		t.Fatal("pinning should be prepared as a reversible action", err)
	}
	if _, err := engine.Apply(plan.ID); err != nil || !backend.inventory.Windows[0].Pinned {
		t.Fatal("window should be pinned to all desktops", err)
	}

	undo, err := engine.PreviewUndo()
	if err != nil || undo.Items[0].Status != "pending" || undo.Items[0].TargetID != a {
		t.Fatal("undo should offer the original desktop", err)
	}
	if _, err := engine.Apply(undo.ID); err != nil || backend.inventory.Windows[0].Pinned || backend.inventory.Windows[0].DesktopID != a {
		t.Fatal("undo should unpin and restore the original desktop", err)
	}

	plan, err = engine.Prepare(windowrules.Empty())
	if err != nil {
		t.Fatal(err)
	}
	plan, err = engine.Assign(plan.ID, "10", winapi.AllDesktopsID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Apply(plan.ID); err != nil {
		t.Fatal(err)
	}
	plan, err = engine.Prepare(windowrules.Empty())
	if err != nil {
		t.Fatal(err)
	}
	plan, err = engine.Assign(plan.ID, "10", b)
	if err != nil || plan.Items[0].Status != "pending" {
		t.Fatal("unpinning to a real desktop should be prepared", err)
	}
	if _, err := engine.Apply(plan.ID); err != nil || backend.inventory.Windows[0].Pinned || backend.inventory.Windows[0].DesktopID != b {
		t.Fatal("unpin should move the window to the selected desktop", err)
	}
}

func TestObsoleteWindowAndJournalFailureNeverMove(t *testing.T) {
	for _, mutate := range []func(*Window){
		func(w *Window) { w.Started = "recycled" },
		func(w *Window) { w.Class = "other" },
		func(w *Window) { w.DesktopID = c },
		func(w *Window) { w.Blocked = "pinned" },
	} {
		engine, backend, _ := setup()
		plan, _ := engine.Prepare(doc())
		mutate(&backend.inventory.Windows[0])
		report, err := engine.Apply(plan.ID)
		if err != nil || backend.moves != 0 || report.Results[0].Status != "ignored" {
			t.Fatal("obsolete window must stay untouched", err)
		}
	}
	for _, failAt := range []int{1, 2, 3} {
		engine, backend, store := setup()
		plan, _ := engine.Prepare(doc())
		store.failAt = failAt
		report, err := engine.Apply(plan.ID)
		if err == nil || report.Recovery == nil {
			t.Fatal("journal failure must expose recovery state")
		}
		want := 0
		if failAt == 3 {
			want = 1
		}
		if backend.moves != want {
			t.Fatalf("journal failure %d: unexpected moves %d", failAt, backend.moves)
		}
	}
}

func TestUncertainMovementAndModifiedUndoJournal(t *testing.T) {
	engine, backend, store := setup()
	plan, _ := engine.Prepare(doc())
	backend.fail = true
	report, err := engine.Apply(plan.ID)
	if err != nil || report.Results[0].Status != "uncertain" || store.journal.Records[0].State != "moving" {
		t.Fatal("failed movement must not claim success", err)
	}
	next, _ := engine.Prepare(doc())
	saves := store.saves
	if report, err := engine.Apply(next.ID); err == nil || report.Recovery == nil || backend.moves != 1 || store.saves != saves {
		t.Fatal("new batch must never overwrite uncertain recovery data")
	}
	undo, err := engine.PreviewUndo()
	if err != nil || undo.Items[0].Status != "blocked" {
		t.Fatal("uncertain movement must not be blindly undone", err)
	}
	backend.fail = false
	engine, backend, store = setup()
	plan, _ = engine.Prepare(doc())
	engine.Apply(plan.ID)
	undo, _ = engine.PreviewUndo()
	store.journal.Records[0].State = "restored"
	if _, err := engine.Apply(undo.ID); err == nil || backend.moves != 1 {
		t.Fatal("changed journal must invalidate undo")
	}
}

func TestDynamicTitleDoesNotBlockMove(t *testing.T) {
	engine, backend, _ := setup()
	plan, _ := engine.Prepare(doc())
	backend.inventory.Windows[0].Title = "deskctl (3 notifications)"
	report, err := engine.Apply(plan.ID)
	if err != nil || backend.moves != 1 || report.Results[0].Status != "success" {
		t.Fatal("a title change must not make the window unmovable", err, report)
	}
}

func layoutSetup() (*Engine, *fakeBackend) {
	engine, backend, _ := setup()
	backend.inventory.Displays = []winapi.DisplayInfo{
		{ID: "L", Name: "Gauche", Width: 1000, Height: 800, Primary: true, Work: winapi.Rect{Width: 1000, Height: 800}},
		{ID: "R", Name: "Droite", X: 1000, Width: 2000, Height: 1000, Work: winapi.Rect{X: 1000, Width: 2000, Height: 1000}},
	}
	backend.inventory.Windows = append(backend.inventory.Windows,
		Window{ID: "12", ProcessID: 2, Application: "msedge", Title: "Docs", Started: "s", Class: "browser", DesktopID: b, DisplayID: "L", Bounds: winapi.Rect{Width: 500, Height: 400}})
	engine.SetGap(0)
	return engine, backend
}

func groupDoc() windowrules.Document {
	return windowrules.Document{Version: 2, Rules: []windowrules.Rule{{
		ID: "g", Name: "Dev cote a cote", Enabled: true, DesktopID: b, DisplayID: "R",
		Layout: &windowrules.Node{Kind: "group", Axis: "h", Children: []windowrules.Node{
			{Kind: "window", Weight: 70, Application: "^code$", Title: "deskctl"},
			{Kind: "window", Weight: 30, Application: "edge"},
		}},
	}}}
}

func TestLayoutPlanTilesGroupOnTargetDisplay(t *testing.T) {
	engine, _ := layoutSetup()
	plan, err := engine.Prepare(groupDoc())
	if err != nil || len(plan.Layouts) != 1 {
		t.Fatal("layout must be planned", err)
	}
	layout := plan.Layouts[0]
	rects := map[string]winapi.Rect{}
	for _, slot := range layout.Slots {
		rects[slot.WindowID] = slot.Rect
	}
	if layout.Status != "pending" || layout.DisplayID != "R" || rects["10"] != (winapi.Rect{X: 1000, Width: 1400, Height: 1000}) ||
		rects["12"] != (winapi.Rect{X: 2400, Width: 600, Height: 1000}) {
		t.Fatal("group must be tiled 70/30 on the target display", layout)
	}
	report, err := engine.Apply(plan.ID)
	if err != nil || len(report.Layouts) != 1 {
		t.Fatal("apply must hand the layouts over for tiling", err)
	}
}

func TestOverlappingGeometryIsAConflictAndApplyFiltersByRule(t *testing.T) {
	engine, backend := layoutSetup()
	rules := groupDoc()
	rules.Rules = append(rules.Rules,
		windowrules.Rule{ID: "s", Name: "Edge a gauche", Enabled: true, Application: "edge", DesktopID: c, DisplayID: "L"},
		windowrules.Rule{ID: "o", Name: "Autre", Enabled: true, Application: "code", Title: "autre", DesktopID: c})
	plan, err := engine.Prepare(rules)
	if err != nil || plan.Items[2].Status != "conflict" {
		t.Fatal("a window claimed by two layouts must be a visible conflict", err, plan.Items[2])
	}
	report, err := engine.Apply(plan.ID, "o")
	if err != nil || backend.moves != 1 || len(report.Results) != 1 || backend.inventory.Windows[1].DesktopID != c || len(report.Layouts) != 0 {
		t.Fatal("apply must be limited to the selected rule", err, report)
	}
	plan, _ = engine.Prepare(windowrules.Empty())
	if _, err := engine.Apply(plan.ID); !errors.Is(err, ErrNothing) {
		t.Fatal("nothing to move must be reported with ErrNothing", err)
	}
}
