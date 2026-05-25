package audio

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	waveMapper     = 0xFFFFFFFF
	waveFormDirect = 0x0008

	captureBackendWASAPI      = "wasapi"
	captureBackendWinMMMapper = "winmm-mapper"
	captureBackendWinMMDevice = "winmm-device"
	captureBackendDSound      = "dsound"
)

type Options struct {
	PreferredInputDevice string
	FallbackInputDevice  string
}

type DeviceInfo struct {
	ID         uint32
	Name       string
	EndpointID string
}

type formatCandidate struct {
	samplesPerSec uint32
	channels      uint16
	bits          uint16
}

var formatCandidates = []formatCandidate{
	{16000, 1, 16},
	{44100, 1, 16},
	{48000, 1, 16},
	{44100, 2, 16},
	{8000, 1, 16},
	{22050, 1, 16},
	{11025, 1, 16},
}

type openAttemptSpec struct {
	Backend    string
	Detail     string
	Format     formatCandidate
	DeviceID   uint32
	DeviceName string
	Flags      uint32
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

type deviceSelection struct {
	targets   []DeviceInfo
	remaining []DeviceInfo
}

func buildWinMMOpenPlan(selection deviceSelection, candidates []formatCandidate) []openAttemptSpec {
	attempts := make([]openAttemptSpec, 0, len(candidates)*(2*(len(selection.targets)+len(selection.remaining)+1)))
	for _, fc := range candidates {
		detail := formatDetail(fc)
		for _, device := range selection.targets {
			attempts = append(attempts,
				openAttemptSpec{Backend: captureBackendWinMMDevice, Detail: formatDeviceDetail(detail, device, false), Format: fc, DeviceID: device.ID, DeviceName: device.Name, Flags: 0},
				openAttemptSpec{Backend: captureBackendWinMMDevice, Detail: formatDeviceDetail(detail, device, true), Format: fc, DeviceID: device.ID, DeviceName: device.Name, Flags: waveFormDirect},
			)
		}
		attempts = append(attempts,
			openAttemptSpec{Backend: captureBackendWinMMMapper, Detail: detail + " mapper", Format: fc, DeviceID: waveMapper, Flags: 0},
			openAttemptSpec{Backend: captureBackendWinMMMapper, Detail: detail + " mapper-direct", Format: fc, DeviceID: waveMapper, Flags: waveFormDirect},
		)
		for _, device := range selection.remaining {
			attempts = append(attempts,
				openAttemptSpec{Backend: captureBackendWinMMDevice, Detail: formatDeviceDetail(detail, device, false), Format: fc, DeviceID: device.ID, DeviceName: device.Name, Flags: 0},
				openAttemptSpec{Backend: captureBackendWinMMDevice, Detail: formatDeviceDetail(detail, device, true), Format: fc, DeviceID: device.ID, DeviceName: device.Name, Flags: waveFormDirect},
			)
		}
	}
	return attempts
}

func formatDetail(fc formatCandidate) string {
	return fmt.Sprintf("%dHz/%dch/%dbit", fc.samplesPerSec, fc.channels, fc.bits)
}

func formatDeviceDetail(format string, device DeviceInfo, direct bool) string {
	if direct {
		return fmt.Sprintf("%s device=%d name=%s direct", format, device.ID, displayDeviceName(device.Name))
	}
	return fmt.Sprintf("%s device=%d name=%s", format, device.ID, displayDeviceName(device.Name))
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
	return fmt.Sprintf("preferred_input_device=%q fallback_input_device=%q", options.PreferredInputDevice, options.FallbackInputDevice)
}

func resolveInputDeviceSelection(devices []DeviceInfo, options Options) (deviceSelection, error) {
	used := make(map[string]bool, len(devices))
	targets := make([]DeviceInfo, 0, 2)

	for _, configured := range []string{options.PreferredInputDevice, options.FallbackInputDevice} {
		if strings.TrimSpace(configured) == "" {
			continue
		}
		device, ok := findDeviceByName(devices, configured)
		if !ok {
			return deviceSelection{}, fmt.Errorf(
				"audio capture init failed: configured input device %q not found; available devices=%s",
				configured,
				DescribeInputDevices(devices),
			)
		}
		key := deviceIdentityKey(device)
		if !used[key] {
			targets = append(targets, device)
			used[key] = true
		}
	}

	remaining := make([]DeviceInfo, 0, len(devices))
	for _, device := range devices {
		if !used[deviceIdentityKey(device)] {
			remaining = append(remaining, device)
		}
	}

	return deviceSelection{targets: targets, remaining: remaining}, nil
}

func selectFailureDeviceLabel(selection deviceSelection, devices []DeviceInfo) string {
	if len(selection.targets) > 0 {
		return selection.targets[0].Name
	}
	if len(devices) == 1 {
		return devices[0].Name
	}
	return "(default mapper)"
}

func findDeviceByName(devices []DeviceInfo, target string) (DeviceInfo, bool) {
	normalizedTarget := normalizeDeviceName(target)
	for _, device := range devices {
		if normalizeDeviceName(device.Name) == normalizedTarget {
			return device, true
		}
	}
	return DeviceInfo{}, false
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
	return msg
}
