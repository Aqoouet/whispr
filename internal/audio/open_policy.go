package audio

import (
	"fmt"
	"strings"
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

type openTarget struct {
	Device    DeviceInfo
	Detail    string
	IsDefault bool
}

func DescribeInputDevices(devices []DeviceInfo) string {
	if len(devices) == 0 {
		return "(none)"
	}
	parts := make([]string, 0, len(devices))
	for _, d := range devices {
		parts = append(parts, fmt.Sprintf("%d:%s", d.ID, d.Name))
	}
	return strings.Join(parts, ", ")
}

func DescribeInputSelection(options Options) string {
	return fmt.Sprintf("preferred_input_device=%q fallback_input_device=%q",
		options.PreferredInputDevice, options.FallbackInputDevice)
}

func normalizeDeviceName(name string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(name))), " ")
}

func resolveInputDevice(devices []DeviceInfo, defaultDevice DeviceInfo, options Options) (DeviceInfo, string, error) {
	if preferred := strings.TrimSpace(options.PreferredInputDevice); preferred != "" {
		if device, ok, mode := findConfiguredDevice(devices, preferred); ok {
			return device, fmt.Sprintf("preferred_input_device=%q match=%s", device.Name, mode), nil
		}
		if fallback := strings.TrimSpace(options.FallbackInputDevice); fallback != "" {
			if device, ok, mode := findConfiguredDevice(devices, fallback); ok {
				return device, fmt.Sprintf("fallback_input_device=%q match=%s", device.Name, mode), nil
			}
		}
		return defaultDevice, "system_default", nil
	}
	if fallback := strings.TrimSpace(options.FallbackInputDevice); fallback != "" {
		if device, ok, mode := findConfiguredDevice(devices, fallback); ok {
			return device, fmt.Sprintf("fallback_input_device=%q match=%s", device.Name, mode), nil
		}
		return defaultDevice, "system_default", nil
	}
	return defaultDevice, "system_default", nil
}

func planOpenTargets(devices []DeviceInfo, defaultDevice DeviceInfo, options Options) ([]openTarget, error) {
	selected, detail, err := resolveInputDevice(devices, defaultDevice, options)
	if err != nil {
		return nil, err
	}
	targets := []openTarget{{
		Device:    selected,
		Detail:    detail,
		IsDefault: selected.EndpointID == defaultDevice.EndpointID,
	}}
	if selected.EndpointID != "" && selected.EndpointID != defaultDevice.EndpointID {
		targets = append(targets, openTarget{
			Device:    defaultDevice,
			Detail:    "system_default_retry_after_backend_failure",
			IsDefault: true,
		})
	}
	return targets, nil
}

func findConfiguredDevice(devices []DeviceInfo, configured string) (DeviceInfo, bool, string) {
	want := normalizeDeviceName(configured)
	if want == "" {
		return DeviceInfo{}, false, ""
	}
	exactMatches := make([]DeviceInfo, 0, 2)
	for _, device := range devices {
		if normalizeDeviceName(device.Name) == want {
			exactMatches = append(exactMatches, device)
		}
	}
	if len(exactMatches) == 1 {
		return exactMatches[0], true, "exact"
	}
	matches := make([]DeviceInfo, 0, 2)
	for _, device := range devices {
		if strings.Contains(normalizeDeviceName(device.Name), want) {
			matches = append(matches, device)
		}
	}
	if len(matches) == 1 {
		return matches[0], true, "substring"
	}
	return DeviceInfo{}, false, ""
}
