package runtimecheck

import (
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
