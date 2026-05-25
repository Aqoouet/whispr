package config

import (
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
