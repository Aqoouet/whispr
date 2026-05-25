package audio

import (
	"strings"
	"testing"
)

func TestResolveInputDeviceSelectionPrefersConfiguredDevices(t *testing.T) {
	devices := []DeviceInfo{
		{ID: 0, Name: "Jabra EVOLVE 20"},
		{ID: 1, Name: "Arctis Pro Wireless Chat"},
		{ID: 2, Name: "HP USB Media Audio"},
	}

	selection, err := resolveInputDeviceSelection(devices, Options{
		PreferredInputDevice: "Arctis Pro Wireless Chat",
		FallbackInputDevice:  "HP USB Media Audio",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(selection.targets) != 2 {
		t.Fatalf("targets = %d, want 2", len(selection.targets))
	}
	if selection.targets[0].Name != "Arctis Pro Wireless Chat" {
		t.Fatalf("first target = %q", selection.targets[0].Name)
	}
	if selection.targets[1].Name != "HP USB Media Audio" {
		t.Fatalf("second target = %q", selection.targets[1].Name)
	}
	if len(selection.remaining) != 1 || selection.remaining[0].Name != "Jabra EVOLVE 20" {
		t.Fatalf("remaining = %#v", selection.remaining)
	}
}

func TestResolveInputDeviceSelectionRejectsUnknownConfiguredDevice(t *testing.T) {
	_, err := resolveInputDeviceSelection([]DeviceInfo{{ID: 1, Name: "Arctis Pro Wireless Chat"}}, Options{
		PreferredInputDevice: "Missing Mic",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), `configured input device "Missing Mic" not found`) {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), `available devices=1:"Arctis Pro Wireless Chat"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReorderInputDevicesPreferGame(t *testing.T) {
	devices := []DeviceInfo{
		{ID: 1, Name: "Arctis Pro Wireless Chat"},
		{ID: 2, Name: "HP USB Media Audio"},
		{ID: 3, Name: "Arctis Pro Wireless Game"},
	}
	got := reorderInputDevicesPreferGame(devices)
	want := []string{
		"Arctis Pro Wireless Game",
		"HP USB Media Audio",
		"Arctis Pro Wireless Chat",
	}
	for i, name := range want {
		if got[i].Name != name {
			t.Fatalf("device[%d] = %q, want %q", i, got[i].Name, name)
		}
	}
}

func TestOrderWASAPIEndpointsPrefersGameOverChatDefault(t *testing.T) {
	endpoints := []DeviceInfo{
		{ID: 0, Name: "Arctis Pro Wireless Chat", EndpointID: "chat-endpoint"},
		{ID: 1, Name: "Arctis Pro Wireless Game", EndpointID: "game-endpoint"},
	}
	selection, err := resolveInputDeviceSelection(endpoints, Options{})
	if err != nil {
		t.Fatal(err)
	}
	ordered := orderWASAPIEndpoints(selection)
	if len(ordered) != 2 {
		t.Fatalf("ordered = %d devices, want 2", len(ordered))
	}
	if ordered[0].Name != "Arctis Pro Wireless Game" {
		t.Fatalf("first endpoint = %q, want Game", ordered[0].Name)
	}
	if ordered[1].Name != "Arctis Pro Wireless Chat" {
		t.Fatalf("second endpoint = %q, want Chat", ordered[1].Name)
	}
}

func TestBuildWinMMOpenPlanOrder(t *testing.T) {
	selection := deviceSelection{
		targets:   []DeviceInfo{{ID: 2, Name: "Arctis Pro Wireless Chat"}},
		remaining: []DeviceInfo{{ID: 0, Name: "HP USB Media Audio"}},
	}
	plan := buildWinMMOpenPlan(selection, []formatCandidate{{samplesPerSec: 16000, channels: 1, bits: 16}})
	if len(plan) != 6 {
		t.Fatalf("expected 6 attempts, got %d", len(plan))
	}
	if plan[0].DeviceID != 2 || plan[0].Backend != captureBackendWinMMDevice {
		t.Fatalf("plan[0] = %#v", plan[0])
	}
	if plan[2].Backend != captureBackendWinMMMapper {
		t.Fatalf("plan[2] = %#v", plan[2])
	}
	if plan[4].DeviceID != 0 || plan[4].Backend != captureBackendWinMMDevice {
		t.Fatalf("plan[4] = %#v", plan[4])
	}
}

func TestOpenFailureIncludesRecoveryHint(t *testing.T) {
	err := newOpenFailure("Jabra EVOLVE 20", 1, []openAttempt{
		{Backend: captureBackendWinMMMapper, Detail: "16000Hz/1ch/16bit mapper", Failure: "INVALPARAM", EndpointReset: true},
		{Backend: captureBackendDSound, Detail: "16000Hz/1ch/16bit", Failure: "CreateCaptureBuffer: 0x80070057"},
	})

	got := err.Error()
	wantParts := []string{
		`device="Jabra EVOLVE 20"`,
		`devices=1`,
		`winmm-mapper[16000Hz/1ch/16bit mapper]=INVALPARAM`,
		`dsound[16000Hz/1ch/16bit]=CreateCaptureBuffer: 0x80070057`,
		`unplug/replug the USB headset`,
	}
	for _, part := range wantParts {
		if !strings.Contains(got, part) {
			t.Fatalf("error %q missing %q", got, part)
		}
	}
}
