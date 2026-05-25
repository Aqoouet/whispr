//go:build !windows

package app

import (
	"context"
	"fmt"
	"log"

	"corpdictation/internal/audio"
	"corpdictation/internal/config"
	"corpdictation/internal/runtimecheck"
	"corpdictation/internal/whisper"
)

func runWindowsLoop(context.Context, *log.Logger, whisper.Backend, runtimecheck.Validation, config.Config, runtimecheck.Selection, audio.Options) error {
	return fmt.Errorf("live dictation mode is only supported on Windows; use --input on Linux for development validation")
}
