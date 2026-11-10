package desktopfiles

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ApplyPlan applique ou simule les déplacements de fichiers décrits dans le plan.
func ApplyPlan(
	plan []PlanItem,
	desktopDir string,
	execute bool,
	undoLogPath string,
) (*DesktopBilan, *UndoLog, error) {
	bilan := &DesktopBilan{Total: len(plan)}
	undoLog := &UndoLog{CreatedAt: time.Now().UTC()}

	for _, item := range plan {
		if item.Status != "Planned" {
			// Éléments déjà marqués en conflit ou ignorés lors du plan
			if item.Status == "Conflict" {
				bilan.Conflicts++
				bilan.Details = append(bilan.Details, fmt.Sprintf("Conflit ignoré : %s -> %s", filepath.Base(item.SourcePath), item.Reason))
			} else {
				bilan.Ignored++
			}
			continue
		}

		// 1. Validation de sécurité des chemins (même si le JSON a été altéré)
		if !IsPathWithinDirectory(item.TargetPath, desktopDir) {
			bilan.Failures++
			bilan.Details = append(bilan.Details, fmt.Sprintf("Chemin refusé (hors bureau) : %s", item.TargetPath))
			continue
		}

		// 2. Vérification de l'état du fichier source
		fi, err := os.Lstat(item.SourcePath)
		if err != nil {
			if os.IsNotExist(err) {
				bilan.Ignored++
				bilan.Details = append(bilan.Details, fmt.Sprintf("Fichier source disparu : %s (ignoré)", filepath.Base(item.SourcePath)))
				continue
			}
			bilan.Failures++
			bilan.Details = append(bilan.Details, fmt.Sprintf("Erreur accès source %s : %v", item.SourcePath, err))
			continue
		}

		// 3. Détection de modification depuis le scan (taille ou date de modif)
		if !item.OriginalModTime.IsZero() && fi.ModTime().UTC().Truncate(time.Second) != item.OriginalModTime.UTC().Truncate(time.Second) {
			bilan.Ignored++
			bilan.Details = append(bilan.Details, fmt.Sprintf("Fichier modifié depuis le scan : %s (ignoré)", filepath.Base(item.SourcePath)))
			continue
		}

		// 4. Vérification collision destination immédiate
		if _, err := os.Lstat(item.TargetPath); err == nil {
			bilan.Conflicts++
			bilan.Details = append(bilan.Details, fmt.Sprintf("Collision détectée à l'exécution : %s existe déjà (non écrasé)", item.TargetPath))
			continue
		}

		if execute {
			// Création du répertoire cible si nécessaire
			targetDir := filepath.Dir(item.TargetPath)
			if err := os.MkdirAll(targetDir, 0755); err != nil {
				bilan.Failures++
				bilan.Details = append(bilan.Details, fmt.Sprintf("Erreur création dossier %s : %v", targetDir, err))
				continue
			}

			// Déplacement atomique sans écrasement
			if err := os.Rename(item.SourcePath, item.TargetPath); err != nil {
				bilan.Failures++
				bilan.Details = append(bilan.Details, fmt.Sprintf("Échec déplacement %s : %v", item.SourcePath, err))
				continue
			}

			bilan.Success++
			bilan.Details = append(bilan.Details, fmt.Sprintf("Déplacé : %s -> %s", filepath.Base(item.SourcePath), filepath.Base(targetDir)))
			undoLog.Entries = append(undoLog.Entries, UndoEntry{
				OriginalSource: item.SourcePath,
				CurrentTarget:  item.TargetPath,
				MovedAt:        time.Now().UTC(),
				Size:           fi.Size(),
			})
		} else {
			bilan.Success++
			bilan.Details = append(bilan.Details, fmt.Sprintf("[SIMULATION] Déplacement : %s -> %s", filepath.Base(item.SourcePath), filepath.Dir(item.TargetPath)))
		}
	}

	// Sauvegarde du journal d'annulation si exécution réelle
	if execute && undoLogPath != "" && len(undoLog.Entries) > 0 {
		data, err := json.MarshalIndent(undoLog, "", "  ")
		if err == nil {
			_ = os.WriteFile(undoLogPath, data, 0644)
		}
	}

	return bilan, undoLog, nil
}
