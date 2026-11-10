package windowrules

import (
	"encoding/json"
	"math"
	"testing"

	"deskctl/internal/winapi"
)

func groupDocument() Document {
	return Document{Version: Version, Groups: []Group{{
		ID: "dev", Name: "Dev", Zone: &Zone{X: .25, Width: .5, Height: 1},
		Layout: Node{Kind: "group", Axis: "h", Children: []Node{{Kind: "window", Application: "^Code$", Weight: 1}}},
	}}, Rules: []Rule{
		{ID: "one", Name: "One", GroupID: "dev", Enabled: true, DesktopID: desktop},
		{ID: "two", Name: "Two", GroupID: "dev", Enabled: true, DesktopID: desktop},
	}}
}

func TestGroupReferencesRoundTripAndShareLayout(t *testing.T) {
	doc, err := Normalize(groupDocument())
	if err != nil {
		t.Fatal(err)
	}
	if !doc.Rules[0].Matches("Code", "dynamic title") || doc.Rules[0].EffectiveLayout() != doc.Rules[1].EffectiveLayout() {
		t.Fatal("rules must resolve one shared group")
	}
	encoded, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Document
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	decoded, err = Normalize(decoded)
	if err != nil || decoded.Rules[0].Layout != nil || decoded.Rules[0].EffectiveZone().X != .25 {
		t.Fatal("references lost on disk", err, decoded)
	}
	rect := decoded.Rules[0].EffectiveZone().Rect(winapi.Rect{X: -1920, Width: 1920, Height: 1000})
	if rect.X != -1440 || rect.Width != 960 || rect.Height != 1000 {
		t.Fatal("relative zone on negative monitor", rect)
	}
}

func TestGroupValidationRejectsOrphansAmbiguityAndInvalidZones(t *testing.T) {
	for _, mutate := range []func(*Document){
		func(doc *Document) { doc.Groups = nil },
		func(doc *Document) { doc.Groups = append(doc.Groups, doc.Groups[0]) },
		func(doc *Document) { doc.Rules[0].Application = "^Code$" },
		func(doc *Document) { doc.Rules[0].Layout = &doc.Groups[0].Layout },
		func(doc *Document) { doc.Groups[0].Zone.Width = 2 },
		func(doc *Document) { doc.Groups[0].Zone.X = math.NaN() },
		func(doc *Document) { doc.Rules[0].Zone = &Zone{} },
	} {
		doc := groupDocument()
		mutate(&doc)
		if _, err := Normalize(doc); err == nil {
			t.Fatal("invalid group accepted", doc)
		}
	}
}
