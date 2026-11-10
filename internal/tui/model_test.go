package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"deskctl/internal/winapi"
)

func press(model Model, code rune, text string) (Model, tea.Cmd) {
	next, command := model.Update(tea.KeyPressMsg{Code: code, Text: text})
	return next.(Model), command
}

func TestRealExecutionRequiresSimulationAndConfirmation(t *testing.T) {
	model := newModel(services{})
	model.screen = previewScreen
	for _, key := range []string{"a", "o"} {
		model, _ = press(model, rune(key[0]), key)
		if model.screen != previewScreen {
			t.Fatal("an overview must not allow real execution")
		}
	}
	model.screen = simulationScreen
	model, command := press(model, 'a', "a")
	if model.screen != confirmScreen || command != nil {
		t.Fatal("requesting execution must only open confirmation")
	}
	model, command = press(model, tea.KeyEnter, "")
	if model.screen != simulationScreen || command != nil {
		t.Fatal("Enter must decline confirmation")
	}
	model, _ = press(model, 'a', "a")
	model, command = press(model, 'o', "o")
	if model.screen != busyScreen || command == nil {
		t.Fatal("explicit confirmation must start a command")
	}
	for _, key := range []string{"o", "a", "q"} {
		model, command = press(model, rune(key[0]), key)
		if model.screen != busyScreen || command != nil {
			t.Fatal("busy operations must not restart or quit")
		}
	}
}

func TestNavigationSettingsAndErrors(t *testing.T) {
	model := newModel(services{})
	model, _ = press(model, tea.KeyDown, "")
	model, _ = press(model, tea.KeyEnter, "")
	if model.selected != exportWindowsAction || len(model.fields) != 1 {
		t.Fatal("window inventory settings not selected")
	}
	model, _ = press(model, 'q', "q")
	if model.screen != settingsScreen {
		t.Fatal("q must remain available in file paths")
	}
	model, _ = press(model, tea.KeyEsc, "")
	if model.screen != menuScreen {
		t.Fatal("Escape must return to menu")
	}
	model.screen = busyScreen
	next, _ := model.Update(preparedMsg{err: errors.New("layout.json absent")})
	model = next.(Model)
	if model.screen != errorScreen || !strings.Contains(model.content, "layout.json absent") {
		t.Fatal("preparation error must be visible")
	}
}

func TestLayoutAssignmentAndExportFlow(t *testing.T) {
	windows := []winapi.WindowInfo{
		{ProcessId: 123, ProcessName: "Code", WindowTitle: "Projet", WindowID: 1},
		{ProcessId: 123, ProcessName: "Code", WindowTitle: "Autre fenetre", WindowID: 2},
		{ProcessId: 456, ProcessName: "zen", WindowTitle: "Documentation", WindowID: 3},
	}
	prepared, err := buildLayoutPreview(settings{layout: "layout.json", windows: "windows.json"}, windows, []winapi.VirtualDesktopInfo{
		{Index: 0, ID: testDesktopGUID, Name: "Dev"},
		{Index: 1, ID: "12345678-1234-1234-1234-1234567890ab", Name: "Web"},
	})
	if err != nil {
		t.Fatal(err)
	}
	exported := false
	model := newModel(services{
		export: func(prepared preview) (report, error) {
			exported = true
			job := prepared.data.(work)
			if len(job.layout) != 2 || len(job.windows) != 3 || job.layout[0].TargetDesktopID != "12345678-1234-1234-1234-1234567890ab" || job.layout[1].TargetDesktopID != testDesktopGUID {
				t.Fatal("export must receive the selected desktop for every process")
			}
			return report{lines: []string{"layout enregistre"}}, nil
		},
	})
	model.selected = createLayoutAction
	model.screen = busyScreen
	next, _ := model.Update(preparedMsg{preview: prepared})
	model = next.(Model)
	if model.screen != layoutEditorScreen {
		t.Fatal("layout preparation must open the assignment editor")
	}
	if len(model.layoutWindows) != 2 || !strings.Contains(model.View().Content, "2 fenetre(s) detectee(s)") {
		t.Fatal("editor must group processes and show their complete window count")
	}
	next, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	model = next.(Model)
	model, _ = press(model, tea.KeyEnter, "")
	if model.layoutTargets[0] != "12345678-1234-1234-1234-1234567890ab" || model.layoutCursor != 1 {
		t.Fatal("Enter must assign the selected desktop and advance to next process")
	}
	next, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	model = next.(Model)
	model, _ = press(model, tea.KeyEnter, "")
	if model.screen != previewScreen || model.preview.data.(work).layout[1].TargetDesktopID != testDesktopGUID {
		t.Fatal("assigning all processes must create a non-moving layout preview")
	}
	model, command := press(model, 'e', "e")
	model = runOperation(t, model, command)
	if !exported || model.screen != previewScreen || model.status != "EXPORT TERMINE - aucun deplacement" {
		t.Fatal("layout preview must export without applying window moves")
	}
}

func TestNamedLayoutEditorFitsDesktopAndCompactTerminals(t *testing.T) {
	windows := []winapi.WindowInfo{
		{ProcessId: 123, ProcessName: "Code", WindowTitle: "Projet exemple - Visual Studio Code", DesktopID: testDesktopGUID, DesktopName: "Dev", WindowID: 1},
		{ProcessId: 456, ProcessName: "zen", WindowTitle: "Documentation technique", DesktopID: testDesktopGUID, DesktopName: "Dev", WindowID: 2},
	}
	desktops := []winapi.VirtualDesktopInfo{
		{Index: 0, ID: testDesktopGUID, Name: "Dev"},
		{Index: 1, ID: "12345678-1234-1234-1234-1234567890ab", Name: "Web"},
		{Index: 2, ID: "87654321-4321-4321-4321-ba0987654321", Name: "Web"},
	}
	prepared, err := buildLayoutPreview(settings{}, windows, desktops)
	if err != nil {
		t.Fatal(err)
	}
	model := newModel(services{})
	model.selected = createLayoutAction
	model.screen = busyScreen
	next, _ := model.Update(preparedMsg{preview: prepared})
	model = next.(Model)
	for _, width := range []int{32, 48, 72, 90, 120} {
		next, _ = model.Update(tea.WindowSizeMsg{Width: width, Height: 24})
		model = next.(Model)
		content := model.View().Content
		if lipgloss.Width(content) > width || lipgloss.Height(content) > 24 {
			t.Fatalf("layout view overflows terminal: width=%d rendered=%dx%d", width, lipgloss.Width(content), lipgloss.Height(content))
		}
		if !strings.Contains(content, "Dev") || !strings.Contains(content, "Web") {
			t.Fatalf("named desks missing at width %d", width)
		}
	}
}

func TestStyledMenuFitsNarrowTerminals(t *testing.T) {
	for _, width := range []int{32, 48, 80, 120} {
		model := newModel(services{})
		next, _ := model.Update(tea.WindowSizeMsg{Width: width, Height: 24})
		model = next.(Model)
		content := model.View().Content
		if lipgloss.Width(content) > width || lipgloss.Height(content) > 24 {
			t.Fatalf("menu overflows terminal: width=%d rendered=%dx%d", width, lipgloss.Width(content), lipgloss.Height(content))
		}
		if !strings.Contains(strings.ToLower(content), "disposition") {
			t.Fatalf("menu entry missing at width %d", width)
		}
	}
}

func TestResultsAndResize(t *testing.T) {
	for _, real := range []bool{false, true} {
		model := newModel(services{})
		model.screen = busyScreen
		next, _ := model.Update(finishedMsg{real: real, report: report{lines: []string{"Conflits : 1", "Succes : 2"}}})
		model = next.(Model)
		if real && model.screen != resultScreen || !real && model.screen != simulationScreen {
			t.Fatal("simulation and real results must be distinct")
		}
		for _, width := range []int{20, 32, 80} {
			next, _ = model.Update(tea.WindowSizeMsg{Width: width, Height: 20})
			model = next.(Model)
			_ = model.View()
			if model.viewport.Width() != width-4 {
				t.Fatal("viewport must follow terminal width")
			}
		}
	}
}

func runOperation(t *testing.T, model Model, command tea.Cmd) Model {
	t.Helper()
	if command == nil {
		t.Fatal("expected an operation")
	}
	message := command()
	if batch, ok := message.(tea.BatchMsg); ok {
		message = batch[0]()
	}
	next, _ := model.Update(message)
	return next.(Model)
}

func TestCompleteFlowWithInjectedOperations(t *testing.T) {
	var calls []bool
	model := newModel(services{
		prepare: func(selected action, config settings) (preview, error) {
			return preview{title: "Apercu", lines: []string{"Un fichier"}}, nil
		},
		apply: func(prepared preview, real bool) (report, error) {
			calls = append(calls, real)
			result := report{lines: []string{"Un fichier traite"}}
			if real {
				return result, errors.New("journal NON confirme")
			}
			return result, nil
		},
	})
	model, _ = press(model, tea.KeyEnter, "")
	model, command := press(model, tea.KeyEnter, "")
	model = runOperation(t, model, command)
	if model.screen != previewScreen || len(calls) != 0 {
		t.Fatal("preview must not apply anything")
	}
	model, command = press(model, 's', "s")
	model = runOperation(t, model, command)
	if model.screen != simulationScreen || len(calls) != 1 || calls[0] {
		t.Fatal("simulation must never pass execute=true")
	}
	model, _ = press(model, 'a', "a")
	model, _ = press(model, 'n', "n")
	if len(calls) != 1 || !strings.Contains(model.content, "Un fichier traite") {
		t.Fatal("declining must preserve the simulation without side effects")
	}
	model, _ = press(model, 'a', "a")
	model, command = press(model, 'o', "o")
	model = runOperation(t, model, command)
	if len(calls) != 2 || !calls[1] || model.screen != errorScreen {
		t.Fatal("confirmation must run once and expose partial failure")
	}
	if !strings.Contains(model.content, "Un fichier traite") || !strings.Contains(model.content, "journal NON confirme") {
		t.Fatal("partial results and recovery errors must both be retained")
	}
}

func TestNarrowViewFitsAndCanReachLastLine(t *testing.T) {
	for _, width := range []int{32, 48, 80, 120} {
		for _, current := range []screen{previewScreen, simulationScreen, confirmScreen, resultScreen, errorScreen} {
			model := newModel(services{})
			model.screen = current
			model.status = "SIMULATION TERMINEE - aucun deplacement"
			model.preview.title = "Annulation des fichiers uniquement"
			var lines []string
			for index := 0; index < 60; index++ {
				lines = append(lines, fmt.Sprintf("%d - Un chemin de fichier suffisamment long pour se replier", index))
			}
			model.setContent(strings.Join(append(lines, "DERNIERE LIGNE"), "\n"))
			next, _ := model.Update(tea.WindowSizeMsg{Width: width, Height: 24})
			model = next.(Model)
			model.viewport.GotoBottom()
			content := model.View().Content
			if lipgloss.Width(content) > width || lipgloss.Height(content) > 24 {
				t.Fatalf("view overflows: width=%d screen=%d actual=%dx%d", width, current, lipgloss.Width(content), lipgloss.Height(content))
			}
			if !strings.Contains(content, "DERNIERE LIGNE") {
				t.Fatalf("last line unreachable at width=%d screen=%d", width, current)
			}
		}
	}
}

func TestSmallTerminalCannotConfirm(t *testing.T) {
	model := newModel(services{})
	model.screen = confirmScreen
	next, _ := model.Update(tea.WindowSizeMsg{Width: 20, Height: 8})
	model, command := press(next.(Model), 'o', "o")
	if model.screen != confirmScreen || command != nil {
		t.Fatal("must not confirm when the confirmation is hidden")
	}
}

func TestTerminalControlSequencesAreRemoved(t *testing.T) {
	model := newModel(services{})
	model.setContent("titre\x1b[2J\x1b[31mtexte\x1b[0m\r\b\x07")
	if model.content != "titretexte" {
		t.Fatalf("untrusted terminal sequences retained: %q", model.content)
	}
}

func TestBubbleTeaProgramStartsAndQuits(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	program := tea.NewProgram(newModel(services{}), tea.WithInput(nil), tea.WithOutput(io.Discard), tea.WithoutRenderer(), tea.WithContext(ctx))
	done := make(chan error, 1)
	go func() {
		final, err := program.Run()
		if err == nil && final.(Model).cursor != 1 {
			err = fmt.Errorf("navigation message was not processed")
		}
		done <- err
	}()
	program.Send(tea.WindowSizeMsg{Width: 80, Height: 24})
	program.Send(tea.KeyPressMsg{Code: tea.KeyDown})
	program.Send(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
