package gui

import (
	"strings"
	"testing"

	"deskctl/internal/grouptracking"
	"deskctl/internal/winapi"
	"deskctl/internal/windoworganizer"
	"deskctl/internal/windowrules"
	"deskctl/internal/windowstore"
)

func groupApp(t *testing.T) *App {
	t.Helper()
	doc := windowrules.Document{Version: windowrules.Version, Rules: []windowrules.Rule{}, Groups: []windowrules.Group{{
		ID: "dev", Name: "Dev", Layout: windowrules.Node{Kind: "group", Axis: "h", Weight: 100, Children: []windowrules.Node{
			{Kind: "window", Application: "^Code\\.exe$", Weight: 100},
		}},
	}}}
	engine := windoworganizer.New(staticBackend{inventoryOf(studioPlan())}, noJournal{})
	plan, err := engine.Prepare(doc)
	if err != nil {
		t.Fatal(err)
	}
	normalized, err := windowrules.Normalize(doc)
	if err != nil {
		t.Fatal(err)
	}
	return &App{engine: engine, store: windowstore.Store{Directory: t.TempDir()},
		state: State{Plan: plan, Rules: normalized, Following: []string{}}}
}

func TestGroupBridgePreviewsAndPreparesWithoutWindowsCalls(t *testing.T) {
	app := groupApp(t)
	group := app.state.Rules.Groups[0]
	preview, err := app.PreviewGroup(group)
	if err != nil || len(preview.Matches) != 2 || preview.Layout == nil {
		t.Fatal("group preview must resolve members", preview, err)
	}
	before := app.state.Plan.ID
	state, err := app.AssignGroup(before, group.ID, winapi.AllDesktopsID, "D1",
		windowrules.Zone{X: .25, Width: .5, Height: 1})
	if err != nil || state.Plan.ID == before || len(state.Plan.Layouts) != 1 || !state.Plan.Layouts[0].Pinned {
		t.Fatal("group preparation", state, err)
	}
	if app.state.Rules.Rules == nil || len(app.state.Rules.Rules) != 0 {
		t.Fatal("manual assignment must not create persistent rules")
	}
}

func TestGroupBridgeRefusesDeletingReferencedGroupAndKeepsManualOverrides(t *testing.T) {
	app := groupApp(t)
	state, err := app.AssignGroup(app.state.Plan.ID, "dev", winapi.AllDesktopsID, "D1",
		windowrules.Zone{Width: 1, Height: 1})
	if err != nil {
		t.Fatal(err)
	}
	doc := app.state.Rules
	doc.Rules = []windowrules.Rule{{ID: "r", Name: "Work", GroupID: "dev", Enabled: true, DesktopID: desk}}
	state, err = app.SaveRules(doc, state.Plan.ID)
	if err != nil || state.Plan.Items[1].TargetID != winapi.AllDesktopsID {
		t.Fatal("saving rules must preserve manual group destination", state, err)
	}
	doc.Groups = nil
	if _, err := app.SaveRules(doc, state.Plan.ID); err == nil || !strings.Contains(err.Error(), "Work") {
		t.Fatal("referenced group deletion must name affected rule", err)
	}
	loaded, err := app.store.LoadRules()
	if err != nil || len(loaded.Groups) != 1 {
		t.Fatal("failed deletion must preserve persisted group", loaded, err)
	}
}

func TestGeometryJournalFailureAndUncertainStateBlockNewGestures(t *testing.T) {
	app := groupApp(t)
	before := grouptracking.Member{ID: "1", ProcessID: 1, Started: "s", Application: "Code", Class: "editor",
		DesktopID: desk, Rect: winapi.Rect{Width: 100, Height: 100}}
	after := before
	after.Rect.X = 50
	backend := trackingBackend{app}
	if err := backend.Save([]grouptracking.Member{before}, []grouptracking.Member{after}); err != nil {
		t.Fatal(err)
	}
	if err := app.geometryReady(); err == nil {
		t.Fatal("an unfinished gesture must block overwriting its journal")
	}
	if err := app.finishGeometry(true); err != nil {
		t.Fatal(err)
	}
	if err := app.geometryReady(); err == nil {
		t.Fatal("an uncertain gesture must block new gestures")
	}
	undo, err := app.store.LoadGeometry()
	if err != nil || undo.Records[0].After.X != 50 || undo.Records[0].Rect.X != 0 {
		t.Fatal("recovery positions missing", undo, err)
	}
}
