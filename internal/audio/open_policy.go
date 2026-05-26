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
