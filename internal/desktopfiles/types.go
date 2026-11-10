package desktopfiles

import "time"

// ScannedFile représente un fichier éligible au tri découvert sur le Bureau.
type ScannedFile struct {
	FileName  string    `json:"FileName"`
	Path      string    `json:"Path"`
	Size      int64     `json:"Size"`
	ModTime   time.Time `json:"ModTime"`
	Extension string    `json:"Extension"`
}

// RulesConfig contient le dictionnaire des extensions par catégorie.
type RulesConfig struct {
	Categories       map[string][]string `json:"Categories"`
	FallbackCategory string              `json:"FallbackCategory"`
}

// PlanItem décrit le déplacement projeté pour un fichier.
type PlanItem struct {
	SourcePath      string    `json:"SourcePath"`
	TargetPath      string    `json:"TargetPath"`
	Category        string    `json:"Category"`
	OriginalModTime time.Time `json:"OriginalModTime"`
	OriginalSize    int64     `json:"OriginalSize"`
	Status          string    `json:"Status"` // "Planned", "Conflict", "Ignored"
	Reason          string    `json:"Reason,omitempty"`
}

// UndoEntry enregistre un déplacement réussi pour permettre son annulation.
type UndoEntry struct {
	OriginalSource string    `json:"OriginalSource"`
	CurrentTarget  string    `json:"CurrentTarget"`
	MovedAt        time.Time `json:"MovedAt"`
	Size           int64     `json:"Size"`
}

// UndoLog représente le journal complet des opérations réelles.
type UndoLog struct {
	CreatedAt time.Time   `json:"CreatedAt"`
	Entries   []UndoEntry `json:"Entries"`
}

// DesktopBilan résume les résultats d'une passe de plan, d'apply ou d'undo.
type DesktopBilan struct {
	Total     int      `json:"Total"`
	Success   int      `json:"Success"`
	Ignored   int      `json:"Ignored"`
	Conflicts int      `json:"Conflicts"`
	Failures  int      `json:"Failures"`
	Details   []string `json:"Details"`
}
