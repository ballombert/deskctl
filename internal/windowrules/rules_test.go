package windowrules

import "testing"

const desktop = "b8c5a6e2-7f10-4d3a-9c2b-1234567890ab"

func TestRulesCombineCriteriaAndNormalize(t *testing.T) {
	doc, err := Normalize(Document{Version: 1, Rules: []Rule{{
		ID: "r", Name: "Projet", Enabled: true, Application: " Code ", Title: " DESKCTL ", DesktopID: "{B8C5A6E2-7F10-4D3A-9C2B-1234567890AB}",
	}}})
	if err != nil {
		t.Fatal(err)
	}
	rule := doc.Rules[0]
	if rule.DesktopID != desktop || !rule.Matches("code", "mon deskctl - Code") || rule.Matches("chrome", "deskctl") || rule.Matches("Code", "autre") {
		t.Fatal("conditions must combine, ignore case and normalize GUID")
	}
	rule.Application = ""
	if !rule.Matches("chrome", "deskctl") {
		t.Fatal("empty application must not filter")
	}
	rule.Title = ""
	rule.Application = "code"
	if !rule.Matches("Code", "") {
		t.Fatal("application-only rule must match untitled windows")
	}
	rule.Enabled = false
	if rule.Matches("code", "deskctl") {
		t.Fatal("disabled rule matched")
	}
}

func TestRejectInvalidAndDuplicateRules(t *testing.T) {
	base := Rule{ID: "r", Name: "R", Application: "Code", DesktopID: desktop}
	for _, change := range []func(*Rule){
		func(r *Rule) { r.ID = "" },
		func(r *Rule) { r.Name = " " },
		func(r *Rule) { r.Application = "" },
		func(r *Rule) { r.DesktopID = "bad" },
	} {
		rule := base
		change(&rule)
		if _, err := Normalize(Document{Version: 1, Rules: []Rule{rule}}); err == nil {
			t.Fatal("invalid rule accepted", rule)
		}
	}
	if _, err := Normalize(Document{Version: 1, Rules: []Rule{base, base}}); err == nil {
		t.Fatal("duplicate ID accepted")
	}
	if _, err := Normalize(Document{Version: 3}); err == nil {
		t.Fatal("future schema accepted")
	}
}
