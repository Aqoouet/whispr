//go:build windows

package app

import (
	"context"
	"log"
	"os"
	"strings"
	"time"

	"corpdictation/internal/audio"
	"corpdictation/internal/clipboard"
	"corpdictation/internal/config"
	"corpdictation/internal/hotkey"
	"corpdictation/internal/paste"
	"corpdictation/internal/runtimecheck"
	"corpdictation/internal/tray"
	"corpdictation/internal/whisper"
)

func runWindowsLoop(ctx context.Context, logger *log.Logger, backend whisper.Backend, validation runtimecheck.Validation, cfg config.Config, selection runtimecheck.Selection, audioOptions audio.Options) error {
	ui, err := tray.New()
	if err != nil {
		return err
	}
	defer ui.Close()
	ui.SetStatus("Idle")

	listener, err := hotkey.Register(cfg.Hotkey)
	if err != nil {
		return err
	}
	defer listener.Close()

	recorder, err := audio.NewRecorder(audioOptions)
	if err != nil {
		return err
	}
	defer recorder.Close()

	recording := false
	events := listener.Events()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ui.WaitForExit():
			return nil
		case <-events:
			if !recording {
				if err := recorder.Start(); err != nil {
					ui.SetStatus("Error")
					ui.Notify("CorpDictation", err.Error())
					logger.Printf("record start failed: %v", err)
					continue
				}
				recording = true
				ui.SetStatus("Recording")
				continue
			}

			recording = false
			ui.SetStatus("Transcribing")
			wavPath, err := recorder.Stop()
			if err != nil {
				ui.SetStatus("Error")
				ui.Notify("CorpDictation", err.Error())
				logger.Printf("record stop failed: %v", err)
				continue
			}
			text, err := backend.Transcribe(whisper.Request{
				RuntimeDir: validation.Paths.RuntimeDir,
				ModelPath:  selection.ModelPath,
				Device:     selection.Device,
				Language:   cfg.Language,
				BeamSize:   cfg.BeamSize,
				InputWAV:   wavPath,
			})
			if !cfg.SaveAudio {
				_ = os.Remove(wavPath)
			}
			if err != nil {
				ui.SetStatus("Error")
				ui.Notify("CorpDictation", err.Error())
				logger.Printf("transcription failed: %v", err)
				continue
			}
			if err := clipboard.SetText(strings.TrimSpace(text)); err != nil {
				ui.SetStatus("Error")
				ui.Notify("CorpDictation", err.Error())
				logger.Printf("clipboard failed: %v", err)
				continue
			}
			if cfg.AutoPaste {
				time.Sleep(100 * time.Millisecond)
				if err := paste.CtrlV(); err != nil {
					ui.SetStatus("Error")
					ui.Notify("CorpDictation", err.Error())
					logger.Printf("paste failed: %v", err)
					continue
				}
			}
			ui.SetStatus("Idle")
			logger.Printf("transcription completed")
		}
	}
}
