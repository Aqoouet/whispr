//go:build windows

package audio

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const (
	waveFormatPCM = 1

	mmNoError       = 0
	mmBadDeviceID   = 2
	mmNotEnabled    = 3
	mmAllocated     = 4
	mmInvalidHandle = 5
	mmNoDriver      = 6
	mmNoMem         = 7
	mmNotSupported  = 8
	mmBadErrNum     = 9
	mmInvalParam    = 11
	mmInvalFlag     = 12
	mmBadFormat     = 32
	mmStillPlaying  = 33
	mmUnprepared    = 34

	dsMethodRelease            = 2
	dsCaptureCreateBuffer      = 3
	dsBufferGetCurrentPosition = 4
	dsBufferLockMethod         = 8
	dsBufferStartMethod        = 9
	dsBufferStopMethod         = 10
	dsBufferUnlockMethod       = 11

	recorderModeFFmpegDShow = "ffmpeg-dshow"
	recorderModeWASAPI      = "wasapi"
	recorderModeWinMM       = "winmm"
	recorderModeDSound      = "dsound"
)

type waveDevCaps struct {
	Mid            uint16
	Pid            uint16
	DriverVersion  uint32
	ProductName    [32]uint16
	Formats        uint32
	Channels       uint16
	Reserved       uint16
	SupportFormats [2]uint16
}

type waveFormatEx struct {
	FormatTag      uint16
	Channels       uint16
	SamplesPerSec  uint32
	AvgBytesPerSec uint32
	BlockAlign     uint16
	BitsPerSample  uint16
	Size           uint16
}

type waveHdr struct {
	Data          *byte
	BufferLength  uint32
	BytesRecorded uint32
	User          uintptr
	Flags         uint32
	Loops         uint32
	Next          uintptr
	Reserved      uintptr
}

var (
	winmm                = syscall.NewLazyDLL("winmm.dll")
	procWaveInGetNumDevs = winmm.NewProc("waveInGetNumDevs")
	procWaveInGetDevCaps = winmm.NewProc("waveInGetDevCapsW")
	procWaveInOpen       = winmm.NewProc("waveInOpen")
	procWaveInPrepare    = winmm.NewProc("waveInPrepareHeader")
	procWaveInUnprepare  = winmm.NewProc("waveInUnprepareHeader")
	procWaveInAddBuffer  = winmm.NewProc("waveInAddBuffer")
	procWaveInStart      = winmm.NewProc("waveInStart")
	procWaveInStop       = winmm.NewProc("waveInStop")
	procWaveInReset      = winmm.NewProc("waveInReset")
	procWaveInClose      = winmm.NewProc("waveInClose")
)

var (
	dsound                        = syscall.NewLazyDLL("dsound.dll")
	procDirectSoundCaptureCreate8 = dsound.NewProc("DirectSoundCaptureCreate8")
)

type Recorder struct {
	options Options
	mode    string
	handle  uintptr
	pDSC    uintptr
	pDSCB   uintptr
	dscBuf  struct {
		bytes  uint32
		offset uint32
	}
	format       waveFormatEx
	buffer       []byte
	header       waveHdr
	wasapiStopCh chan struct{}
	wasapiDoneCh chan wasapiCaptureResult
	wasapiDevice string
	activeDetail string
	ffmpeg       *ffmpegSession
	active       bool
}

func NewRecorder(options Options) (*Recorder, error) {
	return &Recorder{options: options}, nil
}

func EnumerateInputDevices() ([]DeviceInfo, error) {
	devs := waveInGetNumDevs()
	devices := make([]DeviceInfo, 0, devs)
	for id := uint32(0); id < devs; id++ {
		devices = append(devices, DeviceInfo{ID: id, Name: waveInDevCaps(id)})
	}
	return devices, nil
}

func (r *Recorder) Start() error {
	if r.active {
		return fmt.Errorf("recording already active")
	}

	r.resetState()
	r.buffer = make([]byte, 32000*60)
	r.header = waveHdr{Data: &r.buffer[0], BufferLength: uint32(len(r.buffer))}

	if err := r.tryOpen(0, nil); err != nil {
		return err
	}
	if r.mode == recorderModeFFmpegDShow || r.mode == recorderModeWASAPI || r.mode == recorderModeDSound {
		r.active = true
		return nil
	}

	r1, _, _ := procWaveInPrepare.Call(r.handle, uintptr(unsafe.Pointer(&r.header)), unsafe.Sizeof(r.header))
	if err := mmErr(r1); err != nil {
		return fmt.Errorf("waveInPrepareHeader: %w", err)
	}
	r1, _, _ = procWaveInAddBuffer.Call(r.handle, uintptr(unsafe.Pointer(&r.header)), unsafe.Sizeof(r.header))
	if err := mmErr(r1); err != nil {
		return fmt.Errorf("waveInAddBuffer: %w", err)
	}
	r1, _, _ = procWaveInStart.Call(r.handle)
	if err := mmErr(r1); err != nil {
		return fmt.Errorf("waveInStart: %w", err)
	}
	r.active = true
	return nil
}

func (r *Recorder) Stop() (string, error) {
	if !r.active {
		return "", fmt.Errorf("recording is not active")
	}

	var recorded int
	var stopErr error

	switch r.mode {
	case recorderModeFFmpegDShow:
		path, err := r.ffmpegStop()
		r.active = false
		if err != nil {
			return "", fmt.Errorf("ffmpeg-dshow stop: %w", err)
		}
		return path, nil
	case recorderModeWASAPI:
		recorded, stopErr = r.wasapiStop()
		if stopErr != nil {
			return "", fmt.Errorf("wasapi stop: %w", stopErr)
		}
	case recorderModeDSound:
		recorded, stopErr = r.dsoundStop()
		if stopErr != nil {
			return "", fmt.Errorf("dsound stop: %w", stopErr)
		}
	default:
		r1, _, _ := procWaveInStop.Call(r.handle)
		_ = mmErr(r1)
		time.Sleep(150 * time.Millisecond)
		r1, _, _ = procWaveInReset.Call(r.handle)
		_ = mmErr(r1)
		r1, _, _ = procWaveInUnprepare.Call(r.handle, uintptr(unsafe.Pointer(&r.header)), unsafe.Sizeof(r.header))
		_ = mmErr(r1)
		r1, _, _ = procWaveInClose.Call(r.handle)
		_ = mmErr(r1)
		recorded = int(r.header.BytesRecorded)
	}

	r.active = false
	if recorded <= 0 {
		return "", fmt.Errorf("no audio recorded")
	}

	path := filepath.Join(os.TempDir(), fmt.Sprintf("corpdictation-%d.wav", time.Now().UnixNano()))
	if err := writeWAV(path, r.buffer[:recorded], r.format); err != nil {
		return "", err
	}
	return path, nil
}

func (r *Recorder) wasapiStop() (int, error) {
	if r.wasapiStopCh == nil || r.wasapiDoneCh == nil {
		return 0, fmt.Errorf("no capture session")
	}
	close(r.wasapiStopCh)
	result := <-r.wasapiDoneCh
	r.wasapiStopCh = nil
	r.wasapiDoneCh = nil
	if result.err != nil {
		return 0, result.err
	}
	r.buffer = append(r.buffer[:0], result.data...)
	return len(r.buffer), nil
}

func (r *Recorder) dsoundStart() error {
	if err := r.dsoundCreateBuffer(); err != nil {
		r.dsoundReleaseInterfaces()
		return fmt.Errorf("dsound create buffer: %w", err)
	}
	if err := r.dsoundStartCapture(); err != nil {
		r.dsoundReleaseInterfaces()
		return fmt.Errorf("dsound start: %w", err)
	}
	return nil
}

func (r *Recorder) Close() error {
	if r.active {
		_, _ = r.Stop()
	}
	if r.ffmpeg != nil {
		cleanupFFmpegSession(r.ffmpeg)
		r.ffmpeg = nil
	}
	r.dsoundReleaseInterfaces()
	return nil
}

func EnsureWAVPath(path string) (string, error) {
	return path, nil
}

func (r *Recorder) ActiveBackendDescription() string {
	if strings.TrimSpace(r.activeDetail) == "" {
		return "backend=(unknown)"
	}
	return r.activeDetail
}

func writeWAV(path string, pcm []byte, format waveFormatEx) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	dataLen := uint32(len(pcm))
	if _, err := f.Write([]byte("RIFF")); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint32(36)+dataLen); err != nil {
		return err
	}
	if _, err := f.Write([]byte("WAVEfmt ")); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint32(16)); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, format); err != nil {
		return err
	}
	if _, err := f.Write([]byte("data")); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, dataLen); err != nil {
		return err
	}
	_, err = f.Write(pcm)
	return err
}

func mmErr(code uintptr) error {
	if code == 0 {
		return nil
	}
	return fmt.Errorf("mmsys %d (%s)", code, mmErrString(code))
}

func mmErrString(code uintptr) string {
	switch code {
	case mmBadDeviceID:
		return "BADDEVICEID"
	case mmNotEnabled:
		return "NOTENABLED"
	case mmAllocated:
		return "ALLOCATED"
	case mmInvalidHandle:
		return "INVALIDHANDLE"
	case mmNoDriver:
		return "NODRIVER"
	case mmNoMem:
		return "NOMEM"
	case mmNotSupported:
		return "NOTSUPPORTED"
	case mmBadErrNum:
		return "BADERRNUM"
	case mmInvalParam:
		return "INVALPARAM"
	case mmInvalFlag:
		return "INVALFLAG"
	case mmBadFormat:
		return "BADFORMAT"
	case mmStillPlaying:
		return "STILLPLAYING"
	case mmUnprepared:
		return "UNPREPARED"
	default:
		return "unknown"
	}
}

func mmCodeSuggestsEndpointReset(code uintptr) bool {
	return code == mmInvalParam
}

func (r *Recorder) tryOpen(attempt int, prior []openAttempt) error {
	attempts := append([]openAttempt(nil), prior...)

	if ffmpegAttempts, err := r.startFFmpegDShow(); err == nil {
		return nil
	} else {
		attempts = append(attempts, ffmpegAttempts...)
		if r.ffmpeg != nil {
			cleanupFFmpegSession(r.ffmpeg)
			r.ffmpeg = nil
		}
	}

	if wasapiAttempts, err := r.startWASAPI(); err == nil {
		return nil
	} else {
		attempts = append(attempts, wasapiAttempts...)
		var openErr *openFailure
		if err != nil && attempt == 2 {
			if _, ok := err.(*openFailure); ok {
				return err
			}
		}
		_ = openErr
	}

	devices, err := EnumerateInputDevices()
	if err != nil {
		return fmt.Errorf("audio capture init failed: enumerate input devices: %w", err)
	}
	if len(devices) == 0 {
		if len(attempts) > 0 {
			return newOpenFailure("(default mapper)", 0, attempts)
		}
		return fmt.Errorf("audio capture init failed: no capture devices found")
	}

	selection, err := resolveInputDeviceSelection(devices, r.options)
	if err != nil {
		if len(attempts) > 0 {
			attempts = append(attempts, openAttempt{Backend: captureBackendWinMMMapper, Detail: "selection", Failure: err.Error()})
			return newOpenFailure(selectFailureDeviceLabel(selection, devices), uint32(len(devices)), attempts)
		}
		return err
	}

	for _, spec := range buildWinMMOpenPlan(selection, formatCandidates) {
		r.applyFormat(spec.Format)
		code, _ := r.waveInOpenUsing(spec.DeviceID, spec.Flags)
		if code == mmNoError {
			r.mode = recorderModeWinMM
			r.activeDetail = fmt.Sprintf("backend=%s detail=%s", spec.Backend, spec.Detail)
			return nil
		}
		attempts = append(attempts, openAttempt{Backend: spec.Backend, Detail: spec.Detail, Failure: mmErrString(code), EndpointReset: mmCodeSuggestsEndpointReset(code)})
		r.handle = 0
	}

	dsoundCandidates := append([]DeviceInfo(nil), selection.targets...)
	dsoundCandidates = append(dsoundCandidates, selection.remaining...)
	for _, device := range dsoundCandidates {
		for _, fc := range formatCandidates {
			r.applyFormat(fc)
			if err := r.dsoundProbe(); err == nil {
				if err := r.dsoundTryOpen(); err == nil {
					r.mode = recorderModeDSound
					r.activeDetail = fmt.Sprintf("backend=%s detail=%s", captureBackendDSound, fmt.Sprintf("%s target=%s", formatDetail(fc), displayDeviceName(device.Name)))
					if err := r.dsoundStart(); err == nil {
						return nil
					}
				}
			} else {
				attempts = append(attempts, openAttempt{Backend: captureBackendDSound, Detail: fmt.Sprintf("%s target=%s", formatDetail(fc), displayDeviceName(device.Name)), Failure: err.Error()})
			}
			r.dsoundReleaseInterfaces()
		}
	}

	if attempt+1 < 3 {
		time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
		return r.tryOpen(attempt+1, attempts)
	}

	return newOpenFailure(selectFailureDeviceLabel(selection, devices), uint32(len(devices)), attempts)
}

func (r *Recorder) applyFormat(fc formatCandidate) {
	r.format = waveFormatEx{
		FormatTag:      waveFormatPCM,
		Channels:       fc.channels,
		SamplesPerSec:  fc.samplesPerSec,
		BitsPerSample:  fc.bits,
		BlockAlign:     fc.channels * (fc.bits / 8),
		AvgBytesPerSec: fc.samplesPerSec * uint32(fc.channels) * uint32(fc.bits/8),
	}
}

func (r *Recorder) waveInOpenUsing(deviceID uint32, flags uint32) (uintptr, uint32) {
	r1, _, _ := procWaveInOpen.Call(uintptr(unsafe.Pointer(&r.handle)), uintptr(deviceID), uintptr(unsafe.Pointer(&r.format)), 0, 0, uintptr(flags))
	return r1, deviceID
}

func waveInGetNumDevs() uint32 {
	r1, _, _ := procWaveInGetNumDevs.Call()
	return uint32(r1)
}

func waveInDevCaps(deviceID uint32) string {
	var caps waveDevCaps
	r1, _, _ := procWaveInGetDevCaps.Call(uintptr(deviceID), uintptr(unsafe.Pointer(&caps)), unsafe.Sizeof(caps))
	if r1 != 0 {
		return "(unknown)"
	}
	return syscall.UTF16ToString(caps.ProductName[:])
}

func (r *Recorder) dsoundTryOpen() error {
	ret, _, _ := procDirectSoundCaptureCreate8.Call(0, uintptr(unsafe.Pointer(&r.pDSC)), 0)
	if ret != 0 {
		return fmt.Errorf("DirectSoundCaptureCreate8: 0x%X", ret)
	}
	if r.pDSC == 0 {
		return fmt.Errorf("DirectSoundCaptureCreate8: null interface")
	}
	return nil
}

func (r *Recorder) dsoundProbe() error {
	if err := r.dsoundTryOpen(); err != nil {
		return err
	}
	defer r.dsoundReleaseInterfaces()
	if err := r.dsoundCreateBuffer(); err != nil {
		return fmt.Errorf("CreateCaptureBuffer: %w", err)
	}
	if err := r.dsoundStartCapture(); err != nil {
		return fmt.Errorf("Start: %w", err)
	}
	ret, _, _ := syscall.Syscall(dsVtbl(r.pDSCB, dsBufferStopMethod), 1, r.pDSCB, 0, 0)
	if ret != 0 {
		return fmt.Errorf("Stop: 0x%X", ret)
	}
	return nil
}

func (r *Recorder) dsoundCreateBuffer() error {
	f := r.format
	var wfxRaw [18]byte
	putU16(wfxRaw[0:], f.FormatTag)
	putU16(wfxRaw[2:], f.Channels)
	putU32(wfxRaw[4:], f.SamplesPerSec)
	putU32(wfxRaw[8:], f.AvgBytesPerSec)
	putU16(wfxRaw[12:], f.BlockAlign)
	putU16(wfxRaw[14:], f.BitsPerSample)
	putU16(wfxRaw[16:], f.Size)

	type dscBufDesc struct {
		size        uint32
		flags       uint32
		bufferBytes uint32
		reserved    uint32
		pwfxFormat  *byte
		guid3D      [16]byte
	}

	desc := dscBufDesc{size: uint32(unsafe.Sizeof(dscBufDesc{})), flags: 0, bufferBytes: uint32(len(r.buffer)), pwfxFormat: &wfxRaw[0]}
	var pBuf uintptr
	ret, _, _ := syscall.Syscall6(dsVtbl(r.pDSC, dsCaptureCreateBuffer), 3, r.pDSC, uintptr(unsafe.Pointer(&desc)), uintptr(unsafe.Pointer(&pBuf)), 0, 0, 0)
	if ret != 0 {
		return fmt.Errorf("0x%X", ret)
	}
	if pBuf == 0 {
		return fmt.Errorf("null capture buffer")
	}
	r.pDSCB = pBuf
	r.dscBuf.bytes = desc.bufferBytes
	r.dscBuf.offset = 0
	return nil
}

func (r *Recorder) dsoundStartCapture() error {
	ret, _, _ := syscall.Syscall(dsVtbl(r.pDSCB, dsBufferStartMethod), 2, r.pDSCB, 0, 0)
	if ret != 0 {
		return fmt.Errorf("0x%X", ret)
	}
	return nil
}

func (r *Recorder) dsoundStop() (int, error) {
	if r.pDSCB == 0 {
		return 0, fmt.Errorf("no buffer")
	}
	ret, _, _ := syscall.Syscall(dsVtbl(r.pDSCB, dsBufferStopMethod), 1, r.pDSCB, 0, 0)
	if ret != 0 {
		r.dsoundReleaseInterfaces()
		return 0, fmt.Errorf("Stop: 0x%X", ret)
	}
	recorded, err := r.dsoundReadBuffer()
	r.dsoundReleaseInterfaces()
	if err != nil {
		return 0, err
	}
	return recorded, nil
}

func (r *Recorder) dsoundReadBuffer() (int, error) {
	var capturePos uint32
	var readPos uint32
	ret, _, _ := syscall.Syscall(dsVtbl(r.pDSCB, dsBufferGetCurrentPosition), 3, r.pDSCB, uintptr(unsafe.Pointer(&capturePos)), uintptr(unsafe.Pointer(&readPos)))
	if ret != 0 {
		return 0, fmt.Errorf("GetCurrentPosition: 0x%X", ret)
	}
	if capturePos == 0 {
		return 0, nil
	}
	if capturePos > uint32(len(r.buffer)) {
		capturePos = uint32(len(r.buffer))
	}

	var ptr1 uintptr
	var bytes1 uintptr
	var ptr2 uintptr
	var bytes2 uintptr
	ret, _, _ = syscall.Syscall9(dsVtbl(r.pDSCB, dsBufferLockMethod), 8, r.pDSCB, 0, uintptr(capturePos), uintptr(unsafe.Pointer(&ptr1)), uintptr(unsafe.Pointer(&bytes1)), uintptr(unsafe.Pointer(&ptr2)), uintptr(unsafe.Pointer(&bytes2)), 0, 0)
	if ret != 0 {
		return 0, fmt.Errorf("Lock: 0x%X", ret)
	}

	copied := copyLockedBytes(r.buffer, ptr1, bytes1)
	copied += copyLockedBytes(r.buffer[copied:], ptr2, bytes2)
	if err := r.dsoundUnlock(ptr1, bytes1, ptr2, bytes2); err != nil {
		return 0, err
	}
	r.dscBuf.offset = uint32(copied)
	return copied, nil
}

func copyLockedBytes(dst []byte, ptr uintptr, size uintptr) int {
	if ptr == 0 || size == 0 || len(dst) == 0 {
		return 0
	}
	n := int(size)
	if n > len(dst) {
		n = len(dst)
	}
	src := unsafe.Slice((*byte)(unsafe.Pointer(ptr)), n)
	return copy(dst, src)
}

func (r *Recorder) dsoundUnlock(ptr1 uintptr, bytes1 uintptr, ptr2 uintptr, bytes2 uintptr) error {
	ret, _, _ := syscall.Syscall6(dsVtbl(r.pDSCB, dsBufferUnlockMethod), 5, r.pDSCB, ptr1, bytes1, ptr2, bytes2, 0)
	if ret != 0 {
		return fmt.Errorf("Unlock: 0x%X", ret)
	}
	return nil
}

func (r *Recorder) dsoundReleaseInterfaces() {
	if r.pDSCB != 0 {
		syscall.Syscall(dsVtbl(r.pDSCB, dsMethodRelease), 1, r.pDSCB, 0, 0)
		r.pDSCB = 0
	}
	if r.pDSC != 0 {
		syscall.Syscall(dsVtbl(r.pDSC, dsMethodRelease), 1, r.pDSC, 0, 0)
		r.pDSC = 0
	}
}

func (r *Recorder) resetState() {
	r.mode = ""
	r.handle = 0
	r.pDSC = 0
	r.pDSCB = 0
	r.dscBuf = struct {
		bytes  uint32
		offset uint32
	}{}
	r.wasapiStopCh = nil
	r.wasapiDoneCh = nil
	r.wasapiDevice = ""
	r.activeDetail = ""
	r.ffmpeg = nil
	r.active = false
}

func dsVtbl(obj uintptr, method int) uintptr {
	return *(*uintptr)(unsafe.Pointer(*(*uintptr)(unsafe.Pointer(obj)) + uintptr(method)*unsafe.Sizeof(uintptr(0))))
}

func putU16(b []byte, v uint16) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
}

func putU32(b []byte, v uint32) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
	b[2] = byte(v >> 16)
	b[3] = byte(v >> 24)
}
