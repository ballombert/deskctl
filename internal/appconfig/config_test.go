package appconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingConfigUsesDefaults(t *testing.T) {
	config, err := Load(filepath.Join(t.TempDir(), "deskctl.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if config.Version != 1 || config.FancyWM.Executable != "fancywm" {
		t.Fatalf("unexpected defaults: %+v", config)
	}
}

func TestLoadConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deskctl.yaml")
	data := []byte("version: 1\nfancywm:\n  executable: C:/Tools/fancywm.exe\n  display_indices:\n    \\\\.\\DISPLAY2: 1\n")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}

	config, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if config.FancyWM.Executable != "C:/Tools/fancywm.exe" || config.FancyWM.DisplayIndices[`\\.\DISPLAY2`] != 1 {
		t.Fatalf("configuration was not loaded: %+v", config)
	}
}

func TestRepositoryConfigLoads(t *testing.T) {
	config, err := Load(filepath.Join("..", "..", "deskctl.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if config.Version != 1 || config.FancyWM.Executable != "fancywm" || len(config.FancyWM.DisplayIndices) != 0 {
		t.Fatalf("unexpected repository configuration: %+v", config)
	}
}

func TestLoadRejectsUnknownAndInvalidValues(t *testing.T) {
	for _, data := range []string{
		"version: 1\nunknown: true\n",
		"version: 2\n",
		"version: 1\nfancywm:\n  display_indices:\n    DISPLAY1: 10\n",
		"version: 1\nfancywm:\n  display_indices:\n    DISPLAY1: 1\n    DISPLAY2: 1\n",
		"version: 1\n---\nversion: 1\n",
	} {
		t.Run(data, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "deskctl.yaml")
			if err := os.WriteFile(path, []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(path); err == nil {
				t.Fatal("expected invalid configuration to be rejected")
			}
		})
	}
}

func TestSaveRoundTripsAndValidatesTiling(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deskctl.yaml")
	config := Default()
	config.FancyWM.DisplayIndices[`\\.\DISPLAY2`] = 2
	config.Tiling = Tiling{Engine: "native", Gap: 4}
	if err := Save(path, config); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil || loaded.Tiling != config.Tiling || loaded.FancyWM.DisplayIndices[`\\.\DISPLAY2`] != 2 {
		t.Fatal("saved configuration must load back", err, loaded)
	}
	config.Tiling.Engine = "magic"
	if err := Save(path, config); err == nil {
		t.Fatal("unknown tiling engine must be refused")
	}
	if err := os.WriteFile(path, []byte("version: 1\nfancywm:\n  executable: fancywm\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if loaded, err := Load(path); err != nil || loaded.Tiling.Engine != "auto" || loaded.Tiling.Gap != 8 {
		t.Fatal("missing tiling section must use defaults", err, loaded.Tiling)
	}
}
