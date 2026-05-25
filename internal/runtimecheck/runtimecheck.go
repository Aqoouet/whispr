package runtimecheck

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"corpdictation/internal/config"
)

type RuntimePaths struct {
	Root       string
	RuntimeDir string
	ModelsDir  string
	ConfigDir  string
	LogsDir    string
}

type Selection struct {
	Device    string
	ModelName string
	ModelPath string
}

type Validation struct {
	Paths           RuntimePaths
	AvailableModels map[string]string
	RuntimeDLLs     []string
}

func ResolvePaths(root string) RuntimePaths {
	return RuntimePaths{
		Root:       root,
		RuntimeDir: filepath.Join(root, "runtime"),
		ModelsDir:  filepath.Join(root, "models"),
		ConfigDir:  filepath.Join(root, "config"),
		LogsDir:    filepath.Join(root, "logs"),
	}
}

func Validate(root string, requireDLLs bool) (Validation, error) {
	paths := ResolvePaths(root)
	for _, dir := range []string{paths.Root, paths.RuntimeDir, paths.ModelsDir, paths.ConfigDir, paths.LogsDir} {
		if stat, err := os.Stat(dir); err != nil || !stat.IsDir() {
			return Validation{}, fmt.Errorf("required directory missing: %s", dir)
		}
	}

	models, err := discoverModels(paths.ModelsDir)
	if err != nil {
		return Validation{}, err
	}
	if len(models) == 0 {
		return Validation{}, fmt.Errorf("no Whisper model files found in %s", paths.ModelsDir)
	}

	runtimeDLLs, err := discoverDLLs(paths.RuntimeDir)
	if err != nil {
		return Validation{}, err
	}
	if requireDLLs && len(runtimeDLLs) == 0 {
		return Validation{}, fmt.Errorf("no runtime DLL files found in %s", paths.RuntimeDir)
	}

	return Validation{
		Paths:           paths,
		AvailableModels: models,
		RuntimeDLLs:     runtimeDLLs,
	}, nil
}

func SelectModel(cfg config.Config, available map[string]string, cudaAvailable bool) (Selection, error) {
	if cudaAvailable {
		if path, ok := available[cfg.PreferredModel]; ok {
			return Selection{Device: "cuda", ModelName: cfg.PreferredModel, ModelPath: path}, nil
		}
		if path, ok := available[cfg.FallbackModel]; ok {
			return Selection{Device: "cuda", ModelName: cfg.FallbackModel, ModelPath: path}, nil
		}
	}

	if path, ok := available[cfg.CPUFallbackModel]; ok {
		return Selection{Device: "cpu", ModelName: cfg.CPUFallbackModel, ModelPath: path}, nil
	}
	if path, ok := available[cfg.FallbackModel]; ok {
		return Selection{Device: "cpu", ModelName: cfg.FallbackModel, ModelPath: path}, nil
	}
	if path, ok := available[cfg.PreferredModel]; ok {
		return Selection{Device: "cpu", ModelName: cfg.PreferredModel, ModelPath: path}, nil
	}

	return Selection{}, fmt.Errorf("no configured model is available")
}

func discoverModels(dir string) (map[string]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read models dir: %w", err)
	}
	out := map[string]string{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, "ggml-") && strings.HasSuffix(name, ".bin") {
			key := strings.TrimSuffix(strings.TrimPrefix(name, "ggml-"), ".bin")
			out[key] = filepath.Join(dir, name)
		}
	}
	return out, nil
}

func discoverDLLs(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read runtime dir: %w", err)
	}
	var out []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasSuffix(strings.ToLower(entry.Name()), ".dll") {
			out = append(out, filepath.Join(dir, entry.Name()))
		}
	}
	return out, nil
}
