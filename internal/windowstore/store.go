package windowstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"deskctl/internal/winapi"
	"deskctl/internal/windoworganizer"
	"deskctl/internal/windowrules"
)

type Store struct{ Directory string }

func Portable() (Store, error) {
	executable, err := os.Executable()
	if err != nil {
		return Store{}, err
	}
	return Store{Directory: filepath.Dir(executable)}, nil
}

func Read(path string, value any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if info.Size() > 4<<20 {
		return fmt.Errorf("JSON trop volumineux : %s", path)
	}
	decoder := json.NewDecoder(io.LimitReader(file, (4<<20)+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return fmt.Errorf("lecture JSON %s : %w", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return fmt.Errorf("contenu supplementaire apres le JSON : %s", path)
	}
	return nil
}

func Write(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if len(data) > 4<<20 {
		return fmt.Errorf("JSON trop volumineux")
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".deskctl-*.tmp")
	if err != nil {
		return fmt.Errorf("dossier portable non accessible en ecriture : %w", err)
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return fmt.Errorf("remplacement atomique %s : %w", path, err)
	}
	return nil
}

func (store Store) LoadRules() (windowrules.Document, error) {
	var doc windowrules.Document
	path := filepath.Join(store.Directory, "window-rules.json")
	err := Read(path, &doc)
	if errors.Is(err, os.ErrNotExist) {
		return windowrules.Empty(), nil
	}
	if err != nil {
		return doc, err
	}
	normalized, err := windowrules.Normalize(doc)
	if err != nil {
		return windowrules.Document{}, err
	}
	if doc.Version < windowrules.Version {
		if err := backupRules(path, doc.Version); err != nil {
			return windowrules.Document{}, err
		}
		if err := store.SaveRules(normalized); err != nil {
			return windowrules.Document{}, fmt.Errorf("migration non enregistree : %w", err)
		}
	}
	return normalized, nil
}

func backupRules(path string, version int) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	backup := fmt.Sprintf("%s.v%d.bak", path, version)
	file, err := os.OpenFile(backup, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err == nil {
		_, writeErr := file.Write(data)
		syncErr := file.Sync()
		closeErr := file.Close()
		if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
			return fmt.Errorf("sauvegarde de migration incomplete %s : %w", backup, err)
		}
	} else if !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("sauvegarde de migration impossible : %w", err)
	}
	saved, err := os.ReadFile(backup)
	if err != nil || string(saved) != string(data) {
		return fmt.Errorf("sauvegarde de migration non verifiee : %s (conserver et verifier les fichiers) : %v", backup, err)
	}
	return nil
}

func (store Store) SaveRules(doc windowrules.Document) error {
	doc, err := windowrules.Normalize(doc)
	if err != nil {
		return err
	}
	return Write(filepath.Join(store.Directory, "window-rules.json"), doc)
}

func (store Store) LoadJournal() (windoworganizer.Journal, error) {
	var journal windoworganizer.Journal
	if err := Read(filepath.Join(store.Directory, "window-undo.json"), &journal); err != nil {
		return journal, fmt.Errorf("journal d'annulation : %w", err)
	}
	return journal, windoworganizer.ValidateJournal(journal)
}

func (store Store) SaveJournal(journal windoworganizer.Journal) error {
	if err := windoworganizer.ValidateJournal(journal); err != nil {
		return err
	}
	return Write(filepath.Join(store.Directory, "window-undo.json"), journal)
}

// GeometryRecord is the position of a window before deskctl tiled it.
type GeometryRecord struct {
	WindowID    string       `json:"windowId"`
	ProcessID   uint32       `json:"processId"`
	Started     string       `json:"started"`
	Title       string       `json:"title"`
	Rect        winapi.Rect  `json:"rect"`
	After       *winapi.Rect `json:"after,omitempty"`
	Application string       `json:"application,omitempty"`
	Class       string       `json:"class,omitempty"`
	DesktopID   string       `json:"desktopId,omitempty"`
	Pinned      bool         `json:"pinned"`
}

type GeometryUndo struct {
	Version    int              `json:"version"`
	Records    []GeometryRecord `json:"records"`
	InProgress bool             `json:"inProgress,omitempty"`
	Uncertain  bool             `json:"uncertain,omitempty"`
}

func (store Store) LoadGeometry() (GeometryUndo, error) {
	var undo GeometryUndo
	if err := Read(filepath.Join(store.Directory, "window-geometry-undo.json"), &undo); err != nil {
		return undo, fmt.Errorf("annulation de disposition : %w", err)
	}
	if undo.Version != 1 && undo.Version != 2 {
		return GeometryUndo{}, fmt.Errorf("annulation de disposition : version invalide")
	}
	if err := ValidateGeometry(undo); err != nil {
		return GeometryUndo{}, err
	}
	return undo, nil
}

func (store Store) SaveGeometry(undo GeometryUndo) error {
	if undo.Version == 0 {
		undo.Version = 2
	}
	if err := ValidateGeometry(undo); err != nil {
		return err
	}
	return Write(filepath.Join(store.Directory, "window-geometry-undo.json"), undo)
}

func ValidateGeometry(undo GeometryUndo) error {
	if undo.Version != 1 && undo.Version != 2 {
		return fmt.Errorf("version du journal geometrique invalide")
	}
	seen := map[string]bool{}
	for _, record := range undo.Records {
		if record.WindowID == "" || seen[record.WindowID] || record.ProcessID == 0 || record.Started == "" ||
			record.Rect.Width <= 0 || record.Rect.Height <= 0 {
			return fmt.Errorf("identite ou rectangle invalide dans le journal geometrique")
		}
		seen[record.WindowID] = true
		if undo.Version == 2 && (record.Application == "" || record.Class == "" || record.DesktopID == "" ||
			record.After == nil || record.After.Width <= 0 || record.After.Height <= 0) {
			return fmt.Errorf("journal geometrique v2 incomplet")
		}
	}
	return nil
}
