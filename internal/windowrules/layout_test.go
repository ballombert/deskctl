package windowrules

import "testing"

func TestLegacyRulesMigrateToEscapedRegex(t *testing.T) {
	doc, err := Normalize(Document{Version: 1, Rules: []Rule{{
		ID: "r", Name: "R", Enabled: true, Application: "C++", Title: "a.b", DesktopID: desktop,
	}}})
	if err != nil || doc.Version != Version {
		t.Fatal("v1 must migrate", err)
	}
	rule := doc.Rules[0]
	if !rule.Matches("c++", "x a.b y") || rule.Matches("C++X", "a.b") || rule.Matches("C++", "axb") {
		t.Fatal("migration must keep exact application and literal title", rule)
	}
}

func TestLayoutRulesNormalizeWeightsAndAssignSlots(t *testing.T) {
	layout := Node{Kind: "group", Axis: "h", Children: []Node{
		{Kind: "window", Weight: 3, Application: "^code$"},
		{Kind: "group", Axis: "v", Weight: 1, Children: []Node{
			{Kind: "window", Application: "chrome|msedge"},
			{Kind: "window", Title: "terminal"},
		}},
	}}
	doc, err := Normalize(Document{Version: Version, Rules: []Rule{{ID: "g", Name: "G", Enabled: true, DesktopID: desktop, Layout: &layout}}})
	if err != nil {
		t.Fatal(err)
	}
	root := doc.Rules[0].EffectiveLayout()
	if root.Children[0].Weight != 75 || root.Children[1].Weight != 25 || root.Children[1].Children[0].Weight != 50 {
		t.Fatal("weights must be normalized to percent", root)
	}
	slots := doc.Rules[0].Slots([]Candidate{
		{ID: "1", Application: "Code", Title: "x"},
		{ID: "2", Application: "msedge", Title: "Terminal docs"},
		{ID: "3", Application: "WindowsTerminal", Title: "Terminal"},
		{ID: "4", Application: "notepad", Title: "x"},
	})
	if len(slots["0"]) != 1 || slots["1.0"][0] != "2" || slots["1.1"][0] != "3" || len(slots) != 3 {
		t.Fatal("windows must go to the first matching leaf", slots)
	}
	if !doc.Rules[0].Matches("code", "") || doc.Rules[0].Matches("notepad", "x") {
		t.Fatal("layout rule must match through its leaves")
	}
}

func TestRejectInvalidLayouts(t *testing.T) {
	for _, layout := range []Node{
		{Kind: "window", Application: "x"},
		{Kind: "group", Axis: "d", Children: []Node{{Kind: "window", Application: "x"}}},
		{Kind: "group", Axis: "h"},
		{Kind: "group", Axis: "h", Children: []Node{{Kind: "window"}}},
		{Kind: "group", Axis: "h", Children: []Node{{Kind: "window", Application: "("}}},
		{Kind: "group", Axis: "h", Children: []Node{{Kind: "window", Application: "x", Weight: -1}}},
	} {
		layout := layout
		if _, err := Normalize(Document{Version: Version, Rules: []Rule{{ID: "g", Name: "G", DesktopID: desktop, Layout: &layout}}}); err == nil {
			t.Fatal("invalid layout accepted", layout)
		}
	}
}
