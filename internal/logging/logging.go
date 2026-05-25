package logging

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
)

func New(logDir string) (*log.Logger, func() error, error) {
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, nil, fmt.Errorf("create log dir: %w", err)
	}
	path := filepath.Join(logDir, "app.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, nil, fmt.Errorf("open log file: %w", err)
	}
	return log.New(f, "", log.LstdFlags|log.LUTC), f.Close, nil
}
