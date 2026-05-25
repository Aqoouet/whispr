//go:build !windows

package whisper

import (
	"fmt"
	"os/exec"
	"strings"
)

type backend struct{}

func New() Backend {
	return backend{}
}

func (backend) CUDAAvailable(string) (bool, error) {
	return false, nil
}

func (backend) Transcribe(req Request) (string, error) {
	cli := "staging/linux/bin/whisper-cli"
	cmd := exec.Command(cli, "-m", req.ModelPath, "-l", req.Language, "-bs", fmt.Sprint(req.BeamSize), "-f", req.InputWAV, "-nt", "-np")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("run linux whisper backend %s: %w: %s", cli, err, strings.TrimSpace(string(out)))
	}
	text := strings.TrimSpace(string(out))
	if text == "" {
		return "", fmt.Errorf("linux whisper backend returned empty output")
	}
	return text, nil
}
