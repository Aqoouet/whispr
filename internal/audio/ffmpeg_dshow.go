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

// dshowAudioDevice holds both the human-readable display name and the name to
// pass to ffmpeg's -i audio= argument. When the display name contains non-ASCII
// characters, FFmpegName is set to the Alternative name (an ASCII GUID path) to
// avoid encoding issues with older ffmpeg builds like the Altair-bundled binary.
type dshowAudioDevice struct {
	Name       string // display name (used in logs)
	FFmpegName string // passed to -i audio=... (alt name when display has non-ASCII)
}

func parseFFmpegDShowAudioDevices(output string) []dshowAudioDevice {
	lines := strings.Split(output, "\n")
	devices := make([]dshowAudioDevice, 0, len(lines))
	seen := make(map[string]bool, len(lines))
	inAudioSection := false
	var pending *dshowAudioDevice

	flush := func() {
		if pending == nil {
			return
		}
		key := normalizeFFmpegDeviceName(pending.Name)
		if !seen[key] {
			devices = append(devices, *pending)
			seen[key] = true
		}
		pending = nil
	}

	for _, line := range lines {
		if strings.Contains(line, "DirectShow audio devices") {
			inAudioSection = true
			continue
		}
		if strings.Contains(line, "DirectShow video devices") {
			flush()
			inAudioSection = false
			continue
		}
		// Old format: "(audio)" tag on device line.
		if strings.Contains(line, "(audio)") {
			flush()
			inAudioSection = false
			if name, ok := parseQuotedDeviceName(line); ok {
				key := normalizeFFmpegDeviceName(name)
				if !seen[key] {
					devices = append(devices, dshowAudioDevice{Name: name, FFmpegName: name})
					seen[key] = true
				}
			}
			continue
		}
		if !inAudioSection {
			continue
		}
		// Alternative name line follows the device it belongs to.
		if strings.Contains(line, "Alternative name") {
			if pending != nil {
				if altName, ok := parseQuotedDeviceName(line); ok && containsNonASCII(pending.Name) {
					pending.FFmpegName = altName
				}
				flush()
			}
			continue
		}
		// New-format device name line inside audio section.
		flush()
		if name, ok := parseQuotedDeviceName(line); ok {
			pending = &dshowAudioDevice{Name: name, FFmpegName: name}
		}
	}
	flush()
	return devices
}

func containsNonASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] > 127 {
			return true
		}
	}
	return false
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

func resolveFFmpegDShowDevice(devices []dshowAudioDevice, options Options) (dshowAudioDevice, string, error) {
	configuredNames := []string{options.PreferredInputDevice, options.FallbackInputDevice}
	var failures []string
	for _, configured := range configuredNames {
		configured = strings.TrimSpace(configured)
		if configured == "" {
			continue
		}
		if dev, ok := matchFFmpegDShowDevice(devices, configured); ok {
			detail := fmt.Sprintf("configured=%s resolved=%s", displayDeviceName(configured), displayDeviceName(dev.Name))
			return dev, detail, nil
		}
		failures = append(failures, displayDeviceName(configured))
	}
	if len(failures) > 0 {
		return dshowAudioDevice{}, "", fmt.Errorf(
			"directshow device not found for configured input(s) %s; available audio devices=%s",
			strings.Join(failures, ", "),
			describeFFmpegDShowDevices(devices),
		)
	}

	ordered := reorderInputDevicesPreferGame(ffmpegDShowDevicesAsInfo(devices))
	if len(ordered) == 0 {
		return dshowAudioDevice{}, "", fmt.Errorf("directshow device enumeration returned no audio devices")
	}
	for _, dev := range devices {
		if dev.Name == ordered[0].Name {
			return dev, fmt.Sprintf("configured=%q resolved=%s", "(auto)", displayDeviceName(dev.Name)), nil
		}
	}
	dev := dshowAudioDevice{Name: ordered[0].Name, FFmpegName: ordered[0].Name}
	return dev, fmt.Sprintf("configured=%q resolved=%s", "(auto)", displayDeviceName(dev.Name)), nil
}

func matchFFmpegDShowDevice(devices []dshowAudioDevice, configured string) (dshowAudioDevice, bool) {
	normalizedConfigured := normalizeFFmpegDeviceName(configured)
	for _, dev := range devices {
		if dev.Name == configured {
			return dev, true
		}
	}
	for _, dev := range devices {
		if normalizeFFmpegDeviceName(dev.Name) == normalizedConfigured {
			return dev, true
		}
	}
	matches := make([]dshowAudioDevice, 0, 2)
	for _, dev := range devices {
		normalizedDevice := normalizeFFmpegDeviceName(dev.Name)
		if strings.Contains(normalizedDevice, normalizedConfigured) || strings.Contains(normalizedConfigured, normalizedDevice) {
			matches = append(matches, dev)
		}
	}
	if len(matches) == 1 {
		return matches[0], true
	}
	return dshowAudioDevice{}, false
}

func describeFFmpegDShowDevices(devices []dshowAudioDevice) string {
	if len(devices) == 0 {
		return "(none)"
	}
	parts := make([]string, 0, len(devices))
	for _, dev := range devices {
		parts = append(parts, displayDeviceName(dev.Name))
	}
	return strings.Join(parts, ", ")
}

func ffmpegDShowDevicesAsInfo(devices []dshowAudioDevice) []DeviceInfo {
	out := make([]DeviceInfo, 0, len(devices))
	for i, dev := range devices {
		out = append(out, DeviceInfo{ID: uint32(i), Name: dev.Name})
	}
	return out
}

func normalizeFFmpegDeviceName(name string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(name))), " ")
}
