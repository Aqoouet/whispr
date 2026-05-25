package audio

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const (
	captureBackendFFmpegDShow = "ffmpeg-dshow"
)

type Options struct {
	PreferredInputDevice string
	FallbackInputDevice  string
	RuntimeDir           string
	// FFmpegPath overrides automatic ffmpeg discovery. Takes priority over
	// bundled runtime\ffmpeg.exe and well-known Program Files candidates.
	FFmpegPath string
}

type DeviceInfo struct {
	ID         uint32
	Name       string
	EndpointID string
}

type openAttempt struct {
	Backend       string
	Detail        string
	Failure       string
	EndpointReset bool
}

type openFailure struct {
	Device      string
	DeviceCount uint32
	Attempts    []openAttempt
}

// inputDeviceRank orders capture devices: Game-style headset inputs before Chat/aux paths.
func inputDeviceRank(name string) int {
	n := strings.ToLower(name)
	switch {
	case strings.Contains(n, "chat"):
		return 2
	case strings.Contains(n, "game"):
		return 0
	default:
		return 1
	}
}

func reorderInputDevicesPreferGame(devices []DeviceInfo) []DeviceInfo {
	if len(devices) <= 1 {
		return devices
	}
	out := append([]DeviceInfo(nil), devices...)
	sort.SliceStable(out, func(i, j int) bool {
		return inputDeviceRank(out[i].Name) < inputDeviceRank(out[j].Name)
	})
	return out
}

func DescribeInputDevices(devices []DeviceInfo) string {
	if len(devices) == 0 {
		return "(none)"
	}
	parts := make([]string, 0, len(devices))
	for _, device := range devices {
		parts = append(parts, fmt.Sprintf("%d:%s", device.ID, displayDeviceName(device.Name)))
	}
	return strings.Join(parts, ", ")
}

func DescribeInputSelection(options Options) string {
	return fmt.Sprintf("preferred_input_device=%q fallback_input_device=%q runtime_dir=%q", options.PreferredInputDevice, options.FallbackInputDevice, options.RuntimeDir)
}

func normalizeDeviceName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func deviceIdentityKey(device DeviceInfo) string {
	if device.EndpointID != "" {
		return "endpoint:" + strings.ToLower(strings.TrimSpace(device.EndpointID))
	}
	return fmt.Sprintf("wavein:%d", device.ID)
}

func displayDeviceName(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		trimmed = "(unknown)"
	}
	return strconv.QuoteToASCII(trimmed)
}

func newOpenFailure(device string, deviceCount uint32, attempts []openAttempt) error {
	return &openFailure{Device: device, DeviceCount: deviceCount, Attempts: attempts}
}

func (e *openFailure) Error() string {
	if len(e.Attempts) == 0 {
		return fmt.Sprintf("audio capture init failed: device=%s, devices=%d, no backends attempted", displayDeviceName(e.Device), e.DeviceCount)
	}

	parts := make([]string, 0, len(e.Attempts))
	suggestReset := false
	for _, attempt := range e.Attempts {
		part := attempt.Backend
		if attempt.Detail != "" {
			part += "[" + attempt.Detail + "]"
		}
		if attempt.Failure != "" {
			part += "=" + attempt.Failure
		}
		parts = append(parts, part)
		suggestReset = suggestReset || attempt.EndpointReset
	}

	msg := fmt.Sprintf(
		"audio capture init failed: device=%s, devices=%d, attempts=%s",
		displayDeviceName(e.Device),
		e.DeviceCount,
		strings.Join(parts, " -> "),
	)
	if suggestReset {
		msg += "; microphone endpoint is present but failed to initialize; unplug/replug the USB headset, then restart Windows audio or reboot if needed"
	}
	if hint := openFailureRecoveryHint(e.Attempts); hint != "" {
		msg += "; " + hint
	}
	return msg
}

func openFailureRecoveryHint(attempts []openAttempt) string {
	for _, attempt := range attempts {
		if attempt.Backend != captureBackendFFmpegDShow {
			continue
		}
		lowerFailure := strings.ToLower(attempt.Failure)
		switch {
		case strings.Contains(lowerFailure, "bundled ffmpeg.exe missing"),
			strings.Contains(lowerFailure, "ffmpeg.exe not found"):
			return "bundled ffmpeg.exe is missing from runtime\\ffmpeg.exe; stage it there or install ffmpeg on PATH"
		case strings.Contains(lowerFailure, "directshow device not found"):
			return "configured microphone name did not match any ffmpeg DirectShow audio device; verify the saved device name against ffmpeg -list_devices output"
		case strings.Contains(lowerFailure, "directshow launch failed"),
			strings.Contains(lowerFailure, "directshow capture exited early"),
			strings.Contains(lowerFailure, "directshow device enumeration failed"),
			strings.Contains(lowerFailure, "directshow capture failed"):
			return "ffmpeg DirectShow capture failed; inspect the stderr summary in the attempt log and verify Windows microphone permissions"
		}
	}
	return ""
}
