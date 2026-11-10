package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"deskctl/internal/desktopfiles"
	"deskctl/internal/vdesktop"
	"deskctl/internal/winapi"
)

type work struct {
	action     action
	config     settings
	desktop    string
	files      []desktopfiles.ScannedFile
	plan       []desktopfiles.PlanItem
	windows    []winapi.WindowInfo
	desktops   []winapi.VirtualDesktopInfo
	layout     []vdesktop.LayoutEntry
	references map[int]string
	undo       []byte
}

func Run() error {
	_, err := tea.NewProgram(newModel(services{
		prepare: prepare,
		apply:   apply,
		export:  exportJSON,
	})).Run()
	return err
}

func prepare(selected action, config settings) (preview, error) {
	switch selected {
	case filesAction, undoAction:
		desktop, err := winapi.GetUserDesktopPath()
		if err != nil {
			return preview{}, fmt.Errorf("detection du Bureau : %w", err)
		}
		if selected == filesAction {
			return prepareFiles(config, desktop)
		}
		return prepareUndo(config, desktop)
	case exportWindowsAction:
		windows, err := winapi.EnumerateUserWindows()
		if err != nil {
			return preview{}, err
		}
		lines := []string{fmt.Sprintf("%d fenetre(s) detectee(s)", len(windows)), "Export : " + config.windows, ""}
		for _, window := range windows {
			title := window.WindowTitle
			if strings.TrimSpace(title) == "" {
				title = "(sans titre)"
			}
			lines = append(lines, fmt.Sprintf("HWND %d | PID %d | %s | %s | %s", window.WindowID, window.ProcessId, window.ProcessName, window.DesktopName, title))
		}
		return preview{title: "Inventaire des fenetres", lines: lines,
			data: work{action: selected, config: config, windows: windows}}, nil
	case createLayoutAction:
		return prepareLayout(config)
	case windowsAction:
		return prepareWindows(config)
	default:
		return preview{}, fmt.Errorf("operation inconnue")
	}
}

func prepareLayout(config settings) (preview, error) {
	windows, err := winapi.EnumerateUserWindows()
	if err != nil {
		return preview{}, fmt.Errorf("lecture des fenetres : %w", err)
	}
	desktops, err := winapi.EnumerateVirtualDesktops()
	if err != nil {
		return preview{}, fmt.Errorf("lecture des bureaux : %w", err)
	}
	return buildLayoutPreview(config, windows, desktops)
}

func buildLayoutPreview(config settings, windows []winapi.WindowInfo, desktops []winapi.VirtualDesktopInfo) (preview, error) {
	if len(windows) == 0 {
		return preview{}, fmt.Errorf("aucune fenetre utilisateur detectee")
	}
	if len(desktops) == 0 {
		return preview{}, fmt.Errorf("aucun GUID de bureau disponible dans l'inventaire des fenetres")
	}
	seenDesktops := make(map[string]bool, len(desktops))
	for _, desktopID := range desktops {
		if _, err := winapi.ParseDesktopID(desktopID.ID); err != nil {
			return preview{}, fmt.Errorf("GUID de bureau invalide %q", desktopID.ID)
		}
		if seenDesktops[desktopID.ID] {
			return preview{}, fmt.Errorf("GUID de bureau duplique %q", desktopID.ID)
		}
		seenDesktops[desktopID.ID] = true
	}
	seenWindows := make(map[uint64]bool, len(windows))
	processNames := make(map[int]string, len(windows))
	for _, window := range windows {
		if window.ProcessId <= 0 || strings.TrimSpace(window.ProcessName) == "" {
			return preview{}, fmt.Errorf("inventaire invalide : PID ou nom de processus vide")
		}
		if window.WindowID == 0 || seenWindows[window.WindowID] {
			return preview{}, fmt.Errorf("inventaire invalide : WindowId absent ou duplique (%d)", window.WindowID)
		}
		seenWindows[window.WindowID] = true
		if processName, exists := processNames[window.ProcessId]; exists && !strings.EqualFold(processName, window.ProcessName) {
			return preview{}, fmt.Errorf("inventaire invalide : noms de processus differents pour le PID %d", window.ProcessId)
		}
		processNames[window.ProcessId] = window.ProcessName
	}
	return preview{
		title: "Attribution des fenetres",
		lines: []string{fmt.Sprintf("%d fenetre(s) detectee(s) dans %d application(s)", len(windows), len(processNames)), fmt.Sprintf("%d bureau(x) disponible(s)", len(desktops))},
		data:  work{action: createLayoutAction, config: config, windows: windows, desktops: desktops},
	}, nil
}

func uniqueProcessWindows(windows []winapi.WindowInfo) []winapi.WindowInfo {
	seen := make(map[int]bool, len(windows))
	applications := make([]winapi.WindowInfo, 0, len(windows))
	for _, window := range windows {
		if !seen[window.ProcessId] {
			seen[window.ProcessId] = true
			applications = append(applications, window)
		}
	}
	return applications
}

func windowsForProcess(windows []winapi.WindowInfo, processID int) []winapi.WindowInfo {
	var matches []winapi.WindowInfo
	for _, window := range windows {
		if window.ProcessId == processID {
			matches = append(matches, window)
		}
	}
	return matches
}

func desktopNotice(desktop string) []string {
	lines := []string{"Bureau : " + desktop}
	if strings.Contains(strings.ToLower(desktop), "onedrive") {
		lines = append(lines, "ATTENTION OneDrive : les deplacements seront synchronises dans le cloud.")
	}
	return lines
}

func prepareFiles(config settings, desktop string) (preview, error) {
	files, err := desktopfiles.ScanDesktop(desktop)
	if err != nil {
		return preview{}, fmt.Errorf("scan du Bureau : %w", err)
	}
	rules := desktopfiles.DefaultRules()
	rulesNotice := "Classement par defaut"
	if config.rules != "" {
		loaded, err := desktopfiles.LoadRules(config.rules)
		if err != nil && !os.IsNotExist(err) {
			return preview{}, fmt.Errorf("regles %s : %w", config.rules, err)
		}
		if err == nil {
			rules = loaded
			rulesNotice = "Regles : " + config.rules
		} else {
			rulesNotice = "Regles absentes : classement par defaut (" + config.rules + ")"
		}
	}
	plan, bilan, err := desktopfiles.GeneratePlan(files, rules, desktop)
	if err != nil {
		return preview{}, err
	}
	lines := append(desktopNotice(desktop), rulesNotice, "Journal prevu : "+config.log,
		"Exports facultatifs : "+config.scan+" ; "+config.plan, "")
	lines = append(lines, desktopReport(bilan).lines...)
	for _, item := range plan {
		lines = append(lines, fmt.Sprintf("\n[%s] %s\n  Destination : %s", item.Status, item.SourcePath, item.TargetPath))
		if item.Reason != "" {
			lines = append(lines, "  Motif : "+item.Reason)
		}
	}
	return preview{title: "Plan de rangement", desktop: desktop, lines: lines,
		data: work{action: filesAction, config: config, desktop: desktop, files: files, plan: plan}}, nil
}

func prepareWindows(config settings) (preview, error) {
	data, err := os.ReadFile(config.layout)
	if err != nil {
		return preview{}, fmt.Errorf("disposition %s : %w", config.layout, err)
	}
	layout, err := vdesktop.ValidateLayout(data)
	if err != nil {
		return preview{}, err
	}
	data, err = os.ReadFile(config.windows)
	if err != nil {
		return preview{}, fmt.Errorf("inventaire de reference obligatoire %s : %w", config.windows, err)
	}
	var windows []winapi.WindowInfo
	if err := json.Unmarshal(data, &windows); err != nil {
		return preview{}, fmt.Errorf("inventaire de reference invalide : %w", err)
	}
	references, err := vdesktop.ValidateReference(layout, windows)
	if err != nil {
		return preview{}, err
	}
	desktops, err := winapi.EnumerateVirtualDesktops()
	if err != nil {
		return preview{}, fmt.Errorf("lecture des bureaux : %w", err)
	}
	desktopNames, err := vdesktop.ValidateDesktopTargets(layout, desktops)
	if err != nil {
		return preview{}, err
	}
	lines := []string{"Disposition : " + config.layout, "Reference : " + config.windows, "Bureaux : COM interne Windows", ""}
	for _, entry := range layout {
		name := references[entry.ProcessId]
		lines = append(lines, fmt.Sprintf("PID %d | %s\n  Bureau cible : %s", entry.ProcessId, name, desktopNames[entry.TargetDesktopID]))
	}
	return preview{title: "Placement des fenetres", lines: lines,
		data: work{action: windowsAction, config: config, layout: layout, desktops: desktops, references: references}}, nil
}

func prepareUndo(config settings, desktop string) (preview, error) {
	data, err := os.ReadFile(config.log)
	if err != nil {
		return preview{}, fmt.Errorf("lecture du journal : %w", err)
	}
	var log desktopfiles.UndoLog
	if err := json.Unmarshal(data, &log); err != nil {
		return preview{}, fmt.Errorf("journal invalide : %w", err)
	}
	if len(log.Entries) == 0 {
		return preview{}, fmt.Errorf("journal vide : aucune restauration possible")
	}
	lines := append(desktopNotice(desktop), "Journal : "+config.log,
		fmt.Sprintf("%d restauration(s) prevue(s)", len(log.Entries)), "")
	for _, entry := range log.Entries {
		if !desktopfiles.IsPathWithinDirectory(entry.OriginalSource, desktop) ||
			!desktopfiles.IsPathWithinDirectory(entry.CurrentTarget, desktop) {
			return preview{}, fmt.Errorf("journal refuse : un chemin sort du Bureau utilisateur")
		}
		lines = append(lines, entry.CurrentTarget+"\n  Restaurer vers : "+entry.OriginalSource)
	}
	return preview{title: "Annulation des fichiers uniquement", desktop: desktop, lines: lines,
		data: work{action: undoAction, config: config, desktop: desktop, undo: data}}, nil
}

func desktopReport(bilan *desktopfiles.DesktopBilan) report {
	if bilan == nil {
		return report{}
	}
	lines := []string{
		fmt.Sprintf("Total : %d | Succes ou simules : %d", bilan.Total, bilan.Success),
		fmt.Sprintf("Conflits : %d | Ignores : %d | Echecs : %d", bilan.Conflicts, bilan.Ignored, bilan.Failures), "",
	}
	return report{lines: append(lines, bilan.Details...)}
}

func apply(prepared preview, real bool) (report, error) {
	job, ok := prepared.data.(work)
	if !ok {
		return report{}, fmt.Errorf("apercu absent : recommencer depuis le menu")
	}
	switch job.action {
	case filesAction:
		return applyFiles(job, real)
	case windowsAction:
		windows, err := winapi.EnumerateUserWindows()
		if err != nil {
			return report{}, fmt.Errorf("lecture des fenetres courantes : %w", err)
		}
		desktops, err := winapi.EnumerateVirtualDesktops()
		if err != nil {
			return report{}, fmt.Errorf("lecture des bureaux courants : %w", err)
		}
		bilan, err := vdesktop.ApplyLayout(winapi.MoveWindowToDesktop, job.layout, windows, desktops, job.references, real)
		if err != nil {
			return report{}, err
		}
		lines := []string{
			fmt.Sprintf("Total : %d | Succes ou simules : %d", bilan.TotalEntries, bilan.Success),
			fmt.Sprintf("Conflits PID : %d | Ignores : %d | Echecs : %d", bilan.Conflicts, bilan.Ignored, bilan.Failures),
			"Deplacement par COM interne Windows", "",
		}
		return report{lines: append(lines, bilan.Details...)}, nil
	case undoAction:
		current, err := os.ReadFile(job.config.log)
		if err != nil {
			return report{}, err
		}
		if !bytes.Equal(current, job.undo) {
			return report{}, fmt.Errorf("journal modifie depuis l'apercu : recommencer depuis le menu")
		}
		bilan, err := desktopfiles.ApplyUndo(job.config.log, real)
		return desktopReport(bilan), err
	default:
		return report{}, fmt.Errorf("cet inventaire doit etre exporte, pas applique")
	}
}

func applyFiles(job work, real bool) (report, error) {
	data, err := json.Marshal(job.plan)
	if err != nil {
		return report{}, err
	}
	plan, err := desktopfiles.ValidatePlanFile(data, job.desktop)
	if err != nil {
		return report{}, err
	}
	if real {
		if strings.TrimSpace(job.config.log) == "" {
			return report{}, fmt.Errorf("un nouveau chemin de journal est obligatoire")
		}
		file, err := os.OpenFile(job.config.log, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return report{}, fmt.Errorf("journal non disponible (aucun fichier deplace). Choisir un nouveau chemin : %w", err)
		}
		if err := file.Close(); err != nil {
			return report{}, err
		}
	}
	bilan, log, err := desktopfiles.ApplyPlan(plan, job.desktop, real, "")
	result := desktopReport(bilan)
	if real && log != nil {
		if len(log.Entries) == 0 {
			if removeErr := os.Remove(job.config.log); removeErr != nil {
				result.lines = append(result.lines, "Journal vide conserve : "+job.config.log)
			}
		} else {
			data, writeErr := json.MarshalIndent(log, "", "  ")
			if writeErr == nil {
				writeErr = os.WriteFile(job.config.log, data, 0600)
			}
			if writeErr == nil {
				var written []byte
				written, writeErr = os.ReadFile(job.config.log)
				if writeErr == nil && !bytes.Equal(written, data) {
					writeErr = fmt.Errorf("contenu relu different du journal attendu")
				}
			}
			if writeErr != nil {
				result.lines = append(result.lines, "RECUPERATION :\n"+string(data))
				return result, fmt.Errorf("fichiers deja deplaces, journal NON confirme : %w. Ne pas relancer le rangement", writeErr)
			}
			result.lines = append(result.lines, "Journal sauvegarde et verifie : "+job.config.log)
		}
	}
	return result, err
}

func exportJSON(prepared preview) (report, error) {
	job, ok := prepared.data.(work)
	if !ok {
		return report{}, fmt.Errorf("apercu absent")
	}
	result := report{}
	export := func(path string, value any) error {
		if err := writeJSON(path, value); err != nil {
			return err
		}
		result.lines = append(result.lines, "Export enregistre : "+path)
		return nil
	}
	if job.action == exportWindowsAction {
		err := export(job.config.windows, job.windows)
		return result, err
	}
	if job.action == createLayoutAction {
		if err := writeJSONPair(job.config.layout, job.layout, job.config.windows, job.windows); err != nil {
			return result, err
		}
		result.lines = append(result.lines, "Layout enregistre : "+job.config.layout, "Inventaire enregistre : "+job.config.windows)
		return result, nil
	}
	if job.action == filesAction {
		if err := export(job.config.scan, job.files); err != nil {
			return result, err
		}
		err := export(job.config.plan, job.plan)
		return result, err
	}
	return result, fmt.Errorf("cet apercu ne propose pas d'export")
}

func writeJSONPair(firstPath string, firstValue any, secondPath string, secondValue any) error {
	if strings.TrimSpace(firstPath) == "" || strings.TrimSpace(secondPath) == "" || firstPath == "-" || secondPath == "-" {
		return fmt.Errorf("deux chemins d'export JSON sont obligatoires")
	}
	firstData, err := json.MarshalIndent(firstValue, "", "  ")
	if err != nil {
		return err
	}
	secondData, err := json.MarshalIndent(secondValue, "", "  ")
	if err != nil {
		return err
	}
	firstFile, err := os.OpenFile(firstPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("export %s refuse (aucun ecrasement) : %w", filepath.Clean(firstPath), err)
	}
	createdPaths := []string{firstPath}
	cleanup := func() {
		for _, path := range createdPaths {
			_ = os.Remove(path)
		}
	}
	secondFile, err := os.OpenFile(secondPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		_ = firstFile.Close()
		cleanup()
		return fmt.Errorf("export %s refuse (aucun ecrasement) : %w", filepath.Clean(secondPath), err)
	}
	createdPaths = append(createdPaths, secondPath)
	if _, err := firstFile.Write(firstData); err != nil {
		_ = firstFile.Close()
		_ = secondFile.Close()
		cleanup()
		return fmt.Errorf("ecriture du layout %s : %w", firstPath, err)
	}
	if _, err := secondFile.Write(secondData); err != nil {
		_ = firstFile.Close()
		_ = secondFile.Close()
		cleanup()
		return fmt.Errorf("ecriture de l'inventaire %s : %w", secondPath, err)
	}
	if err := firstFile.Close(); err != nil {
		_ = secondFile.Close()
		cleanup()
		return fmt.Errorf("fermeture du layout %s : %w", firstPath, err)
	}
	if err := secondFile.Close(); err != nil {
		cleanup()
		return fmt.Errorf("fermeture de l'inventaire %s : %w", secondPath, err)
	}
	return nil
}

func writeJSON(path string, value any) error {
	if strings.TrimSpace(path) == "" || path == "-" {
		return fmt.Errorf("chemin d'export JSON obligatoire (pas de sortie standard en TUI)")
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("export %s refuse (aucun ecrasement) : %w", filepath.Clean(path), err)
	}
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil {
		return fmt.Errorf("export incomplet %s : %w", path, writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("fermeture de l'export %s : %w", path, closeErr)
	}
	return nil
}
