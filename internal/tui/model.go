package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"deskctl/internal/vdesktop"
	"deskctl/internal/winapi"
)

type action int

const (
	filesAction action = iota
	exportWindowsAction
	createLayoutAction
	windowsAction
	undoAction
)

var actions = []string{
	"Fichiers du Bureau",
	"Inventaire des fenetres",
	"Creer une disposition de fenetres",
	"Placement des fenetres",
	"Annuler un rangement de fichiers",
}

var (
	accentStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#50D5C8"))
	mutedStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("#8A9AA3"))
	selectedRowStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F4FBFA")).Background(lipgloss.Color("#15515A"))
	currentStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#F2C66D"))
	panelTitleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#B7E4DF"))
)

type screen int

const (
	menuScreen screen = iota
	settingsScreen
	busyScreen
	layoutEditorScreen
	previewScreen
	simulationScreen
	confirmScreen
	resultScreen
	errorScreen
)

type settings struct {
	rules, scan, plan, log, layout, windows string
}

type field struct {
	label string
	input textinput.Model
}

type preview struct {
	title   string
	lines   []string
	desktop string
	data    any
}

type report struct {
	lines []string
}

type services struct {
	prepare func(action, settings) (preview, error)
	apply   func(preview, bool) (report, error)
	export  func(preview) (report, error)
}

type preparedMsg struct {
	preview preview
	err     error
}

type finishedMsg struct {
	report report
	err    error
	real   bool
	export bool
}

type pulseMsg struct{}

type Model struct {
	services       services
	screen         screen
	selected       action
	cursor         int
	fields         []field
	focus          int
	width          int
	height         int
	viewport       viewport.Model
	preview        preview
	content        string
	simulation     string
	status         string
	pulse          int
	config         settings
	layoutWindows  []winapi.WindowInfo
	layoutDesktops []winapi.VirtualDesktopInfo
	layoutTargets  []string
	layoutCursor   int
	layoutChoice   int
}

func newModel(operations services) Model {
	return Model{
		services: operations,
		width:    80, height: 24,
		viewport: viewport.New(viewport.WithWidth(76), viewport.WithHeight(14)),
		config: settings{
			rules: "rules.json", scan: "desktop-files.json", plan: "desktop-plan.json",
			log: "undo-log.json", layout: "layout.json", windows: "windows.json",
		},
	}
}

func (model Model) Init() tea.Cmd { return nil }

func pulse() tea.Cmd {
	return tea.Tick(150*time.Millisecond, func(time.Time) tea.Msg { return pulseMsg{} })
}

func (model *Model) setContent(content string) {
	model.content = strings.Map(func(character rune) rune {
		if character < 32 && character != '\n' && character != '\t' || character >= 127 && character <= 159 {
			return -1
		}
		return character
	}, ansi.Strip(content))
	model.viewport.SetContent(ansi.Hardwrap(model.content, max(1, model.width-4), true))
	model.viewport.GotoTop()
}

func (model *Model) begin(status string, command tea.Cmd) tea.Cmd {
	model.screen = busyScreen
	model.status = status
	model.pulse = 0
	model.fields = nil
	return tea.Batch(command, pulse())
}

func (model *Model) openSettings() tea.Cmd {
	model.screen = settingsScreen
	model.focus = 0
	model.fields = nil
	add := func(label, value string) {
		input := textinput.New()
		input.SetValue(value)
		input.SetWidth(max(1, model.width-8))
		input.SetVirtualCursor(true)
		model.fields = append(model.fields, field{label: label, input: input})
	}
	switch model.selected {
	case filesAction:
		add("Regles (vide = classement par defaut)", model.config.rules)
		add("Export du scan JSON", model.config.scan)
		add("Export du plan JSON", model.config.plan)
		add("Nouveau journal d'annulation", model.config.log)
	case exportWindowsAction:
		add("Export de l'inventaire JSON", model.config.windows)
	case createLayoutAction:
		add("Nouveau layout JSON", model.config.layout)
		add("Nouvel inventaire JSON", model.config.windows)
	case windowsAction:
		add("Disposition JSON existante", model.config.layout)
		add("Inventaire JSON de reference (obligatoire)", model.config.windows)
	case undoAction:
		add("Journal d'annulation existant", model.config.log)
	}
	return model.fields[0].input.Focus()
}

func (model *Model) readSettings() {
	value := func(index int) string { return strings.TrimSpace(model.fields[index].input.Value()) }
	switch model.selected {
	case filesAction:
		model.config.rules, model.config.scan = value(0), value(1)
		model.config.plan, model.config.log = value(2), value(3)
	case exportWindowsAction:
		model.config.windows = value(0)
	case createLayoutAction:
		model.config.layout, model.config.windows = value(0), value(1)
	case windowsAction:
		model.config.layout, model.config.windows = value(0), value(1)
	case undoAction:
		model.config.log = value(0)
	}
}

func (model Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	next, command := model.update(message)
	updated := next.(Model)
	headingHeight := lipgloss.Height(ansi.Hardwrap(updated.heading(), max(1, updated.width-4), true)) - 1
	updated.viewport.SetHeight(max(1, updated.bodyHeight()-headingHeight))
	return updated, command
}

func (model Model) update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		model.width, model.height = message.Width, message.Height
		model.viewport.SetWidth(max(1, model.width-4))
		model.viewport.SetContent(ansi.Hardwrap(model.content, max(1, model.width-4), true))
		for index := range model.fields {
			model.fields[index].input.SetWidth(max(1, model.width-8))
		}
		return model, nil
	case pulseMsg:
		if model.screen == busyScreen {
			model.pulse++
			return model, pulse()
		}
		return model, nil
	case preparedMsg:
		if model.screen != busyScreen {
			return model, nil
		}
		if message.err != nil {
			model.screen = errorScreen
			model.status = "ERREUR DE PREPARATION"
			model.setContent(message.err.Error())
		} else {
			model.preview = message.preview
			if model.selected == createLayoutAction {
				job, ok := message.preview.data.(work)
				if !ok {
					model.screen = errorScreen
					model.status = "ERREUR DE PREPARATION"
					model.setContent("brouillon de disposition absent")
					return model, nil
				}
				model.layoutWindows = uniqueProcessWindows(job.windows)
				model.layoutDesktops = job.desktops
				model.layoutTargets = make([]string, len(model.layoutWindows))
				model.layoutCursor = 0
				model.layoutChoice = 0
				model.screen = layoutEditorScreen
				model.status = "ATTRIBUTION DES BUREAUX"
				model.setContent(model.layoutEditorContent())
			} else {
				model.screen = previewScreen
				model.status = "APERCU - aucun deplacement"
				model.setContent(strings.Join(message.preview.lines, "\n"))
			}
		}
		return model, nil
	case finishedMsg:
		if model.screen != busyScreen {
			return model, nil
		}
		if message.err != nil {
			model.screen = errorScreen
			model.status = "ERREUR"
			model.setContent(strings.Join(message.report.lines, "\n") + "\n" + message.err.Error())
		} else if message.export {
			model.screen = previewScreen
			model.status = "EXPORT TERMINE - aucun deplacement"
			model.setContent(strings.Join(append(message.report.lines, model.preview.lines...), "\n"))
		} else {
			model.screen = resultScreen
			model.status = "EXECUTION TERMINEE - consulter le bilan"
			if !message.real {
				model.screen = simulationScreen
				model.status = "SIMULATION TERMINEE - aucun deplacement"
				model.simulation = strings.Join(message.report.lines, "\n")
			}
			model.setContent(strings.Join(message.report.lines, "\n"))
		}
		return model, nil
	case tea.KeyPressMsg:
		key := message.String()
		if model.screen == busyScreen {
			return model, nil
		}
		if key == "ctrl+c" || (key == "q" && model.screen != settingsScreen) {
			return model, tea.Quit
		}
		if key == "esc" {
			if model.screen == confirmScreen {
				model.screen = simulationScreen
				model.setContent(model.simulation)
			} else {
				model.screen = menuScreen
				model.preview = preview{}
			}
			return model, nil
		}
		if model.width < 32 || model.height < 16 {
			return model, nil
		}
		switch model.screen {
		case menuScreen:
			switch key {
			case "up", "k":
				model.cursor = max(0, model.cursor-1)
			case "down", "j":
				model.cursor = min(len(actions)-1, model.cursor+1)
			case "enter":
				model.selected = action(model.cursor)
				command := model.openSettings()
				return model, command
			}
		case settingsScreen:
			if key == "tab" || key == "shift+tab" {
				model.fields[model.focus].input.Blur()
				step := 1
				if key == "shift+tab" {
					step = -1
				}
				model.focus = (model.focus + step + len(model.fields)) % len(model.fields)
				return model, model.fields[model.focus].input.Focus()
			}
			if key == "enter" {
				model.readSettings()
				selected, config := model.selected, model.config
				command := model.begin("Lecture et preparation de l'apercu", func() tea.Msg {
					result, err := model.services.prepare(selected, config)
					return preparedMsg{preview: result, err: err}
				})
				return model, command
			}
			var command tea.Cmd
			model.fields[model.focus].input, command = model.fields[model.focus].input.Update(message)
			return model, command
		case previewScreen:
			if key == "e" && (model.selected == filesAction || model.selected == exportWindowsAction || model.selected == createLayoutAction) {
				command := model.begin("Export JSON sans ecrasement", func() tea.Msg {
					result, err := model.services.export(model.preview)
					return finishedMsg{report: result, err: err, export: true}
				})
				return model, command
			}
			if key == "m" && model.selected == createLayoutAction {
				model.screen = layoutEditorScreen
				model.status = "ATTRIBUTION DES BUREAUX"
				model.setContent(model.layoutEditorContent())
				return model, nil
			}
			if key == "s" && model.selected != exportWindowsAction && model.selected != createLayoutAction {
				command := model.startApply(false)
				return model, command
			}
		case layoutEditorScreen:
			switch key {
			case "up", "k":
				model.layoutCursor = max(0, model.layoutCursor-1)
				model.selectCurrentLayoutChoice()
			case "down", "j":
				model.layoutCursor = min(len(model.layoutWindows)-1, model.layoutCursor+1)
				model.selectCurrentLayoutChoice()
			case "left", "h":
				model.layoutChoice = (model.layoutChoice - 1 + len(model.layoutDesktops)) % len(model.layoutDesktops)
			case "right", "l":
				model.layoutChoice = (model.layoutChoice + 1) % len(model.layoutDesktops)
			case "enter":
				model.assignCurrentLayoutTarget()
			}
			model.setContent(model.layoutEditorContent())
			return model, nil
		case simulationScreen:
			if key == "a" {
				model.screen = confirmScreen
				model.setContent(model.confirmation())
				return model, nil
			}
		case confirmScreen:
			if key == "o" {
				command := model.startApply(true)
				return model, command
			}
			if key == "n" || key == "enter" {
				model.screen = simulationScreen
				model.setContent(model.simulation)
				return model, nil
			}
		}
	}
	if model.screen == previewScreen || model.screen == simulationScreen || model.screen == confirmScreen || model.screen == resultScreen || model.screen == errorScreen {
		var command tea.Cmd
		model.viewport, command = model.viewport.Update(message)
		return model, command
	}
	return model, nil
}

func (model *Model) startApply(real bool) tea.Cmd {
	status := "Simulation en cours - aucun deplacement"
	if real {
		status = "Execution reelle en cours - attendre le bilan"
	}
	return model.begin(status, func() tea.Msg {
		result, err := model.services.apply(model.preview, real)
		return finishedMsg{report: result, err: err, real: real}
	})
}

func (model Model) View() tea.View {
	if model.width < 32 || model.height < 16 {
		view := tea.NewView(ansi.Hardwrap("Terminal trop petit. Agrandir la fenetre (32 x 16 minimum).", max(1, model.width), true))
		view.AltScreen = true
		return view
	}
	title := model.title()
	body := ""
	switch model.screen {
	case menuScreen:
		body = model.menuView()
	case settingsScreen:
		body = actions[model.selected] + "\n\n"
		if model.selected == filesAction {
			body += "Regles absentes : classement par defaut.\n"
		}
		body += model.fields[model.focus].label + "\n" + model.fields[model.focus].input.View()
		body += fmt.Sprintf("\n\nChamp %d/%d", model.focus+1, len(model.fields))
	case busyScreen:
		body = string("|/-\\"[model.pulse%4]) + " " + model.status + "\n\nAttendre la fin de l'operation.\nQuitter ne constitue pas une annulation."
	case layoutEditorScreen:
		body = model.layoutEditorView()
	default:
		body = model.heading() + model.viewport.View()
	}
	width := model.width - 4
	footer := ansi.Hardwrap(model.footer(), width, true)
	if model.screen != layoutEditorScreen {
		body = ansi.Hardwrap(body, width, true)
	}
	view := tea.NewView(title + "\n\n" + lipgloss.NewStyle().MaxHeight(model.bodyHeight()).Render(body) + "\n\n" + footer)
	view.AltScreen = true
	return view
}

func (model Model) menuView() string {
	lines := []string{accentStyle.Render("ORGANISER LES FENETRES"), mutedStyle.Render("Choisis une action pour continuer."), ""}
	for index, label := range actions {
		if model.width < 64 {
			lines[0] = accentStyle.Render("DESKCTL / ACTIONS")
			lines[1] = mutedStyle.Render("Haut/Bas puis Entree")
			label = []string{"Fichiers", "Inventaire", "Disposition", "Placer", "Annuler"}[index]
		}
		line := fmt.Sprintf("%d  %s", index+1, label)
		if index == model.cursor {
			line = selectedRowStyle.Width(max(1, model.width-10)).Render("> " + line)
		} else {
			line = lipgloss.NewStyle().Width(max(1, model.width-10)).Render("  " + line)
		}
		lines = append(lines, line)
	}
	panel := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#34747A")).Padding(1, 2).Width(max(1, model.width-8)).Render(strings.Join(lines, "\n"))
	return lipgloss.Place(max(1, model.width-4), model.bodyHeight(), lipgloss.Center, lipgloss.Center, panel)
}

func (model Model) layoutEditorView() string {
	if len(model.layoutWindows) == 0 || len(model.layoutDesktops) == 0 {
		return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#A85B55")).Padding(1).Render("Aucune fenetre ou aucun bureau disponible.")
	}
	assigned := 0
	for _, target := range model.layoutTargets {
		if target != "" {
			assigned++
		}
	}
	header := accentStyle.Render("AFFECTATION DES BUREAUX") + "  " + mutedStyle.Render(fmt.Sprintf("%d/%d applications affectees", assigned, len(model.layoutWindows)))
	contentWidth := max(1, model.width-4)
	contentHeight := max(6, model.bodyHeight()-2)
	if contentWidth < 76 {
		compactHeader := accentStyle.Render(fmt.Sprintf("FENETRE %d/%d", model.layoutCursor+1, len(model.layoutWindows)))
		return ansi.Hardwrap(compactHeader, contentWidth, true) + "\n" + model.layoutDetailsPanel(contentWidth, contentHeight, true)
	}
	leftWidth := (contentWidth - 3) * 43 / 100
	rightWidth := contentWidth - leftWidth - 3
	left := model.layoutApplicationsPanel(leftWidth, contentHeight)
	right := model.layoutDetailsPanel(rightWidth, contentHeight, false)
	return header + "\n" + lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right)
}

func (model Model) layoutApplicationsPanel(width, height int) string {
	innerWidth := max(8, width-6)
	visible := max(1, (height-5)/2)
	start := max(0, min(model.layoutCursor-visible/2, len(model.layoutWindows)-visible))
	lines := []string{panelTitleStyle.Render("APPLICATIONS")}
	for index := start; index < min(len(model.layoutWindows), start+visible); index++ {
		window := model.layoutWindows[index]
		target := "Non attribue"
		if id := model.layoutTargets[index]; id != "" {
			target = model.desktopName(id)
		}
		count := len(windowsForProcess(model.currentLayoutWindows(), window.ProcessId))
		row := fmt.Sprintf("%02d  %-13s %dx  %s", index+1, truncateText(window.ProcessName, 13), count, truncateText(target, max(8, innerWidth-24)))
		windowTitle := window.WindowTitle
		if strings.TrimSpace(windowTitle) == "" {
			windowTitle = "(sans titre)"
		}
		detail := mutedStyle.Render("    " + truncateText(windowTitle, max(8, innerWidth-5)))
		if index == model.layoutCursor {
			lines = append(lines, selectedRowStyle.Width(innerWidth).Render("> "+row), selectedRowStyle.Width(innerWidth).Render(detail))
		} else {
			lines = append(lines, "  "+row, detail)
		}
	}
	return renderPanel(strings.Join(lines, "\n"), width, height, "#34747A")
}

func (model Model) layoutDetailsPanel(width, height int, compact bool) string {
	window := model.layoutWindows[model.layoutCursor]
	desktop := model.layoutDesktops[model.layoutChoice]
	innerWidth := max(8, width-6)
	appWindows := windowsForProcess(model.currentLayoutWindows(), window.ProcessId)
	if len(appWindows) == 0 {
		appWindows = []winapi.WindowInfo{window}
	}
	title := "APPLICATION SELECTIONNEE"
	if compact {
		title = fmt.Sprintf("APPLICATION %d/%d", model.layoutCursor+1, len(model.layoutWindows))
	}
	lines := []string{panelTitleStyle.Render(title), accentStyle.Render(window.ProcessName), currentStyle.Render(fmt.Sprintf("%d fenetre(s) detectee(s)", len(appWindows))), "", panelTitleStyle.Render("FENETRES")}
	visibleTitles := min(3, len(appWindows))
	for index := 0; index < visibleTitles; index++ {
		windowTitle := appWindows[index].WindowTitle
		if strings.TrimSpace(windowTitle) == "" {
			windowTitle = "(sans titre)"
		}
		lines = append(lines, mutedStyle.Render(truncateText(windowTitle, innerWidth)))
	}
	if len(appWindows) > visibleTitles {
		lines = append(lines, mutedStyle.Render(fmt.Sprintf("et %d autre(s) - Inventaire", len(appWindows)-visibleTitles)))
	}
	lines = append(lines, "", panelTitleStyle.Render("CHOISIR UN BUREAU"))
	visible := max(1, height-len(lines)-5)
	start := max(0, min(model.layoutChoice-visible/2, len(model.layoutDesktops)-visible))
	for index := start; index < min(len(model.layoutDesktops), start+visible); index++ {
		available := model.layoutDesktops[index]
		marker := "  "
		label := available.DisplayName()
		if available.ID == window.DesktopID {
			label += "  (actuel)"
		}
		line := marker + label
		if index == model.layoutChoice {
			line = selectedRowStyle.Width(innerWidth).Render("> " + label)
		}
		lines = append(lines, line)
	}
	lines = append(lines, "", mutedStyle.Render(fmt.Sprintf("Cible : %s", desktop.DisplayName())))
	return renderPanel(strings.Join(lines, "\n"), width, height, "#34747A")
}

func renderPanel(content string, width, height int, borderColor string) string {
	return lipgloss.NewStyle().Width(max(1, width-4)).Height(max(1, height-2)).Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(borderColor)).Padding(0, 1).Render(content)
}

func (model Model) desktopName(id string) string {
	for _, desktop := range model.layoutDesktops {
		if desktop.ID == id {
			return desktop.DisplayName()
		}
	}
	return "Bureau inconnu"
}

func (model Model) currentLayoutWindows() []winapi.WindowInfo {
	job, ok := model.preview.data.(work)
	if !ok {
		return model.layoutWindows
	}
	return job.windows
}

func truncateText(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	if limit == 1 {
		return "…"
	}
	return string(runes[:limit-1]) + "…"
}

func (model *Model) selectCurrentLayoutChoice() {
	if model.layoutCursor < 0 || model.layoutCursor >= len(model.layoutTargets) {
		return
	}
	if target := model.layoutTargets[model.layoutCursor]; target != "" {
		for index, desktop := range model.layoutDesktops {
			if desktop.ID == target {
				model.layoutChoice = index
				return
			}
		}
	}
}

func (model *Model) assignCurrentLayoutTarget() {
	if len(model.layoutDesktops) == 0 || len(model.layoutWindows) == 0 {
		return
	}
	model.layoutTargets[model.layoutCursor] = model.layoutDesktops[model.layoutChoice].ID
	for index := model.layoutCursor + 1; index < len(model.layoutTargets); index++ {
		if model.layoutTargets[index] == "" {
			model.layoutCursor = index
			return
		}
	}
	for index := 0; index < model.layoutCursor; index++ {
		if model.layoutTargets[index] == "" {
			model.layoutCursor = index
			return
		}
	}
	job, ok := model.preview.data.(work)
	if !ok {
		model.screen = errorScreen
		model.status = "ERREUR"
		model.setContent("brouillon de disposition absent")
		return
	}
	job.layout = make([]vdesktop.LayoutEntry, len(model.layoutWindows))
	lines := []string{"Disposition preparee - aucun deplacement", ""}
	for index, window := range model.layoutWindows {
		job.layout[index] = vdesktop.LayoutEntry{ProcessId: window.ProcessId, TargetDesktopID: model.layoutTargets[index]}
		group := windowsForProcess(job.windows, window.ProcessId)
		lines = append(lines, fmt.Sprintf("PID %d | %s | %d fenetre(s)\n  Bureau : %s", window.ProcessId, window.ProcessName, len(group), model.desktopName(model.layoutTargets[index])))
		for _, appWindow := range group {
			title := appWindow.WindowTitle
			if strings.TrimSpace(title) == "" {
				title = "(sans titre)"
			}
			lines = append(lines, "  - "+title)
		}
	}
	model.preview.data = job
	model.preview.title = "Disposition des fenetres"
	model.preview.lines = lines
	model.screen = previewScreen
	model.status = "APERCU - aucun deplacement"
	model.setContent(strings.Join(lines, "\n"))
}

func (model Model) layoutEditorContent() string {
	if len(model.layoutWindows) == 0 || len(model.layoutDesktops) == 0 {
		return "Aucune fenetre ou aucun bureau disponible."
	}
	window := model.layoutWindows[model.layoutCursor]
	target := model.layoutTargets[model.layoutCursor]
	if target == "" {
		target = "(non attribue)"
	}
	lines := []string{
		fmt.Sprintf("Application %d/%d", model.layoutCursor+1, len(model.layoutWindows)),
		"Application : " + window.ProcessName,
		fmt.Sprintf("Fenetre(s) : %d", len(windowsForProcess(model.currentLayoutWindows(), window.ProcessId))),
		"Cible : " + target,
		"Choix : " + model.layoutDesktops[model.layoutChoice].DisplayName(),
		"",
		"Bureaux disponibles :",
	}
	for index, desktop := range model.layoutDesktops {
		marker := "  "
		if index == model.layoutChoice {
			marker = "> "
		}
		lines = append(lines, marker+desktop.DisplayName())
	}
	return strings.Join(lines, "\n")
}

func (model Model) title() string {
	brand := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#50D5C8")).Render("deskctl")
	if model.width < 64 {
		return brand
	}
	subtitle := lipgloss.NewStyle().Foreground(lipgloss.Color("#A7B7BD")).Render("Organisation du Bureau")
	return ansi.Hardwrap(brand+"  "+subtitle, max(1, model.width-4), true)
}

func (model Model) bodyHeight() int {
	footerHeight := lipgloss.Height(ansi.Hardwrap(model.footer(), max(1, model.width-4), true))
	return max(1, model.height-lipgloss.Height(model.title())-footerHeight-3)
}

func (model Model) heading() string {
	if model.screen == confirmScreen {
		return "CONFIRMATION : MODIFICATIONS REELLES\n\n"
	}
	return model.status + "\n" + model.preview.title + "\n\n"
}

func (model Model) confirmation() string {
	content := actions[model.selected]
	if model.preview.desktop != "" {
		content += "\nBureau : " + model.preview.desktop
	}
	if model.selected == filesAction || model.selected == undoAction {
		content += "\nJournal : " + model.config.log
	}
	if strings.Contains(strings.ToLower(model.preview.desktop), "onedrive") {
		content += "\nATTENTION : OneDrive synchronisera les deplacements."
	}
	return content + "\n\nLes fichiers ou fenetres seront deplaces.\nLes deplacements ne sont pas annules en quittant."
}

func (model Model) footer() string {
	if model.width < 48 && model.screen == menuScreen {
		return "Haut/Bas / Entree"
	}
	switch model.screen {
	case menuScreen:
		return "Haut/Bas : choisir | Entree : ouvrir | q : quitter"
	case settingsScreen:
		return "Tab / Maj+Tab : champs | Entree : apercu | Echap : menu"
	case layoutEditorScreen:
		return "Haut/Bas : fenetre | Gauche/Droite : bureau | Entree : affecter | Echap : annuler"
	case busyScreen:
		return "Traitement actif ; actions clavier suspendues."
	case confirmScreen:
		return "o : confirmer | n / Entree / Echap : ne pas appliquer"
	}
	help := "Haut/Bas/PgUp/PgDn : defiler | Echap : menu | q : quitter"
	if model.screen == simulationScreen {
		return "a : demander l'application reelle | " + help
	}
	if model.screen == previewScreen {
		if model.selected == createLayoutAction {
			return "e : enregistrer layout et inventaire | m : modifier | " + help
		}
		if model.selected == exportWindowsAction {
			return "e : exporter JSON | " + help
		}
		if model.selected == filesAction {
			return "s : simuler | e : exporter JSON | " + help
		}
		return "s : simuler | " + help
	}
	return help
}
