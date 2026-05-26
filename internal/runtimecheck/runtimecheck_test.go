package runtimecheck

import (
	"path/filepath"
	"testing"

	"corpdictation/internal/config"
)

func TestSelectModelPrefersCUDAPreferredModel(t *testing.T) {
	cfg := config.Default()
	available := map[string]string{
		cfg.PreferredModel:   "/models/preferred.bin",
		cfg.FallbackModel:    "/models/fallback.bin",
		cfg.CPUFallbackModel: "/models/cpu.bin",
	}
	got, err := SelectModel(cfg, available, true)
	if err != nil {
		t.Fatal(err)
	}
	if got.Device != "cuda" || got.ModelName != cfg.PreferredModel {
		t.Fatalf("got %+v, want cuda/%s", got, cfg.PreferredModel)
	}
}

func TestSelectModelFallsBackToCPU(t *testing.T) {
	cfg := config.Default()
	available := map[string]string{
		cfg.CPUFallbackModel: "/models/cpu.bin",
	}
	got, err := SelectModel(cfg, available, false)
	if err != nil {
		t.Fatal(err)
	}
	if got.Device != "cpu" || got.ModelName != cfg.CPUFallbackModel {
		t.Fatalf("got %+v, want cpu/%s", got, cfg.CPUFallbackModel)
	}
}

func TestResolveDefaultPathsMatchesConfigRootPolicy(t *testing.T) {
	dir := t.TempDir()
	policy := config.RootPolicy{
		Deployment: true,
		GoOS:       "windows",
		Getenv: func(key string) string {
			switch key {
			case "CORPDICTATION_ROOT":
				return filepath.Join(dir, "override")
			case "LOCALAPPDATA":
				return filepath.Join(dir, "local")
			default:
				return ""
			}
		},
	}
	paths, err := ResolveDefaultPaths(policy)
	if err != nil {
		t.Fatal(err)
	}
	configPath, err := config.ConfigPathForPolicy(policy)
	if err != nil {
		t.Fatal(err)
	}
	if paths.Root != filepath.Join(dir, "override") {
		t.Fatalf("root = %q", paths.Root)
	}
	if filepath.Join(paths.ConfigDir, "config.json") != configPath {
		t.Fatalf("config path = %q, want %q", filepath.Join(paths.ConfigDir, "config.json"), configPath)
	}
	if paths.RuntimeDir != filepath.Join(paths.Root, "runtime") {
		t.Fatalf("runtime dir = %q", paths.RuntimeDir)
	}
	if paths.ModelsDir != filepath.Join(paths.Root, "models") {
		t.Fatalf("models dir = %q", paths.ModelsDir)
	}
	if paths.LogsDir != filepath.Join(paths.Root, "logs") {
		t.Fatalf("logs dir = %q", paths.LogsDir)
	}
}
