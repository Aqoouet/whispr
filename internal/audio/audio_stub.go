//go:build !windows

package audio

import (
	"fmt"
	"os"
)

type Recorder struct{}

func NewRecorder(_ Options) (*Recorder, error) {
	return &Recorder{}, nil
}

func EnumerateFFmpegDevices(_ Options) ([]DeviceInfo, error) {
	return nil, nil
}

func (r *Recorder) Start() error {
	return fmt.Errorf("live microphone recording is only supported on Windows")
}

func (r *Recorder) Stop() (string, error) {
	return "", fmt.Errorf("live microphone recording is only supported on Windows")
}

func (r *Recorder) Close() error {
	return nil
}

func (r *Recorder) ActiveBackendDescription() string {
	return "backend=unsupported"
}

func EnsureWAVPath(path string) (string, error) {
	if _, err := os.Stat(path); err != nil {
		return "", err
	}
	return path, nil
}
