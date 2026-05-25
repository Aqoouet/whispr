# Plan: Radical Voice Capture Simplification

## Context

The whispr audio capture layer has grown into a 3,000-line, 5-backend fallback maze (FFmpeg DirectShow → WASAPI → WinMM → DirectSound). Each backend added more complexity and more things that can silently fail. Recent git history is a graveyard of WASAPI format negotiation hacks, vtable offsets, and endpoint reset hints — none of it working reliably on the target hardware.

FFmpeg DirectShow is the *only* backend that has any chance of being "100% working": it's a mature binary that handles all device and format negotiation internally, has been battle-tested for 20+ years, and the only requirement is that `ffmpeg.exe` is present in the runtime directory. It was just promoted to primary on 2026-05-25. The rest should go.

**Goal:** Delete every backend except FFmpeg DirectShow. Reduce the audio package from ~3,000 lines to ~450 lines. Make failures fast, obvious, and actionable.

---

## What Gets Deleted

| File | Lines | Action |
|------|-------|--------|
| `internal/audio/wasapi_windows.go` | 1,107 | **Delete entirely** |
| `internal/audio/wasapi_windows_test.go` | 206 | **Delete entirely** |

---

## What Gets Rewritten

### `internal/audio/audio_windows.go` (643 → ~120 lines)

Remove all WinMM and DirectSound code: `winmm.dll` / `dsound.dll` lazy DLL loads, all `procWaveIn*` and `procDirectSound*` procs, `waveDevCaps`, `waveHdr`, `dsVtbl`, `copyLockedBytes`, `putU16/32`, `writeWAV`, `mmErr*` family, `dsoundTryOpen/Probe/CreateBuffer/StartCapture/Stop/ReadBuffer/Unlock/ReleaseInterfaces/Start`, `waveInOpenUsing`, `waveInGetNumDevs/DevCaps`, `applyFormat`, `resetState`, `tryOpen`.

New Recorder struct — only what FFmpeg needs:
```go
type Recorder struct {
    options      Options
    ffmpeg       *ffmpegSession
    activeDetail string
    active       bool
}
```

New `Start()`: call `r.startFFmpegDShow()`, set `r.active = true` on success. Fail immediately on error — no retries, no fallbacks.

New `Stop()`: call `r.ffmpegStop()`, set `r.active = false`. FFmpeg already writes the WAV, so no `writeWAV` needed.

New `Close()`: stop if active, cleanup ffmpeg session.

`EnumerateInputDevices()`: replace WinMM enumeration with a thin wrapper around `listFFmpegDShowAudioDevices` using the RuntimeDir from a package-level variable set at init, OR change signature to `EnumerateInputDevices(runtimeDir string)` and update the single caller in `app.go`. (Simplest: rename to `EnumerateFFmpegDevices(runtimeDir string) ([]DeviceInfo, error)` — see app.go changes below.)

### `internal/audio/open_policy.go` (316 → ~160 lines)

Remove: `buildWinMMOpenPlan`, `orderWASAPIEndpoints`, `resolveInputDeviceSelection`, `openAttemptSpec`, `formatCandidate`, `formatCandidates`, `waveMapper`, `waveFormDirect`, `captureBackendWASAPI`, `captureBackendWinMMMapper`, `captureBackendWinMMDevice`, `captureBackendDSound`, `preferredWindowsCaptureBackends`, `inputDeviceRank`, `reorderInputDevicesPreferGame`, `findDeviceByName`, `selectFailureDeviceLabel`.

Keep: `Options`, `DeviceInfo`, `openAttempt`, `openFailure`, `newOpenFailure`, `displayDeviceName`, `deviceIdentityKey`, `normalizeDeviceName`, `DescribeInputDevices`, `DescribeInputSelection`.

Simplify `openFailureRecoveryHint` to only handle FFmpeg failure reasons (it already branches on `captureBackendFFmpegDShow`, just remove the `hasNativeFailure` branch).

### `internal/audio/open_policy_test.go` (170 → ~60 lines)

Remove tests for `buildWinMMOpenPlan`, `orderWASAPIEndpoints`, `resolveInputDeviceSelection`. Keep tests for FFmpeg device resolution and error hint logic.

---

## What Stays Unchanged

- `internal/audio/audio_stub.go` — non-Windows stub, no change
- `internal/audio/ffmpeg_dshow.go` — device parsing & resolution, no change
- `internal/audio/ffmpeg_dshow_windows.go` — subprocess management, no change
- `internal/audio/ffmpeg_dshow_test.go` — all tests remain valid

---

## App Integration Change

**`internal/app/app.go`** (lines 68–79): Remove the `EnumerateInputDevices()` + `EnumerateWASAPIInputDevices()` logging block, or replace both with a single `audio.EnumerateFFmpegDevices(audioOptions.RuntimeDir)` call that logs what FFmpeg sees at startup. This makes the startup log consistent with the actual capture path used.

---

## Verification

```bash
# Cross-compile check (catches build errors without Windows)
GOCACHE=/tmp/whispr-gocache GOOS=windows GOARCH=amd64 go build ./...

# Compile and run tests
GOCACHE=/tmp/whispr-gocache go test ./internal/audio/...
GOCACHE=/tmp/whispr-gocache GOOS=windows GOARCH=amd64 go test -c -o /tmp/audio.test.exe ./internal/audio

# Then follow standard deploy workflow:
# commit → push → pull on stressii-wg → build → deploy to T:\whispr\dictation.exe
# Validate startup log: grep for "startup build=" line, then test a recording
```
