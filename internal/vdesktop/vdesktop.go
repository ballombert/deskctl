package vdesktop

import (
	"encoding/json"
	"fmt"
	"strings"

	"deskctl/internal/winapi"
)

// LayoutEntry représente l'entrée d'un processus dans layout.json.
type LayoutEntry struct {
	ProcessId       int    `json:"ProcessId"`
	TargetDesktopID string `json:"TargetDesktopId"`
}

// WindowReference stocke les métadonnées d'origine pour détecter les PIDs recyclés.
type WindowReference struct {
	ProcessId   int
	ProcessName string
}

// Bilan résume le résultat de la simulation ou de l'application.
type Bilan struct {
	TotalEntries int
	Success      int
	Ignored      int
	Conflicts    int
	Failures     int
	Details      []string
}

type WindowMover func(hwnd uintptr, desktopID string) error

// ValidateLayout vérifie l'intégrité du JSON : non vide, PIDs uniques, cibles non vides.
func ValidateLayout(data []byte) ([]LayoutEntry, error) {
	var legacyEntries []struct {
		TargetDesktopName string `json:"TargetDesktopName"`
	}
	if err := json.Unmarshal(data, &legacyEntries); err == nil {
		for _, entry := range legacyEntries {
			if strings.TrimSpace(entry.TargetDesktopName) != "" {
				return nil, fmt.Errorf("ancien format de layout avec TargetDesktopName; recreer le fichier avec TargetDesktopId")
			}
		}
	}

	var entries []LayoutEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("JSON invalide : %w", err)
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("le fichier layout est vide")
	}

	seenPID := make(map[int]bool)
	for i, entry := range entries {
		if entry.ProcessId <= 0 {
			return nil, fmt.Errorf("entrée %d : ProcessId invalide (%d)", i, entry.ProcessId)
		}
		if seenPID[entry.ProcessId] {
			return nil, fmt.Errorf("ProcessId %d présent en double (chaque PID doit apparaître exactement une fois)", entry.ProcessId)
		}
		seenPID[entry.ProcessId] = true

		if _, err := winapi.ParseDesktopID(entry.TargetDesktopID); err != nil {
			return nil, fmt.Errorf("entrée %d (PID %d) : TargetDesktopId invalide : %w", i, entry.ProcessId, err)
		}
	}

	return entries, nil
}

func ValidateReference(entries []LayoutEntry, windows []winapi.WindowInfo) (map[int]string, error) {
	references := make(map[int]string, len(windows))
	seenWindows := make(map[uint64]bool, len(windows))
	for _, window := range windows {
		if window.ProcessId <= 0 || strings.TrimSpace(window.ProcessName) == "" {
			return nil, fmt.Errorf("inventaire invalide : PID ou nom de processus vide")
		}
		if window.WindowID == 0 || seenWindows[window.WindowID] {
			return nil, fmt.Errorf("inventaire invalide : WindowId absent ou duplique (%d)", window.WindowID)
		}
		seenWindows[window.WindowID] = true
		if _, err := winapi.ParseDesktopID(window.DesktopID); err != nil {
			return nil, fmt.Errorf("inventaire invalide pour WindowId %d : DesktopId manquant ou invalide; reexporter l'inventaire", window.WindowID)
		}
		if originalName, exists := references[window.ProcessId]; exists {
			if !strings.EqualFold(originalName, window.ProcessName) {
				return nil, fmt.Errorf("inventaire invalide : noms de processus differents pour le PID %d", window.ProcessId)
			}
		} else {
			references[window.ProcessId] = window.ProcessName
		}
	}
	for _, entry := range entries {
		if _, found := references[entry.ProcessId]; !found {
			return nil, fmt.Errorf("PID %d absent de l'inventaire de reference", entry.ProcessId)
		}
	}
	return references, nil
}

func ValidateDesktopTargets(entries []LayoutEntry, desktops []winapi.VirtualDesktopInfo) (map[string]string, error) {
	available := make(map[string]string, len(desktops))
	for _, desktop := range desktops {
		if _, err := winapi.ParseDesktopID(desktop.ID); err != nil {
			return nil, fmt.Errorf("GUID de bureau invalide %q", desktop.ID)
		}
		if _, exists := available[desktop.ID]; exists {
			return nil, fmt.Errorf("GUID de bureau duplique %q", desktop.ID)
		}
		available[desktop.ID] = desktop.DisplayName()
	}
	for _, entry := range entries {
		if _, found := available[entry.TargetDesktopID]; !found {
			return nil, fmt.Errorf("GUID cible %s absent de la liste des bureaux; actualiser le layout", entry.TargetDesktopID)
		}
	}
	return available, nil
}

func ApplyLayout(moveWindow WindowMover, entries []LayoutEntry, windows []winapi.WindowInfo, desktops []winapi.VirtualDesktopInfo, windowsRef map[int]string, execute bool) (*Bilan, error) {
	bilan := &Bilan{}
	desktopNames, err := ValidateDesktopTargets(entries, desktops)
	if err != nil {
		return nil, err
	}
	currentWindows := make(map[int][]winapi.WindowInfo, len(windows))
	for _, window := range windows {
		currentWindows[window.ProcessId] = append(currentWindows[window.ProcessId], window)
	}
	for _, entry := range entries {
		processWindows, found := currentWindows[entry.ProcessId]
		if !found {
			bilan.TotalEntries++
			bilan.Ignored++
			bilan.Details = append(bilan.Details, fmt.Sprintf("PID %d ferme (ignore)", entry.ProcessId))
			continue
		}
		for _, window := range processWindows {
			bilan.TotalEntries++
			if originalName, hasRef := windowsRef[entry.ProcessId]; hasRef {
				if !strings.EqualFold(originalName, window.ProcessName) {
					bilan.Conflicts++
					bilan.Details = append(bilan.Details, fmt.Sprintf("PID %d recycle ! Nom d'origine=%q, nom actuel=%q (deplacement annule)", entry.ProcessId, originalName, window.ProcessName))
					continue
				}
			}
			if execute {
				if moveWindow == nil {
					return nil, fmt.Errorf("fonction de deplacement Windows absente")
				}
				if err := moveWindow(window.WindowHandle, entry.TargetDesktopID); err != nil {
					bilan.Failures++
					bilan.Details = append(bilan.Details, fmt.Sprintf("Echec deplacement WindowId %d (%s) vers %s : %v", window.WindowID, window.ProcessName, desktopNames[entry.TargetDesktopID], err))
					continue
				}
				bilan.Success++
				bilan.Details = append(bilan.Details, fmt.Sprintf("WindowId %d (%s) deplace vers %s", window.WindowID, window.ProcessName, desktopNames[entry.TargetDesktopID]))
			} else {
				bilan.Success++
				bilan.Details = append(bilan.Details, fmt.Sprintf("[SIMULATION] WindowId %d (%s) -> bureau %s", window.WindowID, window.ProcessName, desktopNames[entry.TargetDesktopID]))
			}
		}
	}

	return bilan, nil
}
