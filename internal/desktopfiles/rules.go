package desktopfiles

import (
	"encoding/json"
	"os"
	"strings"
)

// DefaultRules fournit les règles par défaut standardisées.
func DefaultRules() RulesConfig {
	return RulesConfig{
		Categories: map[string][]string{
			"Documents": {
				".pdf", ".docx", ".doc", ".xlsx", ".xls", ".pptx", ".txt", ".md", ".csv", ".odt",
			},
			"Images": {
				".png", ".jpg", ".jpeg", ".gif", ".svg", ".bmp", ".webp", ".ico",
			},
			"Archives": {
				".zip", ".tar", ".gz", ".7z", ".rar",
			},
			"Installateurs": {
				".exe", ".msi", ".iso",
			},
			"Raccourcis": {
				".lnk", ".url",
			},
			"Autres": {},
		},
		FallbackCategory: "Autres",
	}
}

// LoadRules charge la configuration des règles depuis un fichier JSON.
func LoadRules(path string) (RulesConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return RulesConfig{}, err
	}

	var rules RulesConfig
	if err := json.Unmarshal(data, &rules); err != nil {
		return RulesConfig{}, err
	}

	if rules.FallbackCategory == "" {
		rules.FallbackCategory = "Autres"
	}

	return rules, nil
}

// MatchCategory détermine la catégorie d'un fichier d'après son extension.
func MatchCategory(ext string, rules RulesConfig) string {
	ext = strings.ToLower(strings.TrimSpace(ext))
	for category, extensions := range rules.Categories {
		for _, e := range extensions {
			if strings.EqualFold(ext, strings.TrimSpace(e)) {
				return category
			}
		}
	}
	return rules.FallbackCategory
}
