package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"corpdictation/internal/app"
)

var BuildTime = "dev"

func main() {
	var input string
	var language string
	flag.StringVar(&input, "input", "", "path to a WAV file for development transcription")
	flag.StringVar(&language, "language", "ru", "language code for development transcription")
	flag.Parse()

	ctx := context.Background()
	if err := app.Run(ctx, app.Options{
		InputPath: input,
		Language:  language,
		BuildTime: BuildTime,
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
