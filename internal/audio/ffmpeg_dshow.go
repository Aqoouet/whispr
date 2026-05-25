package audio

import (
	"fmt"
	"strings"
)

const (
	ffmpegCaptureSampleRate = 44100
	ffmpegCaptureChannels   = 2
	ffmpegCaptureBits       = 16
)

func parseFFmpegDShowAudioDevices(output string) []string {
	lines := strings.Split(output, "\n")
	devices := make([]string, 0, len(lines))
	seen := make(map[string]bool, len(lines))
	inAudioSection := false
	for _, line := range lines {
		// New format: section headers delimit audio vs video devices.
		if strings.Contains(line, "DirectShow audio devices") {
			inAudioSection = true
			continue
		}
		if strings.Contains(line, "DirectShow video devices") {
			inAudioSection = false
			continue
		}
		// Old format: "(audio)" tag on device line.
		if strings.Contains(line, "(audio)") {
			inAudioSection = false
			if device, ok := parseQuotedDeviceName(line); ok {
				key := normalizeFFmpegDeviceName(device)
				if !seen[key] {
					devices = append(devices, device)
					seen[key] = true
				}
			}
			continue
		}
		// New format: device name lines inside audio section (skip "Alternative name").
		if inAudioSection && !strings.Contains(line, "Alternative name") {
			if device, ok := parseQuotedDeviceName(line); ok {
				key := normalizeFFmpegDeviceName(device)
				if !seen[key] {
					devices = append(devices, device)
					seen[key] = true
				}
			}
		}
	}
	return devices
}

func parseQuotedDeviceName(line string) (string, bool) {
	start := strings.IndexByte(line, '"')
	if start < 0 {
		return "", false
	}
	end := strings.IndexByte(line[start+1:], '"')
	if end < 0 {
		return "", false
	}
	name := strings.TrimSpace(line[start+1 : start+1+end])
	if name == "" {
		return "", false
	}
	return name, true
}

func resolveFFmpegDShowDevice(devices []string, options Options) (string, string, error) {
	configuredNames := []string{options.PreferredInputDevice, options.FallbackInputDevice}
	var failures []string
	for _, configured := range configuredNames {
		configured = strings.TrimSpace(configured)
		if configured == "" {
			continue
		}
		if device, ok := matchFFmpegDShowDevice(devices, configured); ok {
			return device, fmt.Sprintf("configured=%s resolved=%s", displayDeviceName(configured), displayDeviceName(device)), nil
		}
		failures = append(failures, displayDeviceName(configured))
	}
	if len(failures) > 0 {
		return "", "", fmt.Errorf(
			"directshow device not found for configured input(s) %s; available audio devices=%s",
			strings.Join(failures, ", "),
			describeFFmpegDShowDevices(devices),
		)
	}

	ordered := reorderInputDevicesPreferGame(ffmpegDShowDevicesAsInfo(devices))
	if len(ordered) == 0 {
		return "", "", fmt.Errorf("directshow device enumeration returned no audio devices")
	}
	return ordered[0].Name, fmt.Sprintf("configured=%q resolved=%s", "(auto)", displayDeviceName(ordered[0].Name)), nil
}

func matchFFmpegDShowDevice(devices []string, configured string) (string, bool) {
	normalizedConfigured := normalizeFFmpegDeviceName(configured)
	for _, device := range devices {
		if device == configured {
			return device, true
		}
	}
	for _, device := range devices {
		if normalizeFFmpegDeviceName(device) == normalizedConfigured {
			return device, true
		}
	}

	matches := make([]string, 0, 2)
	for _, device := range devices {
		normalizedDevice := normalizeFFmpegDeviceName(device)
		if strings.Contains(normalizedDevice, normalizedConfigured) || strings.Contains(normalizedConfigured, normalizedDevice) {
			matches = append(matches, device)
		}
	}
	if len(matches) == 1 {
		return matches[0], true
	}
	return "", false
}

func describeFFmpegDShowDevices(devices []string) string {
	if len(devices) == 0 {
		return "(none)"
	}
	parts := make([]string, 0, len(devices))
	for _, device := range devices {
		parts = append(parts, displayDeviceName(device))
	}
	return strings.Join(parts, ", ")
}

func ffmpegDShowDevicesAsInfo(devices []string) []DeviceInfo {
	out := make([]DeviceInfo, 0, len(devices))
	for i, device := range devices {
		out = append(out, DeviceInfo{ID: uint32(i), Name: device})
	}
	return out
}

func normalizeFFmpegDeviceName(name string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(name))), " ")
}
