package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"deskctl/internal/desktopfiles"
	"deskctl/internal/vdesktop"
	"deskctl/internal/winapi"
)

const testDesktopGUID = "b8c5a6e2-7f10-4d3a-9c2b-1234567890ab"

func writeFixture(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func filePreview(t *testing.T) (preview, settings, string) {
	t.Helper()
	directory := t.TempDir()
	desktop := filepath.Join(directory, "Desktop")
	if err := os.Mkdir(desktop, 0700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(desktop, "rapport.txt")
	if err := os.WriteFile(source, []byte("contenu"), 0600); err != nil {
		t.Fatal(err)
	}
	config := settings{
		log:  filepath.Join(directory, "undo.json"),
		scan: filepath.Join(directory, "scan.json"),
		plan: filepath.Join(directory, "plan.json"),
	}
	prepared, err := prepareFiles(config, desktop)
	if err != nil {
		t.Fatal(err)
	}
	return prepared, config, source
}

func TestBuildLayoutPreviewValidatesWindowsAndDesktops(t *testing.T) {
	config := settings{layout: "layout.json", windows: "windows.json"}
	windows := []winapi.WindowInfo{
		{ProcessId: 123, ProcessName: "Code", WindowTitle: "Projet", WindowID: 1},
		{ProcessId: 456, ProcessName: "zen", WindowTitle: "Documentation", WindowID: 2},
	}
	desktops := []winapi.VirtualDesktopInfo{
		{Index: 0, ID: testDesktopGUID, Name: "Dev"},
		{Index: 1, ID: "12345678-1234-1234-1234-1234567890ab", Name: "Web"},
	}
	prepared, err := buildLayoutPreview(config, windows, desktops)
	if err != nil {
		t.Fatal(err)
	}
	job := prepared.data.(work)
	if job.action != createLayoutAction || len(job.windows) != 2 || len(job.desktops) != 2 {
		t.Fatal("layout draft must retain windows and available desktops")
	}
	if _, err := buildLayoutPreview(config, nil, desktops); err == nil {
		t.Fatal("empty window inventory must be rejected")
	}
	if _, err := buildLayoutPreview(config, windows, nil); err == nil {
		t.Fatal("empty desktop list must be rejected")
	}
	if _, err := buildLayoutPreview(config, []winapi.WindowInfo{windows[0], {ProcessId: 123, ProcessName: "Code", WindowTitle: "Autre", WindowID: 3}}, desktops); err != nil {
		t.Fatal("multiple windows for one application must be accepted", err)
	}
	if _, err := buildLayoutPreview(config, []winapi.WindowInfo{windows[0], windows[0]}, desktops); err == nil {
		t.Fatal("duplicate HWNDs must be rejected")
	}
	if _, err := buildLayoutPreview(config, windows, []winapi.VirtualDesktopInfo{desktops[0], desktops[0]}); err == nil {
		t.Fatal("duplicate desktop IDs must be rejected")
	}
}

func TestLayoutExportWritesBothReadableFilesAndNeverOverwrites(t *testing.T) {
	directory := t.TempDir()
	config := settings{
		layout:  filepath.Join(directory, "layout.json"),
		windows: filepath.Join(directory, "windows.json"),
	}
	windows := []winapi.WindowInfo{{ProcessId: 123, ProcessName: "Code", WindowTitle: "Projet", WindowID: 1}}
	layout := []vdesktop.LayoutEntry{{ProcessId: 123, TargetDesktopID: testDesktopGUID}}
	prepared := preview{data: work{action: createLayoutAction, config: config, windows: windows, layout: layout}}
	if _, err := exportJSON(prepared); err != nil {
		t.Fatal(err)
	}
	layoutData, err := os.ReadFile(config.layout)
	if err != nil {
		t.Fatal(err)
	}
	windowsData, err := os.ReadFile(config.windows)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(layoutData), "\n  {") || !strings.Contains(string(windowsData), "\n  {") {
		t.Fatal("both JSON exports must be indented")
	}
	var exportedLayout []vdesktop.LayoutEntry
	var exportedWindows []winapi.WindowInfo
	if err := json.Unmarshal(layoutData, &exportedLayout); err != nil || len(exportedLayout) != 1 || exportedLayout[0] != layout[0] {
		t.Fatal("layout export does not match assigned process", err)
	}
	if err := json.Unmarshal(windowsData, &exportedWindows); err != nil || len(exportedWindows) != 1 || exportedWindows[0] != windows[0] {
		t.Fatal("inventory export does not match source inventory", err)
	}
	if _, err := exportJSON(prepared); err == nil {
		t.Fatal("existing exports must not be overwritten")
	}
	afterLayout, _ := os.ReadFile(config.layout)
	afterWindows, _ := os.ReadFile(config.windows)
	if string(afterLayout) != string(layoutData) || string(afterWindows) != string(windowsData) {
		t.Fatal("existing exports changed after overwrite rejection")
	}
}

func TestLayoutExportCleansFirstFileWhenSecondPathExists(t *testing.T) {
	directory := t.TempDir()
	config := settings{
		layout:  filepath.Join(directory, "layout.json"),
		windows: filepath.Join(directory, "windows.json"),
	}
	if err := os.WriteFile(config.windows, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	prepared := preview{data: work{
		action: createLayoutAction, config: config,
		windows: []winapi.WindowInfo{{ProcessId: 123, ProcessName: "Code", WindowID: 1}},
		layout:  []vdesktop.LayoutEntry{{ProcessId: 123, TargetDesktopID: testDesktopGUID}},
	}}
	if _, err := exportJSON(prepared); err == nil {
		t.Fatal("existing inventory path must reject the paired export")
	}
	if _, err := os.Stat(config.layout); !os.IsNotExist(err) {
		t.Fatal("failed paired export must clean up the newly created layout")
	}
	data, err := os.ReadFile(config.windows)
	if err != nil || string(data) != "keep" {
		t.Fatal("existing inventory file must remain unchanged", err)
	}
}

func TestFilesSimulationExecutionAndUndo(t *testing.T) {
	prepared, config, source := filePreview(t)
	job := prepared.data.(work)
	if len(job.plan) != 1 || job.plan[0].Status != "Planned" {
		t.Fatal("expected a planned file")
	}
	if _, err := apply(prepared, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatal("simulation changed source", err)
	}
	if _, err := os.Stat(config.log); !os.IsNotExist(err) {
		t.Fatal("simulation must not create an undo log")
	}
	result, err := apply(prepared, true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(result.lines, "\n"), "Journal sauvegarde et verifie") {
		t.Fatal("verified undo log missing from result")
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatal("real execution did not move source")
	}
	undo, err := prepareUndo(config, job.desktop)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := apply(undo, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatal("undo simulation restored a file")
	}
	if _, err := apply(undo, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatal("undo did not restore source", err)
	}
}

func TestExistingJournalBlocksMoves(t *testing.T) {
	prepared, config, source := filePreview(t)
	writeFixture(t, config.log, "journal precedent")
	if _, err := apply(prepared, true); err == nil {
		t.Fatal("must not overwrite an existing journal")
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatal("journal rejection must happen before moving")
	}
	data, _ := os.ReadFile(config.log)
	if string(data) != `"journal precedent"` {
		t.Fatal("previous journal was changed")
	}
}

func TestExportNeverOverwritesAndUsesPreparedPlan(t *testing.T) {
	prepared, config, _ := filePreview(t)
	result, err := exportJSON(prepared)
	if err != nil || len(result.lines) != 2 {
		t.Fatal("expected scan and plan exports", err)
	}
	before, _ := os.ReadFile(config.plan)
	if _, err := exportJSON(prepared); err == nil {
		t.Fatal("must not overwrite a previous export")
	}
	after, _ := os.ReadFile(config.plan)
	if string(before) != string(after) {
		t.Fatal("plan was overwritten")
	}
	var exported []desktopfiles.PlanItem
	if err := json.Unmarshal(after, &exported); err != nil || len(exported) != 1 {
		t.Fatal("invalid plan export", err)
	}
}

func TestInvalidRulesAndUnsafeUndo(t *testing.T) {
	prepared, config, _ := filePreview(t)
	job := prepared.data.(work)
	config.rules = filepath.Join(t.TempDir(), "rules.json")
	writeFixture(t, config.rules, "not rules")
	if _, err := prepareFiles(config, job.desktop); err == nil {
		t.Fatal("invalid custom rules must not silently use defaults")
	}
	writeFixture(t, config.log, desktopfiles.UndoLog{Entries: []desktopfiles.UndoEntry{{
		OriginalSource: filepath.Join(t.TempDir(), "outside.txt"),
		CurrentTarget:  filepath.Join(job.desktop, "Documents", "rapport.txt"),
	}}})
	if _, err := prepareUndo(config, job.desktop); err == nil {
		t.Fatal("undo outside the desktop must be rejected")
	}
}

func TestUndoRejectsModifiedJournal(t *testing.T) {
	prepared, config, _ := filePreview(t)
	if _, err := apply(prepared, true); err != nil {
		t.Fatal(err)
	}
	undo, err := prepareUndo(config, prepared.desktop)
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, config.log, desktopfiles.UndoLog{})
	if _, err := apply(undo, true); err == nil || !strings.Contains(err.Error(), "modifie depuis l'apercu") {
		t.Fatal("a modified journal must require another preview", err)
	}
}

func TestWindowPreviewValidationAndDesktopIDs(t *testing.T) {
	directory := t.TempDir()
	config := settings{
		layout:  filepath.Join(directory, "layout.json"),
		windows: filepath.Join(directory, "windows.json"),
	}
	writeFixture(t, config.layout, "not a layout")
	if _, err := prepareWindows(config); err == nil {
		t.Fatal("invalid layout accepted")
	}
	pid := os.Getpid()
	name, err := winapi.GetProcessName(uint32(pid))
	if err != nil {
		t.Fatal(err)
	}
	desktops, err := winapi.EnumerateVirtualDesktops()
	if err != nil || len(desktops) == 0 {
		t.Fatal("could not read a live desktop for the fixture", err)
	}
	writeFixture(t, config.layout, []vdesktop.LayoutEntry{{ProcessId: pid, TargetDesktopID: desktops[0].ID}})
	if _, err := prepareWindows(config); err == nil {
		t.Fatal("missing reference accepted")
	}
	writeFixture(t, config.windows, []winapi.WindowInfo{})
	if _, err := prepareWindows(config); err == nil {
		t.Fatal("missing PID reference accepted")
	}
	writeFixture(t, config.windows, []winapi.WindowInfo{{ProcessId: pid, ProcessName: name, DesktopID: desktops[0].ID, DesktopName: desktops[0].DisplayName(), WindowID: 1}})
	prepared, err := prepareWindows(config)
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, config.layout, []vdesktop.LayoutEntry{{ProcessId: pid, TargetDesktopID: "12345678-1234-1234-1234-1234567890ab"}})
	if _, err := prepareWindows(config); err == nil {
		t.Fatal("target GUID absent from live desktops must be rejected")
	}
	if _, err := apply(prepared, false); err != nil {
		t.Fatal("simulation must accept a valid GUID layout", err)
	}
}

func TestOneDriveNotice(t *testing.T) {
	if !strings.Contains(strings.Join(desktopNotice(`C:\OneDrive\Desktop`), "\n"), "ATTENTION OneDrive") {
		t.Fatal("OneDrive warning missing")
	}
}
