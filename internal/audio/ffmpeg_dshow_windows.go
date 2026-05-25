//go:build windows

package audio

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

const ffmpegShutdownTimeout = 3 * time.Second

var execCommand = exec.Command

type ffmpegSession struct {
	outputPath string
	stdin      io.WriteCloser
	stderr     *limitedBuffer
	waitCh     chan error
	waitOnce   sync.Once
	waitErr    error
	stopDetail string
	cmd        *exec.Cmd
}

type limitedBuffer struct {
	max  int
	data []byte
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.max <= 0 {
		return len(p), nil
	}
	if len(p) >= b.max {
		b.data = append([]byte(nil), p[len(p)-b.max:]...)
		return len(p), nil
	}
	total := len(b.data) + len(p)
	if total > b.max {
		keep := b.max - len(p)
		if keep < 0 {
			keep = 0
		}
		if keep > len(b.data) {
			keep = len(b.data)
		}
		b.data = append([]byte(nil), b.data[len(b.data)-keep:]...)
	}
	b.data = append(b.data, p...)
	return len(p), nil
}

func (b *limitedBuffer) String() string {
	return strings.TrimSpace(string(bytes.TrimSpace(b.data)))
}

func (r *Recorder) startFFmpegDShow() ([]openAttempt, error) {
	ffmpegPath, source, err := findFFmpegExecutable(r.options.FFmpegPath, r.options.RuntimeDir)
	if err != nil {
		return []openAttempt{{
			Backend: captureBackendFFmpegDShow,
			Detail:  "binary",
			Failure: err.Error(),
		}}, err
	}

	devices, enumErr := listFFmpegDShowAudioDevices(ffmpegPath)
	if enumErr != nil {
		attempt := openAttempt{
			Backend: captureBackendFFmpegDShow,
			Detail:  fmt.Sprintf("device-list path=%s", displayDeviceName(ffmpegPath)),
			Failure: fmt.Sprintf("directshow device enumeration failed: %v", enumErr),
		}
		return []openAttempt{attempt}, enumErr
	}

	deviceName, resolutionDetail, err := resolveFFmpegDShowDevice(devices, r.options)
	if err != nil {
		attempt := openAttempt{
			Backend: captureBackendFFmpegDShow,
			Detail:  "device-resolution",
			Failure: err.Error(),
		}
		return []openAttempt{attempt}, err
	}

	outputPath := filepath.Join(os.TempDir(), fmt.Sprintf("corpdictation-%d.wav", time.Now().UnixNano()))
	args := []string{
		"-hide_banner",
		"-y",
		"-f", "dshow",
		"-i", "audio=" + deviceName,
		"-ar", fmt.Sprintf("%d", ffmpegCaptureSampleRate),
		"-ac", fmt.Sprintf("%d", ffmpegCaptureChannels),
		"-acodec", "pcm_s16le",
		outputPath,
	}
	command := formatFFmpegCommand(ffmpegPath, args)
	cmd := execCommand(ffmpegPath, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	stderr := &limitedBuffer{max: 8192}
	cmd.Stderr = stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		attempt := openAttempt{
			Backend: captureBackendFFmpegDShow,
			Detail:  fmt.Sprintf("%s cmd=%s", resolutionDetail, command),
			Failure: fmt.Sprintf("directshow launch failed: stdin pipe: %v", err),
		}
		return []openAttempt{attempt}, err
	}
	if err := cmd.Start(); err != nil {
		attempt := openAttempt{
			Backend: captureBackendFFmpegDShow,
			Detail:  fmt.Sprintf("%s cmd=%s", resolutionDetail, command),
			Failure: fmt.Sprintf("directshow launch failed: %v; stderr=%s", err, summarizeFFmpegStderr(stderr.String())),
		}
		return []openAttempt{attempt}, err
	}

	session := &ffmpegSession{
		outputPath: outputPath,
		stdin:      stdin,
		stderr:     stderr,
		waitCh:     make(chan error, 1),
		stopDetail: fmt.Sprintf("%s source=%s cmd=%s", resolutionDetail, source, command),
		cmd:        cmd,
	}
	go func() {
		session.waitCh <- cmd.Wait()
	}()

	r.ffmpeg = session
	r.mode = recorderModeFFmpegDShow
	r.activeDetail = fmt.Sprintf("backend=%s detail=%s", captureBackendFFmpegDShow, session.stopDetail)
	r.format = waveFormatEx{
		FormatTag:      waveFormatPCM,
		Channels:       ffmpegCaptureChannels,
		SamplesPerSec:  ffmpegCaptureSampleRate,
		BitsPerSample:  ffmpegCaptureBits,
		BlockAlign:     ffmpegCaptureChannels * (ffmpegCaptureBits / 8),
		AvgBytesPerSec: ffmpegCaptureSampleRate * uint32(ffmpegCaptureChannels) * uint32(ffmpegCaptureBits/8),
	}
	return nil, nil
}

func (r *Recorder) ffmpegStop() (string, error) {
	session := r.ffmpeg
	if session == nil || session.cmd == nil {
		return "", fmt.Errorf("no ffmpeg-dshow capture session")
	}

	select {
	case err := <-session.waitCh:
		session.waitErr = err
		cleanupFFmpegSession(session)
		r.ffmpeg = nil
		return "", fmt.Errorf("directshow capture exited early: %s; stderr=%s", waitErrString(err), summarizeFFmpegStderr(session.stderr.String()))
	default:
	}

	if session.stdin != nil {
		_, _ = io.WriteString(session.stdin, "q\n")
		_ = session.stdin.Close()
		session.stdin = nil
	}

	waitErr := waitForFFmpegExit(session)
	if waitErr != nil {
		cleanupFFmpegSession(session)
		r.ffmpeg = nil
		return "", fmt.Errorf("directshow capture failed: %s; stderr=%s", waitErrString(waitErr), summarizeFFmpegStderr(session.stderr.String()))
	}
	if err := validateRecordedWAV(session.outputPath); err != nil {
		cleanupFFmpegSession(session)
		r.ffmpeg = nil
		return "", fmt.Errorf("directshow capture failed: invalid wav %s: %w", displayDeviceName(session.outputPath), err)
	}
	path := session.outputPath
	r.ffmpeg = nil
	return path, nil
}

func waitForFFmpegExit(session *ffmpegSession) error {
	session.waitOnce.Do(func() {
		select {
		case session.waitErr = <-session.waitCh:
		case <-time.After(ffmpegShutdownTimeout):
			_ = session.cmd.Process.Kill()
			session.waitErr = <-session.waitCh
		}
	})
	return session.waitErr
}

func cleanupFFmpegSession(session *ffmpegSession) {
	if session == nil {
		return
	}
	if session.stdin != nil {
		_ = session.stdin.Close()
	}
	if session.outputPath != "" {
		_ = os.Remove(session.outputPath)
	}
}

func findFFmpegExecutable(explicitPath, runtimeDir string) (string, string, error) {
	// 1. Explicit config override — highest priority, policy-safe path chosen by user.
	if explicitPath != "" {
		if info, err := os.Stat(explicitPath); err == nil && !info.IsDir() {
			return explicitPath, "config", nil
		}
	}

	// 2. Well-known Program Files installations — policy-safe, checked before AppData bundled.
	for _, candidate := range wellKnownFFmpegCandidates() {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, "well-known", nil
		}
	}

	// 3. Bundled runtime\ffmpeg.exe — may be blocked by group policy on some machines.
	if runtimeDir != "" {
		bundled := filepath.Join(runtimeDir, "ffmpeg.exe")
		if info, err := os.Stat(bundled); err == nil && !info.IsDir() {
			return bundled, "bundled", nil
		}
	}

	// 4. PATH.
	if path, err := exec.LookPath("ffmpeg.exe"); err == nil {
		return path, "PATH", nil
	}
	if path, err := exec.LookPath("ffmpeg"); err == nil {
		return path, "PATH", nil
	}

	return "", "", fmt.Errorf("ffmpeg.exe not found: checked config path %q, Program Files candidates, bundled %s, and PATH",
		explicitPath, filepath.Join(runtimeDir, "ffmpeg.exe"))
}

// wellKnownFFmpegCandidates returns ffmpeg.exe paths from Program Files locations
// that are not subject to AppData execution policy restrictions.
func wellKnownFFmpegCandidates() []string {
	var candidates []string
	for _, envVar := range []string{"PROGRAMFILES", "PROGRAMW6432", "PROGRAMFILES(X86)"} {
		dir := os.Getenv(envVar)
		if dir == "" {
			continue
		}
		// Standard ffmpeg install layout.
		candidates = append(candidates, filepath.Join(dir, "ffmpeg", "bin", "ffmpeg.exe"))
		// Altair HyperWorks bundles ffmpeg; glob across version directories.
		matches, _ := filepath.Glob(filepath.Join(dir, "Altair", "*", "hwdesktop", "hw", "bin", "win64", "ffmpeg.exe"))
		candidates = append(candidates, matches...)
	}
	return candidates
}

func listFFmpegDShowAudioDevices(ffmpegPath string) ([]string, error) {
	cmd := execCommand(ffmpegPath, "-hide_banner", "-list_devices", "true", "-f", "dshow", "-i", "dummy")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	stderr := &limitedBuffer{max: 8192}
	cmd.Stdout = io.Discard
	cmd.Stderr = stderr
	err := cmd.Run()
	devices := parseFFmpegDShowAudioDevices(stderr.String())
	if len(devices) > 0 {
		return devices, nil
	}
	if err == nil {
		return nil, fmt.Errorf("no audio devices found in ffmpeg DirectShow list")
	}
	return nil, fmt.Errorf("%w; stderr=%s", err, summarizeFFmpegStderr(stderr.String()))
}

func validateRecordedWAV(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(data) <= 44 {
		return fmt.Errorf("wav file too small (%d bytes)", len(data))
	}
	if string(data[:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return errors.New("missing RIFF/WAVE header")
	}
	return nil
}

func formatFFmpegCommand(path string, args []string) string {
	quoted := make([]string, 0, len(args)+1)
	quoted = append(quoted, displayDeviceName(path))
	for _, arg := range args {
		quoted = append(quoted, displayDeviceName(arg))
	}
	return strings.Join(quoted, " ")
}

func summarizeFFmpegStderr(stderr string) string {
	if stderr == "" {
		return "(empty)"
	}
	lines := strings.Split(stderr, "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			filtered = append(filtered, line)
		}
	}
	if len(filtered) == 0 {
		return "(empty)"
	}
	if len(filtered) > 4 {
		filtered = filtered[len(filtered)-4:]
	}
	return strings.Join(filtered, " | ")
}

func waitErrString(err error) string {
	if err == nil {
		return "exit=0"
	}
	return err.Error()
}
