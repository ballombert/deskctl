package desktopfiles

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ApplyUndo restaure les fichiers à leur emplacement d'origine d'après le journal.
// Protections :
// - Simulation par défaut.
// - Aucun écrasement si un fichier existe déjà à l'emplacement d'origine.
// - Ignore proprement les fichiers disparus du dossier cible.
func ApplyUndo(logPath string, execute bool) (*DesktopBilan, error) {
	data, err := os.ReadFile(logPath)
	if err != nil {
		return nil, fmt.Errorf("impossible de lire le journal d'annulation %s : %w", logPath, err)
	}

	var log UndoLog
	if err := json.Unmarshal(data, &log); err != nil {
		return nil, fmt.Errorf("journal d'annulation invalide : %w", err)
	}

	bilan := &DesktopBilan{Total: len(log.Entries)}

	// Appliquer en ordre inverse pour cohérence
	for i := len(log.Entries) - 1; i >= 0; i-- {
		entry := log.Entries[i]

		// 1. Vérifier si le fichier déplacé est toujours présent
		if _, err := os.Lstat(entry.CurrentTarget); err != nil {
			if os.IsNotExist(err) {
				bilan.Ignored++
				bilan.Details = append(bilan.Details, fmt.Sprintf("Fichier introuvable dans la cible : %s (ignoré)", filepath.Base(entry.CurrentTarget)))
				continue
			}
			bilan.Failures++
			bilan.Details = append(bilan.Details, fmt.Sprintf("Erreur accès cible %s : %v", entry.CurrentTarget, err))
			continue
		}

		// 2. Protection contre l'écrasement à la source
		if _, err := os.Lstat(entry.OriginalSource); err == nil {
			bilan.Conflicts++
			bilan.Details = append(bilan.Details, fmt.Sprintf("Collision : %s existe déjà à l'emplacement d'origine (non restauré)", filepath.Base(entry.OriginalSource)))
			continue
		}

		if execute {
			// S'assurer que le dossier source d'origine existe
			origDir := filepath.Dir(entry.OriginalSource)
			_ = os.MkdirAll(origDir, 0755)

			if err := os.Rename(entry.CurrentTarget, entry.OriginalSource); err != nil {
				bilan.Failures++
				bilan.Details = append(bilan.Details, fmt.Sprintf("Échec restauration %s : %v", filepath.Base(entry.CurrentTarget), err))
				continue
			}

			bilan.Success++
			bilan.Details = append(bilan.Details, fmt.Sprintf("Restauré : %s -> %s", filepath.Base(entry.CurrentTarget), entry.OriginalSource))
		} else {
			bilan.Success++
			bilan.Details = append(bilan.Details, fmt.Sprintf("[SIMULATION] Restauration : %s -> %s", filepath.Base(entry.CurrentTarget), entry.OriginalSource))
		}
	}

	return bilan, nil
}
