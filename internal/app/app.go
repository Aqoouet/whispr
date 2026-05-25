package app

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"corpdictation/internal/audio"
	"corpdictation/internal/config"
	"corpdictation/internal/logging"
	"corpdictation/internal/runtimecheck"
	"corpdictation/internal/whisper"
)

type Options struct {
	InputPath string
	Language  string
	BuildTime string
}

func Run(ctx context.Context, opts Options) error {
	root, err := config.LocalAppDataRoot()
	if err != nil && opts.InputPath == "" {
		return err
	}
	if root == "" {
		root = filepath.Join("staging", "windows-localappdata", "CorpDictation")
	}

	validation, err := runtimecheck.Validate(root, requireRuntimeDLLs(opts))
	if err != nil {
		return err
	}

	cfg := config.Default()
	configPath := filepath.Join(validation.Paths.ConfigDir, "config.json")
	if loaded, loadErr := config.Load(configPath); loadErr == nil {
		cfg = loaded
	}
	if opts.Language != "" {
		cfg.Language = opts.Language
	}

	logger, closeLog, err := logging.New(validation.Paths.LogsDir)
	if err != nil {
		return err
	}
	defer closeLog()

	backend := whisper.New()
	cudaAvailable, cudaErr := backend.CUDAAvailable(validation.Paths.RuntimeDir)
	if cudaErr != nil {
		logger.Printf("cuda probe failed: %v", cudaErr)
	}
	selection, err := runtimecheck.SelectModel(cfg, validation.AvailableModels, cudaAvailable)
	if err != nil {
		return err
	}

	audioOptions := audio.Options{
		PreferredInputDevice: cfg.PreferredInputDevice,
		FallbackInputDevice:  cfg.FallbackInputDevice,
		RuntimeDir:           validation.Paths.RuntimeDir,
		FFmpegPath:           cfg.FFmpegPath,
	}
	if opts.InputPath == "" {
		ffmpegDevices, enumErr := audio.EnumerateFFmpegDevices(audioOptions)
		if enumErr != nil {
			logger.Printf("capture device enumeration failed: %v", enumErr)
		} else {
			logger.Printf("capture devices: %s", audio.DescribeInputDevices(ffmpegDevices))
		}
		logger.Printf("capture selection: %s", audio.DescribeInputSelection(audioOptions))
	}

	logger.Printf("startup build=%s root=%s model=%s device=%s cuda_available=%t", opts.BuildTime, validation.Paths.Root, selection.ModelName, selection.Device, cudaAvailable)

	if opts.InputPath != "" {
		return runCLI(ctx, logger, backend, validation, cfg, selection, opts.InputPath)
	}
	return runWindowsLoop(ctx, logger, backend, validation, cfg, selection, audioOptions)
}

func runCLI(_ context.Context, logger *log.Logger, backend whisper.Backend, validation runtimecheck.Validation, cfg config.Config, selection runtimecheck.Selection, inputPath string) error {
	wavPath, err := audio.EnsureWAVPath(inputPath)
	if err != nil {
		return fmt.Errorf("validate input wav: %w", err)
	}
	text, err := backend.Transcribe(whisper.Request{
		RuntimeDir: validation.Paths.RuntimeDir,
		ModelPath:  selection.ModelPath,
		Device:     selection.Device,
		Language:   cfg.Language,
		BeamSize:   cfg.BeamSize,
		InputWAV:   wavPath,
	})
	if err != nil {
		return err
	}
	logger.Printf("cli transcription completed")
	fmt.Println(strings.TrimSpace(text))
	return nil
}
