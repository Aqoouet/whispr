package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	Language             string `json:"language"`
	PreferredDevice      string `json:"preferred_device"`
	FallbackDevice       string `json:"fallback_device"`
	PreferredInputDevice string `json:"preferred_input_device"`
	FallbackInputDevice  string `json:"fallback_input_device"`
	PreferredModel       string `json:"preferred_model"`
	FallbackModel        string `json:"fallback_model"`
	CPUFallbackModel     string `json:"cpu_fallback_model"`
	Hotkey               string `json:"hotkey"`
	AutoPaste            bool   `json:"auto_paste"`
	SaveAudio            bool   `json:"save_audio"`
	LogTranscripts       bool   `json:"log_transcripts"`
	BeamSize             int    `json:"beam_size"`
	// FFmpegPath overrides bundled ffmpeg.exe. Use when group policy blocks
	// execution from AppData\Local — point to a Program Files installation.
	FFmpegPath string `json:"ffmpeg_path,omitempty"`
}

func Default() Config {
	return Config{
		Language:             "ru",
		PreferredDevice:      "cuda",
		FallbackDevice:       "cpu",
		PreferredInputDevice: "",
		FallbackInputDevice:  "",
		PreferredModel:       "large-v3-turbo-q5_0",
		FallbackModel:        "medium-q5_0",
		CPUFallbackModel:     "small-q5_1",
		Hotkey:               "Ctrl+Alt+Space",
		AutoPaste:            true,
		SaveAudio:            false,
		LogTranscripts:       false,
		BeamSize:             1,
	}
}

func LocalAppDataRoot() (string, error) {
	if v := os.Getenv("LOCALAPPDATA"); v != "" {
		return filepath.Join(v, "CorpDictation"), nil
	}
	if v := os.Getenv("CORPDICTATION_ROOT"); v != "" {
		return v, nil
	}
	return "", fmt.Errorf("LOCALAPPDATA or CORPDICTATION_ROOT is not set")
}

func ConfigPath() (string, error) {
	root, err := LocalAppDataRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "config", "config.json"), nil
}

func LoadDefaultPath() (Config, string, error) {
	path, err := ConfigPath()
	if err != nil {
		return Config{}, "", err
	}
	cfg, err := Load(path)
	return cfg, path, err
}

func Load(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	cfg := Default()
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	if cfg.Language == "" {
		cfg.Language = "ru"
	}
	if cfg.PreferredDevice == "" {
		cfg.PreferredDevice = "cuda"
	}
	if cfg.FallbackDevice == "" {
		cfg.FallbackDevice = "cpu"
	}
	if cfg.PreferredModel == "" {
		cfg.PreferredModel = "large-v3-turbo-q5_0"
	}
	if cfg.FallbackModel == "" {
		cfg.FallbackModel = "medium-q5_0"
	}
	if cfg.CPUFallbackModel == "" {
		cfg.CPUFallbackModel = "small-q5_1"
	}
	if cfg.Hotkey == "" {
		cfg.Hotkey = "Ctrl+Alt+Space"
	}
	if cfg.BeamSize <= 0 {
		cfg.BeamSize = 1
	}
	return cfg, nil
}

func Write(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}
