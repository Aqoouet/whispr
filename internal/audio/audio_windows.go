//go:build windows

package audio

import (
	"fmt"
	"strings"
)

const (
	waveFormatPCM        = 1
	recorderModeFFmpegDShow = "ffmpeg-dshow"
)

type waveFormatEx struct {
	FormatTag      uint16
	Channels       uint16
	SamplesPerSec  uint32
	AvgBytesPerSec uint32
	BlockAlign     uint16
	BitsPerSample  uint16
	Size           uint16
}

type Recorder struct {
	options      Options
	mode         string
	format       waveFormatEx
	ffmpeg       *ffmpegSession
	activeDetail string
	active       bool
}

func NewRecorder(options Options) (*Recorder, error) {
	return &Recorder{options: options}, nil
}

func EnumerateFFmpegDevices(options Options) ([]DeviceInfo, error) {
	ffmpegPath, _, err := findFFmpegExecutable(options.FFmpegPath, options.RuntimeDir)
	if err != nil {
		return nil, fmt.Errorf("ffmpeg not found: %w", err)
	}
	names, err := listFFmpegDShowAudioDevices(ffmpegPath)
	if err != nil {
		return nil, fmt.Errorf("ffmpeg device list: %w", err)
	}
	return ffmpegDShowDevicesAsInfo(names), nil
}

func (r *Recorder) Start() error {
	if r.active {
		return fmt.Errorf("recording already active")
	}
	attempts, err := r.startFFmpegDShow()
	if err != nil {
		if r.ffmpeg != nil {
			cleanupFFmpegSession(r.ffmpeg)
			r.ffmpeg = nil
		}
		return newOpenFailure("(default)", 0, attempts)
	}
	r.active = true
	return nil
}

func (r *Recorder) Stop() (string, error) {
	if !r.active {
		return "", fmt.Errorf("recording is not active")
	}
	path, err := r.ffmpegStop()
	r.active = false
	if err != nil {
		return "", fmt.Errorf("ffmpeg-dshow stop: %w", err)
	}
	return path, nil
}

func (r *Recorder) Close() error {
	if r.active {
		_, _ = r.Stop()
	}
	if r.ffmpeg != nil {
		cleanupFFmpegSession(r.ffmpeg)
		r.ffmpeg = nil
	}
	return nil
}

func EnsureWAVPath(path string) (string, error) {
	return path, nil
}

func (r *Recorder) ActiveBackendDescription() string {
	if strings.TrimSpace(r.activeDetail) == "" {
		return "backend=(unknown)"
	}
	return r.activeDetail
}
