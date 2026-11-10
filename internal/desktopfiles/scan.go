package desktopfiles

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// ScanDesktop recense les fichiers éligibles du Bureau utilisateur.
// Protections :
// - Ignore les répertoires.
// - Ignore desktop.ini, éléments cachés et système.
// - Ne traverse pas les liens symboliques ou jonctions.
// - Ne modifie pas le Bureau public.
func ScanDesktop(desktopPath string) ([]ScannedFile, error) {
	entries, err := os.ReadDir(desktopPath)
	if err != nil {
		return nil, err
	}

	var files []ScannedFile

	for _, entry := range entries {
		name := entry.Name()

		// 1. Ignorer explicitement desktop.ini
		if strings.EqualFold(name, "desktop.ini") {
			continue
		}

		// 2. Ignorer les répertoires existants
		if entry.IsDir() {
			continue
		}

		fullPath := filepath.Join(desktopPath, name)

		// 3. Inspection via Lstat pour ne pas suivre les symlinks/jonctions
		fi, err := os.Lstat(fullPath)
		if err != nil {
			continue
		}

		// Si c'est un lien symbolique de système de fichiers (NTFS reparse point)
		if fi.Mode()&os.ModeSymlink != 0 {
			// On ne traverse pas le lien symbolique
			continue
		}

		// 4. Ignorer les fichiers cachés et système via attributs Windows
		if winData, ok := fi.Sys().(*syscall.Win32FileAttributeData); ok {
			const (
				FILE_ATTRIBUTE_HIDDEN = 0x00000002
				FILE_ATTRIBUTE_SYSTEM = 0x00000004
			)
			if winData.FileAttributes&(FILE_ATTRIBUTE_HIDDEN|FILE_ATTRIBUTE_SYSTEM) != 0 {
				continue
			}
		}

		files = append(files, ScannedFile{
			FileName:  name,
			Path:      fullPath,
			Size:      fi.Size(),
			ModTime:   fi.ModTime(),
			Extension: strings.ToLower(filepath.Ext(name)),
		})
	}

	return files, nil
}
