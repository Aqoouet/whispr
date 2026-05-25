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
		if got[i].Name != want[i] {
			t.Fatalf("devices[%d].Name=%q want %q", i, got[i].Name, want[i])
		}
		if got[i].FFmpegName != want[i] {
			t.Fatalf("devices[%d].FFmpegName=%q want %q (old format should equal Name)", i, got[i].FFmpegName, want[i])
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
		if got[i].Name != want[i] {
			t.Fatalf("devices[%d].Name=%q want %q", i, got[i].Name, want[i])
		}
	}
}

func TestParseFFmpegDShowAudioDevicesUsesAltNameForNonASCII(t *testing.T) {
	altName := `@device_cm_{33D9A762-90C8-11D0-BD43-00A0C911CE86}\wave_{12345678-1234-1234-1234-123456789abc}`
	output := strings.Join([]string{
		`[dshow @ 000001] DirectShow audio devices`,
		`[dshow @ 000001]  "ASCII Device"`,
		`[dshow @ 000001]     Alternative name "@device_cm_{...}\wave_{ascii}"`,
		`[dshow @ 000001]  "Микрофон (2- Jabra EVOLVE 20)"`,
		`[dshow @ 000001]     Alternative name "` + altName + `"`,
	}, "\n")

	got := parseFFmpegDShowAudioDevices(output)
	if len(got) != 2 {
		t.Fatalf("want 2 devices, got %d: %v", len(got), got)
	}
	if got[0].FFmpegName != got[0].Name {
		t.Fatalf("ASCII device: FFmpegName=%q want Name=%q", got[0].FFmpegName, got[0].Name)
	}
	if got[1].Name != "Микрофон (2- Jabra EVOLVE 20)" {
		t.Fatalf("Name=%q", got[1].Name)
	}
	if got[1].FFmpegName != altName {
		t.Fatalf("non-ASCII device: FFmpegName=%q want %q", got[1].FFmpegName, altName)
	}
}

func TestResolveFFmpegDShowDevicePrefersNormalizedExactMatch(t *testing.T) {
	devices := []dshowAudioDevice{
		{Name: "Microphone (SteelSeries Sonar)", FFmpegName: "Microphone (SteelSeries Sonar)"},
		{Name: "Arctis Pro Wireless Chat", FFmpegName: "Arctis Pro Wireless Chat"},
	}
	dev, detail, err := resolveFFmpegDShowDevice(devices, Options{PreferredInputDevice: "  arctis   pro wireless chat "})
	if err != nil {
		t.Fatal(err)
	}
	if dev.Name != "Arctis Pro Wireless Chat" {
		t.Fatalf("device=%q", dev.Name)
	}
	if !strings.Contains(detail, `resolved="Arctis Pro Wireless Chat"`) {
		t.Fatalf("detail=%q", detail)
	}
}

func TestResolveFFmpegDShowDeviceAllowsUniqueSubstringMatch(t *testing.T) {
	devices := []dshowAudioDevice{
		{Name: "Microphone Array (Intel)", FFmpegName: "Microphone Array (Intel)"},
		{Name: "Microphone (Arctis Pro Wireless Chat)", FFmpegName: "Microphone (Arctis Pro Wireless Chat)"},
	}
	dev, _, err := resolveFFmpegDShowDevice(devices, Options{PreferredInputDevice: "Arctis Pro Wireless Chat"})
	if err != nil {
		t.Fatal(err)
	}
	if dev.Name != "Microphone (Arctis Pro Wireless Chat)" {
		t.Fatalf("device=%q", dev.Name)
	}
}

func TestResolveFFmpegDShowDeviceRejectsMissingConfiguredDevice(t *testing.T) {
	devices := []dshowAudioDevice{
		{Name: "Microphone (USB Audio Device)", FFmpegName: "Microphone (USB Audio Device)"},
	}
	_, _, err := resolveFFmpegDShowDevice(devices, Options{
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
	devices := []dshowAudioDevice{
		{Name: "Microphone (USB Audio Device)", FFmpegName: "Microphone (USB Audio Device)"},
		{Name: "Microphone (HP USB Media Audio)", FFmpegName: "Microphone (HP USB Media Audio)"},
	}
	dev, _, err := resolveFFmpegDShowDevice(devices, Options{
		PreferredInputDevice: "Missing Mic",
		FallbackInputDevice:  "HP USB Media Audio",
	})
	if err != nil {
		t.Fatal(err)
	}
	if dev.Name != "Microphone (HP USB Media Audio)" {
		t.Fatalf("device=%q", dev.Name)
	}
}
