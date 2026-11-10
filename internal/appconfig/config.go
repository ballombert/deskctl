package appconfig

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v3"
)

const maxConfigSize = 64 << 10

type Config struct {
	Version int     `yaml:"version" json:"version"`
	FancyWM FancyWM `yaml:"fancywm" json:"fancyWM"`
	Tiling  Tiling  `yaml:"tiling" json:"tiling"`
}

// Tiling chooses how window groups are laid out: auto (FancyWM, native fallback), fancywm or native.
type Tiling struct {
	Engine string `yaml:"engine" json:"engine"`
	Gap    int32  `yaml:"gap" json:"gap"`
}

type FancyWM struct {
	Executable     string         `yaml:"executable" json:"executable"`
	DisplayIndices map[string]int `yaml:"display_indices" json:"displayIndices"`
}

func Default() Config {
	return Config{
		Version: 1,
		FancyWM: FancyWM{
			Executable:     "fancywm",
			DisplayIndices: map[string]int{},
		},
		Tiling: Tiling{Engine: "auto", Gap: 8},
	}
}

func Load(path string) (Config, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return Default(), nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("ouvrir la configuration %s : %w", path, err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return Config{}, fmt.Errorf("lire les métadonnées de configuration %s : %w", path, err)
	}
	if info.Size() > maxConfigSize {
		return Config{}, fmt.Errorf("configuration trop volumineuse : %s", path)
	}

	config := Default()
	decoder := yaml.NewDecoder(io.LimitReader(file, maxConfigSize+1))
	decoder.KnownFields(true)
	if err := decoder.Decode(&config); err != nil {
		return Config{}, fmt.Errorf("configuration YAML invalide %s : %w", path, err)
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return Config{}, fmt.Errorf("document YAML supplémentaire interdit : %s", path)
	}
	if err := config.Validate(); err != nil {
		return Config{}, fmt.Errorf("configuration invalide %s : %w", path, err)
	}
	return config, nil
}

func (config Config) Validate() error {
	if config.Version != 1 {
		return fmt.Errorf("version non prise en charge : %d", config.Version)
	}
	if config.FancyWM.Executable == "" {
		return fmt.Errorf("fancywm.executable ne peut pas être vide")
	}
	switch config.Tiling.Engine {
	case "auto", "fancywm", "native":
	default:
		return fmt.Errorf("tiling.engine doit valoir auto, fancywm ou native")
	}
	if config.Tiling.Gap < 0 || config.Tiling.Gap > 64 {
		return fmt.Errorf("tiling.gap doit être compris entre 0 et 64")
	}
	indices := map[int]bool{}
	for deviceID, index := range config.FancyWM.DisplayIndices {
		if deviceID == "" || index < 1 || index > 9 {
			return fmt.Errorf("fancywm.display_indices doit associer un identifiant d’écran à un indice entre 1 et 9")
		}
		if indices[index] {
			return fmt.Errorf("indice FancyWM %d associé à plusieurs écrans", index)
		}
		indices[index] = true
	}
	return nil
}

// Save validates and atomically writes the configuration. YAML comments are not kept.
func Save(path string, config Config) error {
	if err := config.Validate(); err != nil {
		return err
	}
	data, err := yaml.Marshal(config)
	if err != nil {
		return err
	}
	data = append([]byte("# Configuration deskctl (réécrite par l'interface ; les commentaires ne sont pas conservés)\n"), data...)
	temp, err := os.CreateTemp(filepath.Dir(path), ".deskctl-*.yaml")
	if err != nil {
		return fmt.Errorf("dossier de configuration non accessible en écriture : %w", err)
	}
	defer os.Remove(temp.Name())
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), path)
}
