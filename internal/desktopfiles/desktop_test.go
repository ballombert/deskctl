package desktopfiles

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMatchCategory(t *testing.T) {
	rules := DefaultRules()

	tests := []struct {
		ext      string
		expected string
	}{
		{".PDF", "Documents"},
		{".pdf", "Documents"},
		{".docx", "Documents"},
		{".png", "Images"},
		{".ZIP", "Archives"},
		{".exe", "Installateurs"},
		{".lnk", "Raccourcis"},
		{".url", "Raccourcis"},
		{".xyz123", "Autres"},
	}

	for _, tt := range tests {
		got := MatchCategory(tt.ext, rules)
		if got != tt.expected {
			t.Errorf("MatchCategory(%q) = %q, attendu %q", tt.ext, got, tt.expected)
		}
	}
}

func TestIsPathWithinDirectory(t *testing.T) {
	base := `C:\Users\test\Desktop`

	valid := `C:\Users\test\Desktop\Documents\file.pdf`
	if !IsPathWithinDirectory(valid, base) {
		t.Errorf("Attendu valide pour %s", valid)
	}

	// Tentative d'évasion
	escape := `C:\Users\test\Desktop\..\Documents\file.pdf`
	if IsPathWithinDirectory(escape, base) {
		t.Errorf("Attendu invalide pour évasion %s", escape)
	}

	root := `C:\Users\test\Desktop`
	if IsPathWithinDirectory(root, base) {
		t.Errorf("Attendu invalide pour racine exacte %s", root)
	}
}

func TestPlanCollisionsAndExecution(t *testing.T) {
	// Création d'un faux bureau temporaire
	tempDir, err := os.MkdirTemp("", "test_desktop_*")
	if err != nil {
		t.Fatalf("Erreur création dossier temporaire : %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Fichier 1 : rapport.pdf
	file1 := filepath.Join(tempDir, "rapport.pdf")
	_ = os.WriteFile(file1, []byte("contenu pdf"), 0644)

	// Fichier 2 : image.png
	file2 := filepath.Join(tempDir, "image.png")
	_ = os.WriteFile(file2, []byte("contenu image"), 0644)

	// Créer une collision préalable : Documents/rapport.pdf existe déjà !
	docsDir := filepath.Join(tempDir, "Documents")
	_ = os.MkdirAll(docsDir, 0755)
	existingTarget := filepath.Join(docsDir, "rapport.pdf")
	_ = os.WriteFile(existingTarget, []byte("existant"), 0644)

	rules := DefaultRules()
	scanned := []ScannedFile{
		{
			FileName:  "rapport.pdf",
			Path:      file1,
			Size:      11,
			ModTime:   time.Now(),
			Extension: ".pdf",
		},
		{
			FileName:  "image.png",
			Path:      file2,
			Size:      13,
			ModTime:   time.Now(),
			Extension: ".png",
		},
	}

	plan, bilan, err := GeneratePlan(scanned, rules, tempDir)
	if err != nil {
		t.Fatalf("GeneratePlan échoué : %v", err)
	}

	if bilan.Conflicts != 1 {
		t.Errorf("Attendu 1 collision pour rapport.pdf, obtenu %d", bilan.Conflicts)
	}
	if bilan.Success != 1 {
		t.Errorf("Attendu 1 planifié pour image.png, obtenu %d", bilan.Success)
	}

	// 1. Simulation de ApplyPlan
	undoLogPath := filepath.Join(tempDir, "undo.json")
	simBilan, _, err := ApplyPlan(plan, tempDir, false, undoLogPath)
	if err != nil {
		t.Fatalf("ApplyPlan simulation échoué : %v", err)
	}
	if simBilan.Success != 1 {
		t.Errorf("Simulation attendu Success=1, obtenu %d", simBilan.Success)
	}
	// Vérifier que image.png n'a PAS été déplacé en simulation
	if _, err := os.Stat(file2); err != nil {
		t.Errorf("Le fichier original ne doit pas être déplacé en simulation")
	}

	// 2. Exécution réelle de ApplyPlan
	execBilan, _, err := ApplyPlan(plan, tempDir, true, undoLogPath)
	if err != nil {
		t.Fatalf("ApplyPlan exécution échoué : %v", err)
	}
	if execBilan.Success != 1 {
		t.Errorf("Exécution attendu Success=1, obtenu %d", execBilan.Success)
	}
	targetImage := filepath.Join(tempDir, "Images", "image.png")
	if _, err := os.Stat(targetImage); err != nil {
		t.Errorf("Le fichier cible %s doit exister après exécution", targetImage)
	}

	// Vérifier que le fichier en collision (rapport.pdf) n'a PAS été écrasé
	contentOrig, _ := os.ReadFile(existingTarget)
	if string(contentOrig) != "existant" {
		t.Errorf("Le fichier existant en collision a été altéré !")
	}

	// 3. Test de Undo
	undoBilan, err := ApplyUndo(undoLogPath, true)
	if err != nil {
		t.Fatalf("ApplyUndo échoué : %v", err)
	}
	if undoBilan.Success != 1 {
		t.Errorf("Undo attendu Success=1, obtenu %d", undoBilan.Success)
	}
	if _, err := os.Stat(file2); err != nil {
		t.Errorf("image.png doit être restauré à son emplacement d'origine")
	}
}
