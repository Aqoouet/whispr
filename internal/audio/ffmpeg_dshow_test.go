package audio

import (
	"strings"
	"testing"
)

func TestParseFFmpegDShowAudioDevices(t *testing.T) {
	output := strings.Join([]string{
		`[dshow @ 000001] DirectShow audio devices`,
		`[dshow @ 000001]  "Microphone (Arctis Pro Wireless Chat)" (audio)`,
		`[dshow @ 000001]  "Микрофон (USB гарнитура)" (audio)`,
		`[dshow @ 000001]  "Integrated Camera" (video)`,
		`[dshow @ 000001]  "Microphone (Arctis Pro Wireless Chat)" (audio)`,
	}, "\n")

	got := parseFFmpegDShowAudioDevices(output)
	want := []string{
		"Microphone (Arctis Pro Wireless Chat)",
		"Микрофон (USB гарнитура)",
	}
	if len(got) != len(want) {
		t.Fatalf("devices=%v want=%v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("devices[%d]=%q want %q", i, got[i], want[i])
		}
	}
}

func TestParseFFmpegDShowAudioDevicesNewFormat(t *testing.T) {
	// New ffmpeg format: section headers, no "(audio)" tag per device line.
	output := strings.Join([]string{
		`[dshow @ 000001] DirectShow video devices`,
		`[dshow @ 000001]  "Integrated Camera"`,
		`[dshow @ 000001]     Alternative name "@device_pnp_..."`,
		`[dshow @ 000001] DirectShow audio devices`,
		`[dshow @ 000001]  "Microphone (Arctis Pro Wireless Chat)"`,
		`[dshow @ 000001]     Alternative name "@device_cm_{...}\wave_{...}"`,
		`[dshow @ 000001]  "Микрофон (2- Jabra EVOLVE 20)"`,
		`[dshow @ 000001]     Alternative name "@device_cm_{...}\wave_{...}"`,
		`dummy: Immediate exit requested`,
	}, "\n")

	got := parseFFmpegDShowAudioDevices(output)
	want := []string{
		"Microphone (Arctis Pro Wireless Chat)",
		"Микрофон (2- Jabra EVOLVE 20)",
	}
	if len(got) != len(want) {
		t.Fatalf("devices=%v want=%v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("devices[%d]=%q want %q", i, got[i], want[i])
		}
	}
}

func TestResolveFFmpegDShowDevicePrefersNormalizedExactMatch(t *testing.T) {
	device, detail, err := resolveFFmpegDShowDevice([]string{
		"Microphone (SteelSeries Sonar)",
		"Arctis Pro Wireless Chat",
	}, Options{PreferredInputDevice: "  arctis   pro wireless chat "})
	if err != nil {
		t.Fatal(err)
	}
	if device != "Arctis Pro Wireless Chat" {
		t.Fatalf("device=%q", device)
	}
	if !strings.Contains(detail, `resolved="Arctis Pro Wireless Chat"`) {
		t.Fatalf("detail=%q", detail)
	}
}

func TestResolveFFmpegDShowDeviceAllowsUniqueSubstringMatch(t *testing.T) {
	device, _, err := resolveFFmpegDShowDevice([]string{
		"Microphone Array (Intel)",
		"Microphone (Arctis Pro Wireless Chat)",
	}, Options{PreferredInputDevice: "Arctis Pro Wireless Chat"})
	if err != nil {
		t.Fatal(err)
	}
	if device != "Microphone (Arctis Pro Wireless Chat)" {
		t.Fatalf("device=%q", device)
	}
}

func TestResolveFFmpegDShowDeviceRejectsMissingConfiguredDevice(t *testing.T) {
	_, _, err := resolveFFmpegDShowDevice([]string{"Microphone (USB Audio Device)"}, Options{
		PreferredInputDevice: "Arctis Pro Wireless Chat",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), `directshow device not found`) {
		t.Fatalf("err=%v", err)
	}
}

func TestResolveFFmpegDShowDeviceFallsBackToConfiguredFallback(t *testing.T) {
	device, _, err := resolveFFmpegDShowDevice([]string{
		"Microphone (USB Audio Device)",
		"Microphone (HP USB Media Audio)",
	}, Options{
		PreferredInputDevice: "Missing Mic",
		FallbackInputDevice:  "HP USB Media Audio",
	})
	if err != nil {
		t.Fatal(err)
	}
	if device != "Microphone (HP USB Media Audio)" {
		t.Fatalf("device=%q", device)
	}
}
