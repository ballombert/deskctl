package windowstore

import (
	"os"
	"path/filepath"
	"testing"

	"deskctl/internal/windowrules"
)

func TestPortableRulesReplaceAtomicallyAndRejectMalformed(t *testing.T) {
	store := Store{Directory: t.TempDir()}
	doc, err := store.LoadRules()
	if err != nil || doc.Version != windowrules.Version || len(doc.Rules) != 0 {
		t.Fatal("missing rules must start empty", err)
	}
	doc.Rules = append(doc.Rules, windowrules.Rule{
		ID: "r", Name: "Dev", Application: "Code", Enabled: true, DesktopID: "b8c5a6e2-7f10-4d3a-9c2b-1234567890ab",
	})
	if err := store.SaveRules(doc); err != nil {
		t.Fatal(err)
	}
	doc.Rules[0].Title = "deskctl"
	if err := store.SaveRules(doc); err != nil {
		t.Fatal("replacement on Windows must preserve a readable file", err)
	}
	loaded, err := store.LoadRules()
	if err != nil || loaded.Rules[0].Title != "deskctl" {
		t.Fatal("replacement not persisted", err)
	}
	path := filepath.Join(store.Directory, "window-rules.json")
	for _, text := range []string{`{"version":1,"rules":[],"unknown":true}`, `{"version":1,"rules":[]} {}`, `{"version":3}`, `{`} {
		if err := os.WriteFile(path, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := store.LoadRules(); err == nil {
			t.Fatal("malformed rules must not silently become empty", text)
		}
	}
}

func TestInaccessiblePortableDirectoryDoesNotFallback(t *testing.T) {
	store := Store{Directory: filepath.Join(t.TempDir(), "missing")}
	if err := store.SaveRules(windowrules.Empty()); err == nil {
		t.Fatal("missing portable directory must fail explicitly")
	}
}

func TestLegacyRulesAreBackedUpOnce(t *testing.T) {
	store := Store{Directory: t.TempDir()}
	path := filepath.Join(store.Directory, "window-rules.json")
	legacy := `{"version":1,"rules":[{"id":"r","name":"Dev","enabled":true,"application":"Code","title":"","desktopId":"b8c5a6e2-7f10-4d3a-9c2b-1234567890ab"}]}`
	if err := os.WriteFile(path, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	doc, err := store.LoadRules()
	if err != nil || doc.Version != windowrules.Version || doc.Rules[0].Application != "^Code$" {
		t.Fatal("v1 rules must migrate", err, doc)
	}

	backup, err := os.ReadFile(path + ".v1.bak")
	if err != nil || string(backup) != legacy {
		t.Fatal("v1 file must be backed up", err)
	}
}

func TestV2GroupsMigrateOnceAndBackupFailurePreservesSource(t *testing.T) {
	const legacy = `{"version":2,"rules":[{"id":"r","name":"Dev","enabled":true,"application":"","title":"","desktopId":"b8c5a6e2-7f10-4d3a-9c2b-1234567890ab","displayId":"D1","layout":{"kind":"group","axis":"h","weight":100,"children":[{"kind":"window","weight":100,"application":"^Code$"}]}}]}`
	store := Store{Directory: t.TempDir()}
	path := filepath.Join(store.Directory, "window-rules.json")
	if err := os.WriteFile(path, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".v2.bak", []byte("different source"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadRules(); err == nil {
		t.Fatal("unverified backup must stop migration")
	}
	source, _ := os.ReadFile(path)
	if string(source) != legacy {
		t.Fatal("migration failure changed original")
	}
	if err := os.Remove(path + ".v2.bak"); err != nil {
		t.Fatal(err)
	}
	doc, err := store.LoadRules()
	if err != nil || len(doc.Groups) != 1 || doc.Rules[0].GroupID != doc.Groups[0].ID || doc.Rules[0].DisplayID != "D1" {
		t.Fatal("layout migration", doc, err)
	}
	again, err := store.LoadRules()
	if err != nil || len(again.Groups) != 1 || again.Groups[0].ID != doc.Groups[0].ID {
		t.Fatal("migration not idempotent", again, err)
	}
	backup, err := os.ReadFile(path + ".v2.bak")
	if err != nil || string(backup) != legacy {
		t.Fatal("original not preserved", err)
	}
}
