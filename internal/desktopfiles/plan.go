package desktopfiles

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// IsPathWithinDirectory vérifie qu'un chemin se situe strictement à l'intérieur du répertoire racine.
func IsPathWithinDirectory(targetPath, baseDir string) bool {
	cleanTarget := filepath.Clean(strings.ToLower(targetPath))
	cleanBase := filepath.Clean(strings.ToLower(baseDir))

	rel, err := filepath.Rel(cleanBase, cleanTarget)
	if err != nil {
		return false
	}

	if rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return false
	}
	return true
}

// GeneratePlan crée les actions projetées en appliquant les règles et en détectant les collisions.
func GeneratePlan(files []ScannedFile, rules RulesConfig, desktopDir string) ([]PlanItem, *DesktopBilan, error) {
	bilan := &DesktopBilan{Total: len(files)}
	var plan []PlanItem

	plannedTargets := make(map[string]bool)

	for _, f := range files {
		cat := MatchCategory(f.Extension, rules)
		targetDir := filepath.Join(desktopDir, cat)
		targetPath := filepath.Join(targetDir, f.FileName)

		// Vérification de sécurité : la cible doit être strictement dans desktopDir
		if !IsPathWithinDirectory(targetPath, desktopDir) {
			bilan.Failures++
			bilan.Details = append(bilan.Details, fmt.Sprintf("Chemin cible non autorisé pour %s : %s", f.FileName, targetPath))
			continue
		}

		// Si le fichier est déjà à l'emplacement cible
		if strings.EqualFold(filepath.Clean(f.Path), filepath.Clean(targetPath)) {
			bilan.Ignored++
			plan = append(plan, PlanItem{
				SourcePath:      f.Path,
				TargetPath:      targetPath,
				Category:        cat,
				OriginalModTime: f.ModTime,
				OriginalSize:    f.Size,
				Status:          "Ignored",
				Reason:          "Déjà à l'emplacement cible",
			})
			continue
		}

		// Détection de collision 1 : le fichier cible existe déjà sur le disque
		if _, err := os.Lstat(targetPath); err == nil {
			bilan.Conflicts++
			plan = append(plan, PlanItem{
				SourcePath:      f.Path,
				TargetPath:      targetPath,
				Category:        cat,
				OriginalModTime: f.ModTime,
				OriginalSize:    f.Size,
				Status:          "Conflict",
				Reason:          "Collision : un fichier de même nom existe déjà dans le dossier cible",
			})
			bilan.Details = append(bilan.Details, fmt.Sprintf("Conflit pour %s : %s existe déjà", f.FileName, targetPath))
			continue
		}

		// Détection de collision 2 : deux fichiers de même nom dans le même scan
		lowerTarget := strings.ToLower(targetPath)
		if plannedTargets[lowerTarget] {
			bilan.Conflicts++
			plan = append(plan, PlanItem{
				SourcePath:      f.Path,
				TargetPath:      targetPath,
				Category:        cat,
				OriginalModTime: f.ModTime,
				OriginalSize:    f.Size,
				Status:          "Conflict",
				Reason:          "Collision : doublon de destination dans le même plan",
			})
			bilan.Details = append(bilan.Details, fmt.Sprintf("Conflit doublon pour %s", f.FileName))
			continue
		}

		plannedTargets[lowerTarget] = true
		bilan.Success++
		plan = append(plan, PlanItem{
			SourcePath:      f.Path,
			TargetPath:      targetPath,
			Category:        cat,
			OriginalModTime: f.ModTime,
			OriginalSize:    f.Size,
			Status:          "Planned",
		})
	}

	return plan, bilan, nil
}

// ValidatePlanFile valide la structure et la sécurité des chemins d'un plan externe avant exécution.
func ValidatePlanFile(planData []byte, desktopDir string) ([]PlanItem, error) {
	var items []PlanItem
	if err := json.Unmarshal(planData, &items); err != nil {
		return nil, fmt.Errorf("JSON de plan invalide : %w", err)
	}

	for i, item := range items {
		if strings.TrimSpace(item.SourcePath) == "" || strings.TrimSpace(item.TargetPath) == "" {
			return nil, fmt.Errorf("entrée %d : SourcePath ou TargetPath vide", i)
		}

		// Sécurité absolue : SourcePath et TargetPath doivent être dans le Bureau utilisateur
		if !IsPathWithinDirectory(item.SourcePath, filepath.Dir(desktopDir)) &&
			!strings.EqualFold(filepath.Dir(item.SourcePath), desktopDir) {
			return nil, fmt.Errorf("entrée %d : SourcePath %q n'est pas situé sur le Bureau", i, item.SourcePath)
		}

		if !IsPathWithinDirectory(item.TargetPath, desktopDir) {
			return nil, fmt.Errorf("entrée %d : TargetPath %q s'échappe du Bureau autorisé", i, item.TargetPath)
		}
	}

	return items, nil
}
