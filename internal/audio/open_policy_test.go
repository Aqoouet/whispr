package audio

import "testing"

func TestResolveInputDevicePrefersExactConfiguredMatch(t *testing.T) {
	devices := []DeviceInfo{
		{ID: 0, Name: "Microphone Array"},
		{ID: 1, Name: "Arctis Pro Wireless Chat"},
	}

	device, detail, err := resolveInputDevice(devices, devices[0], Options{
		PreferredInputDevice: "  arctis   pro wireless chat ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if device.Name != "Arctis Pro Wireless Chat" {
		t.Fatalf("device=%q", device.Name)
	}
	if detail != `preferred_input_device="Arctis Pro Wireless Chat" match=exact` {
		t.Fatalf("detail=%q", detail)
	}
}

func TestResolveInputDeviceAllowsUniqueSubstringMatch(t *testing.T) {
	devices := []DeviceInfo{
		{ID: 0, Name: "Microphone Array (Intel)"},
		{ID: 1, Name: "Microphone (Arctis Pro Wireless Chat)"},
	}

	device, _, err := resolveInputDevice(devices, devices[0], Options{
		PreferredInputDevice: "Arctis Pro Wireless Chat",
	})
	if err != nil {
		t.Fatal(err)
	}
	if device.Name != "Microphone (Arctis Pro Wireless Chat)" {
		t.Fatalf("device=%q", device.Name)
	}
}

func TestResolveInputDeviceFallsBackWhenPreferredMissing(t *testing.T) {
	devices := []DeviceInfo{
		{ID: 0, Name: "Microphone (USB Audio Device)"},
		{ID: 1, Name: "Microphone (HP USB Media Audio)"},
	}

	device, detail, err := resolveInputDevice(devices, devices[0], Options{
		PreferredInputDevice: "Missing Mic",
		FallbackInputDevice:  "HP USB Media Audio",
	})
	if err != nil {
		t.Fatal(err)
	}
	if device.Name != "Microphone (HP USB Media Audio)" {
		t.Fatalf("device=%q", device.Name)
	}
	if detail != `fallback_input_device="Microphone (HP USB Media Audio)" match=substring` {
		t.Fatalf("detail=%q", detail)
	}
}

func TestResolveInputDeviceFallsBackToDefaultWhenPreferredMissing(t *testing.T) {
	devices := []DeviceInfo{{ID: 0, Name: "Microphone (USB Audio Device)"}}

	device, detail, err := resolveInputDevice(devices, devices[0], Options{
		PreferredInputDevice: "Arctis Pro Wireless Chat",
	})
	if err != nil {
		t.Fatal(err)
	}
	if device != devices[0] {
		t.Fatalf("device=%+v", device)
	}
	if detail != "system_default" {
		t.Fatalf("detail=%q", detail)
	}
}

func TestResolveInputDeviceUsesDefaultWhenConfigEmpty(t *testing.T) {
	defaultDevice := DeviceInfo{ID: 9, Name: "Default Microphone", EndpointID: "default"}

	device, detail, err := resolveInputDevice([]DeviceInfo{{ID: 0, Name: "Other"}}, defaultDevice, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if device != defaultDevice {
		t.Fatalf("device=%+v", device)
	}
	if detail != "system_default" {
		t.Fatalf("detail=%q", detail)
	}
}

func TestResolveInputDeviceFallsBackToDefaultWhenExactMatchIsAmbiguous(t *testing.T) {
	devices := []DeviceInfo{
		{ID: 0, Name: "Microphone (USB Audio Device)"},
		{ID: 1, Name: "Microphone (USB Audio Device)"},
	}

	device, detail, err := resolveInputDevice(devices, devices[0], Options{
		PreferredInputDevice: "Microphone (USB Audio Device)",
	})
	if err != nil {
		t.Fatal(err)
	}
	if device != devices[0] {
		t.Fatalf("device=%+v", device)
	}
	if detail != "system_default" {
		t.Fatalf("detail=%q", detail)
	}
}

func TestPlanOpenTargetsRetriesSystemDefaultAfterConfiguredDevice(t *testing.T) {
	devices := []DeviceInfo{
		{ID: 0, Name: "Default Microphone", EndpointID: "default"},
		{ID: 1, Name: "Arctis Pro Wireless Chat", EndpointID: "preferred"},
	}

	targets, err := planOpenTargets(devices, devices[0], Options{
		PreferredInputDevice: "Arctis Pro Wireless Chat",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 {
		t.Fatalf("targets=%d", len(targets))
	}
	if targets[0].Device.EndpointID != "preferred" || targets[0].IsDefault {
		t.Fatalf("primary=%+v", targets[0])
	}
	if targets[1].Device.EndpointID != "default" || !targets[1].IsDefault {
		t.Fatalf("fallback=%+v", targets[1])
	}
	if targets[1].Detail != "system_default_retry_after_backend_failure" {
		t.Fatalf("detail=%q", targets[1].Detail)
	}
}

func TestPlanOpenTargetsDoesNotDuplicateDefaultSelection(t *testing.T) {
	defaultDevice := DeviceInfo{ID: 0, Name: "Default Microphone", EndpointID: "default"}

	targets, err := planOpenTargets([]DeviceInfo{defaultDevice}, defaultDevice, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 {
		t.Fatalf("targets=%d", len(targets))
	}
	if !targets[0].IsDefault {
		t.Fatalf("target=%+v", targets[0])
	}
}
