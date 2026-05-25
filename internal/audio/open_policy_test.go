package audio

import (
	"strings"
	"testing"
)

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

func TestOpenFailureIncludesFFmpegSpecificRecoveryHint(t *testing.T) {
	err := newOpenFailure("Arctis Pro Wireless Chat", 1, []openAttempt{
		{Backend: captureBackendFFmpegDShow, Detail: "binary", Failure: `bundled ffmpeg.exe missing at "C:\\runtime\\ffmpeg.exe" and ffmpeg.exe not found on PATH`},
	})
	if !strings.Contains(err.Error(), `bundled ffmpeg.exe is missing from runtime\ffmpeg.exe`) {
		t.Fatalf("unexpected error: %v", err)
	}
}
