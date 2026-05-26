package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfigHasExpectedRussianDefaults(t *testing.T) {
	cfg := Default()
	if cfg.Language != "ru" {
		t.Fatalf("Language = %q, want ru", cfg.Language)
	}
	if cfg.PreferredDevice != "cuda" {
		t.Fatalf("PreferredDevice = %q, want cuda", cfg.PreferredDevice)
	}
	if cfg.CPUFallbackModel != "small-q5_1" {
		t.Fatalf("CPUFallbackModel = %q, want small-q5_1", cfg.CPUFallbackModel)
	}
	if !cfg.AutoPaste {
		t.Fatal("AutoPaste = false, want true")
	}
	if cfg.PreferredInputDevice != "" {
		t.Fatalf("PreferredInputDevice = %q, want empty", cfg.PreferredInputDevice)
	}
}

func TestLoadMergesDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := Write(path, Config{Language: "ru", BeamSize: 0}); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BeamSize != 1 {
		t.Fatalf("BeamSize = %d, want 1", cfg.BeamSize)
	}
	if cfg.Hotkey != "Ctrl+Alt+Space" {
		t.Fatalf("Hotkey = %q, want Ctrl+Alt+Space", cfg.Hotkey)
	}
}

func TestLoadPreservesPreferredInputDevice(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := Write(path, Config{
		Language:             "ru",
		PreferredInputDevice: "Arctis Pro Wireless Chat",
		FallbackInputDevice:  "HP USB Media Audio",
	}); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PreferredInputDevice != "Arctis Pro Wireless Chat" {
		t.Fatalf("PreferredInputDevice = %q", cfg.PreferredInputDevice)
	}
	if cfg.FallbackInputDevice != "HP USB Media Audio" {
		t.Fatalf("FallbackInputDevice = %q", cfg.FallbackInputDevice)
	}
}

func TestResolveRuntimeRootPrefersOverrideOverLocalAppData(t *testing.T) {
	dir := t.TempDir()
	root, err := ResolveRuntimeRoot(RootPolicy{
		Deployment: true,
		GoOS:       "windows",
		Getenv: func(key string) string {
			switch key {
			case "CORPDICTATION_ROOT":
				return filepath.Join(dir, "override")
			case "LOCALAPPDATA":
				return filepath.Join(dir, "local")
			case "ProgramData":
				return filepath.Join(dir, "programdata")
			default:
				return ""
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "override")
	if root != want {
		t.Fatalf("root = %q, want %q", root, want)
	}
}

func TestResolveRuntimeRootUsesMachineRootWhenPresent(t *testing.T) {
	dir := t.TempDir()
	machineRoot := filepath.Join(dir, "programdata", "CorpDictation")
	if err := os.MkdirAll(machineRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	root, err := ResolveRuntimeRoot(RootPolicy{
		Deployment: true,
		GoOS:       "windows",
		Getenv: func(key string) string {
			switch key {
			case "ProgramData":
				return filepath.Join(dir, "programdata")
			case "LOCALAPPDATA":
				return filepath.Join(dir, "local")
			default:
				return ""
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if root != machineRoot {
		t.Fatalf("root = %q, want %q", root, machineRoot)
	}
}

func TestResolveRuntimeRootFallsBackToLocalAppData(t *testing.T) {
	dir := t.TempDir()
	root, err := ResolveRuntimeRoot(RootPolicy{
		Deployment: true,
		GoOS:       "windows",
		Getenv: func(key string) string {
			switch key {
			case "LOCALAPPDATA":
				return filepath.Join(dir, "local")
			case "ProgramData":
				return filepath.Join(dir, "missing-programdata")
			default:
				return ""
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "local", "CorpDictation")
	if root != want {
		t.Fatalf("root = %q, want %q", root, want)
	}
}

func TestResolveRuntimeRootUsesStagingFallbackForNonWindowsCLI(t *testing.T) {
	root, err := ResolveRuntimeRoot(RootPolicy{
		AllowStagingFallback: true,
		GoOS:                 "linux",
		Getenv:               func(string) string { return "" },
	})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join("staging", "windows-localappdata", "CorpDictation")
	if root != want {
		t.Fatalf("root = %q, want %q", root, want)
	}
}
