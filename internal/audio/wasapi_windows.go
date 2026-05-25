//go:build windows

package audio

import (
	"encoding/binary"
	"fmt"
	"math"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const (
	clsctxAll                = 23
	coinitMultithreaded      = 0x0
	deviceStateActive        = 0x1
	eDataFlowCapture         = 1
	eroleMultimedia          = 1
	stgmRead                 = 0x00000000
	audclntShareModeShared   = 0
	audclntBufferFlagsSilent = 0x2
	waveFormatIEEEFloat      = 3
	waveFormatExtensibleTag  = 0xFFFE
	wasapiPollInterval       = 20 * time.Millisecond

	audclntStreamFlagsAutoConvertPCM    = 0x80000000
	audclntStreamFlagsSrcDefaultQuality = 0x08000000
)

const (
	mmDeviceEnumEndpoints = 3
	mmDeviceGetDefault    = 4
	mmDeviceGetByID       = 5

	mmCollectionGetCount = 3
	mmCollectionItem     = 4

	mmDeviceActivate          = 3
	mmDeviceOpenPropertyStore = 4
	mmDeviceGetID             = 5

	propertyStoreGetValue = 5

	audioClientInitializeMethod        = 3
	audioClientIsFormatSupportedMethod = 5
	audioClientGetMixFormatMethod      = 8
	audioClientStartMethod             = 10
	audioClientStopMethod              = 11
	audioClientGetServiceMethod        = 14

	hresultSFalse = uintptr(1)

	captureClientGetBufferMethod         = 3
	captureClientReleaseBufferMethod     = 4
	captureClientGetNextPacketSizeMethod = 5
)

type guid struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

type propertyKey struct {
	Fmtid guid
	Pid   uint32
}

type propVariant struct {
	VT         uint16
	Reserved1  uint16
	Reserved2  uint16
	Reserved3  uint16
	Value      uintptr
	ValueExtra uintptr
}

type waveFormatExtensible struct {
	Format      waveFormatEx
	Samples     uint16
	ChannelMask uint32
	SubFormat   guid
}

type wasapiInputFormat struct {
	channels      uint16
	samplesPerSec uint32
	bitsPerSample uint16
	validBits     uint16
	isFloat       bool
	channelMask   uint32
}

type wasapiStartResult struct {
	format   waveFormatEx
	device   string
	attempts []openAttempt
	startErr error
}

type wasapiInitAttempt struct {
	duration int64
}

type wasapiNegotiatedFormat struct {
	formatPtr uintptr
	input     wasapiInputFormat
	output    waveFormatEx
	needsFree bool
}

type wasapiCaptureResult struct {
	data []byte
	err  error
}

var (
	ole32                = syscall.NewLazyDLL("ole32.dll")
	procCoInitializeEx   = ole32.NewProc("CoInitializeEx")
	procCoUninitialize   = ole32.NewProc("CoUninitialize")
	procCoCreateInstance = ole32.NewProc("CoCreateInstance")
	procCoTaskMemFree    = ole32.NewProc("CoTaskMemFree")
	procPropVariantClear = ole32.NewProc("PropVariantClear")
)

var (
	clsidMMDeviceEnumerator  = guid{0xBCDE0395, 0xE52F, 0x467C, [8]byte{0x8E, 0x3D, 0xC4, 0x57, 0x92, 0x91, 0x69, 0x2E}}
	iidIMMDeviceEnumerator   = guid{0xA95664D2, 0x9614, 0x4F35, [8]byte{0xA7, 0x46, 0xDE, 0x8D, 0xB6, 0x36, 0x17, 0xE6}}
	iidIAudioClient          = guid{0x1CB9AD4C, 0xDBFA, 0x4C32, [8]byte{0xB1, 0x78, 0xC2, 0xF5, 0x68, 0xA7, 0x03, 0xB2}}
	iidIAudioCaptureClient   = guid{0xC8ADBD64, 0xE71E, 0x48A0, [8]byte{0xA4, 0xDE, 0x18, 0x5C, 0x39, 0x5C, 0xD3, 0x17}}
	pkeyDeviceFriendlyName   = propertyKey{Fmtid: guid{0xA45C254E, 0xDF1C, 0x4EFD, [8]byte{0x80, 0x20, 0x67, 0xD1, 0x46, 0xA8, 0x50, 0xE0}}, Pid: 14}
	ksDataFormatSubtypePCM   = guid{0x00000001, 0x0000, 0x0010, [8]byte{0x80, 0x00, 0x00, 0xAA, 0x00, 0x38, 0x9B, 0x71}}
	ksDataFormatSubtypeFloat = guid{0x00000003, 0x0000, 0x0010, [8]byte{0x80, 0x00, 0x00, 0xAA, 0x00, 0x38, 0x9B, 0x71}}
)

func EnumerateWASAPIInputDevices() ([]DeviceInfo, error) {
	var devices []DeviceInfo
	err := withCOM(func() error {
		enumerator, err := newMMDeviceEnumerator()
		if err != nil {
			return err
		}
		defer releaseCOM(enumerator)

		listed, _, err := wasapiListEndpoints(enumerator)
		if err != nil {
			return err
		}
		devices = listed
		return nil
	})
	return devices, err
}

func (r *Recorder) startWASAPI() ([]openAttempt, error) {
	stopCh := make(chan struct{})
	initCh := make(chan wasapiStartResult, 1)
	doneCh := make(chan wasapiCaptureResult, 1)
	go r.runWASAPIWorker(stopCh, initCh, doneCh)

	result := <-initCh
	if result.startErr != nil {
		close(stopCh)
		return result.attempts, result.startErr
	}
	r.wasapiStopCh = stopCh
	r.wasapiDoneCh = doneCh
	r.mode = recorderModeWASAPI
	r.format = result.format
	r.wasapiDevice = result.device
	return nil, nil
}

func (r *Recorder) runWASAPIWorker(stopCh <-chan struct{}, initCh chan<- wasapiStartResult, doneCh chan<- wasapiCaptureResult) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := coInitialize(); err != nil {
		initCh <- wasapiStartResult{attempts: []openAttempt{{Backend: captureBackendWASAPI, Detail: "init", Failure: err.Error()}}, startErr: err}
		return
	}
	defer coUninitialize()

	enumerator, err := newMMDeviceEnumerator()
	if err != nil {
		initCh <- wasapiStartResult{attempts: []openAttempt{{Backend: captureBackendWASAPI, Detail: "enumeration", Failure: err.Error()}}, startErr: err}
		return
	}
	defer releaseCOM(enumerator)

	endpoints, _, err := wasapiListEndpoints(enumerator)
	if err != nil {
		initCh <- wasapiStartResult{attempts: []openAttempt{{Backend: captureBackendWASAPI, Detail: "enumeration", Failure: err.Error()}}, startErr: err}
		return
	}
	if len(endpoints) == 0 {
		err := fmt.Errorf("no active WASAPI capture endpoints found")
		initCh <- wasapiStartResult{attempts: []openAttempt{{Backend: captureBackendWASAPI, Detail: "enumeration", Failure: err.Error()}}, startErr: err}
		return
	}

	selection, err := resolveInputDeviceSelection(endpoints, r.options)
	if err != nil {
		initCh <- wasapiStartResult{startErr: err}
		return
	}

	ordered := orderWASAPIEndpoints(selection)
	attempts := make([]openAttempt, 0, len(ordered))
	for _, endpoint := range ordered {
		audioClient, captureClient, input, output, openAttempts, err := tryOpenWASAPIEndpoint(enumerator, endpoint)
		attempts = append(attempts, openAttempts...)
		if err != nil {
			continue
		}
		if err := wasapiStartClient(audioClient); err != nil {
			releaseCOM(captureClient)
			releaseCOM(audioClient)
			attempts = append(attempts, openAttempt{Backend: captureBackendWASAPI, Detail: fmt.Sprintf("target=%s", displayDeviceName(endpoint.Name)), Failure: err.Error()})
			continue
		}

		initCh <- wasapiStartResult{format: output, device: endpoint.Name}
		data, captureErr := runWASAPICaptureLoop(audioClient, captureClient, input, stopCh)
		releaseCOM(captureClient)
		releaseCOM(audioClient)
		doneCh <- wasapiCaptureResult{data: data, err: captureErr}
		return
	}

	initCh <- wasapiStartResult{attempts: attempts, startErr: fmt.Errorf("WASAPI capture initialization failed")}
}

func tryOpenWASAPIEndpoint(enumerator uintptr, endpoint DeviceInfo) (uintptr, uintptr, wasapiInputFormat, waveFormatEx, []openAttempt, error) {
	attempts := make([]openAttempt, 0, 4)
	label := displayDeviceName(endpoint.Name)

	strategies := []struct {
		detail string
		open   func(uintptr, DeviceInfo) (uintptr, uintptr, wasapiInputFormat, waveFormatEx, error)
	}{
		{detail: "mixformat", open: openWASAPIAudioClient},
		{detail: "mixformat-autoconvert", open: openWASAPIAudioClientMixAutoConvert},
		{detail: "pcm-autoconvert", open: openWASAPIAudioClientAutoConvert},
	}
	for _, strategy := range strategies {
		audioClient, captureClient, input, output, err := strategy.open(enumerator, endpoint)
		if err == nil {
			return audioClient, captureClient, input, output, attempts, nil
		}
		attempts = append(attempts, openAttempt{
			Backend:       captureBackendWASAPI,
			Detail:        fmt.Sprintf("target=%s %s", label, strategy.detail),
			Failure:       err.Error(),
			EndpointReset: wasapiErrorSuggestsEndpointReset(err),
		})
	}
	return 0, 0, wasapiInputFormat{}, waveFormatEx{}, attempts, fmt.Errorf("WASAPI capture initialization failed")
}

func openWASAPIAudioClient(enumerator uintptr, endpoint DeviceInfo) (uintptr, uintptr, wasapiInputFormat, waveFormatEx, error) {
	return openWASAPIAudioClientWithMixFormat(enumerator, endpoint, 0)
}

func openWASAPIAudioClientMixAutoConvert(enumerator uintptr, endpoint DeviceInfo) (uintptr, uintptr, wasapiInputFormat, waveFormatEx, error) {
	flags := uintptr(audclntStreamFlagsAutoConvertPCM | audclntStreamFlagsSrcDefaultQuality)
	return openWASAPIAudioClientWithMixFormat(enumerator, endpoint, flags)
}

func openWASAPIAudioClientWithMixFormat(enumerator uintptr, endpoint DeviceInfo, streamFlags uintptr) (uintptr, uintptr, wasapiInputFormat, waveFormatEx, error) {
	device, err := mmDeviceByID(enumerator, endpoint.EndpointID)
	if err != nil {
		return 0, 0, wasapiInputFormat{}, waveFormatEx{}, err
	}
	defer releaseCOM(device)

	audioClient, err := mmDeviceActivateAudioClient(device)
	if err != nil {
		return 0, 0, wasapiInputFormat{}, waveFormatEx{}, err
	}
	formatPtr, err := audioClientGetMixFormat(audioClient)
	if err != nil {
		releaseCOM(audioClient)
		return 0, 0, wasapiInputFormat{}, waveFormatEx{}, err
	}

	negotiated, err := negotiateWASAPISharedFormat(audioClient, formatPtr, streamFlags)
	coTaskMemFree(formatPtr)
	if err != nil {
		releaseCOM(audioClient)
		return 0, 0, wasapiInputFormat{}, waveFormatEx{}, err
	}
	defer releaseWASAPINegotiatedFormat(negotiated)

	captureClient, err := audioClientGetCaptureClient(audioClient)
	if err != nil {
		releaseCOM(audioClient)
		return 0, 0, wasapiInputFormat{}, waveFormatEx{}, err
	}
	return audioClient, captureClient, negotiated.input, negotiated.output, nil
}

func openWASAPIAudioClientAutoConvert(enumerator uintptr, endpoint DeviceInfo) (uintptr, uintptr, wasapiInputFormat, waveFormatEx, error) {
	device, err := mmDeviceByID(enumerator, endpoint.EndpointID)
	if err != nil {
		return 0, 0, wasapiInputFormat{}, waveFormatEx{}, err
	}
	defer releaseCOM(device)

	formats := defaultWASAPIAutoConvertFormatSpecs()

	var lastErr error
	for _, spec := range formats {
		audioClient, err := mmDeviceActivateAudioClient(device)
		if err != nil {
			lastErr = err
			continue
		}

		wfx := waveFormatEx{
			FormatTag:      waveFormatPCM,
			Channels:       spec.channels,
			SamplesPerSec:  spec.rate,
			BitsPerSample:  spec.bits,
			BlockAlign:     spec.channels * (spec.bits / 8),
			AvgBytesPerSec: spec.rate * uint32(spec.channels) * uint32(spec.bits/8),
		}
		flags := uintptr(audclntStreamFlagsAutoConvertPCM | audclntStreamFlagsSrcDefaultQuality)
		negotiated, err := negotiateWASAPISharedFormat(audioClient, uintptr(unsafe.Pointer(&wfx)), flags)
		if err != nil {
			releaseCOM(audioClient)
			lastErr = err
			continue
		}

		captureClient, err := audioClientGetCaptureClient(audioClient)
		if err != nil {
			releaseWASAPINegotiatedFormat(negotiated)
			releaseCOM(audioClient)
			lastErr = err
			continue
		}
		output := negotiated.output
		input := negotiated.input
		releaseWASAPINegotiatedFormat(negotiated)
		return audioClient, captureClient, input, output, nil
	}

	if lastErr != nil {
		return 0, 0, wasapiInputFormat{}, waveFormatEx{}, lastErr
	}
	return 0, 0, wasapiInputFormat{}, waveFormatEx{}, fmt.Errorf("no format supported")
}

func runWASAPICaptureLoop(audioClient uintptr, captureClient uintptr, input wasapiInputFormat, stopCh <-chan struct{}) ([]byte, error) {
	defer wasapiStopClient(audioClient)
	buffer := make([]byte, 0, 32000*4)

	for {
		if err := appendWASAPIPackets(captureClient, input, &buffer); err != nil {
			return nil, err
		}
		select {
		case <-stopCh:
			if err := appendWASAPIPackets(captureClient, input, &buffer); err != nil {
				return nil, err
			}
			return buffer, nil
		default:
			time.Sleep(wasapiPollInterval)
		}
	}
}

func appendWASAPIPackets(captureClient uintptr, input wasapiInputFormat, dst *[]byte) error {
	for {
		frames, err := captureClientGetNextPacketSize(captureClient)
		if err != nil {
			return err
		}
		if frames == 0 {
			return nil
		}
		dataPtr, packetFrames, flags, err := captureClientGetBuffer(captureClient)
		if err != nil {
			return err
		}
		if flags&audclntBufferFlagsSilent != 0 || dataPtr == 0 {
			appendSilentFrames(dst, packetFrames)
		} else {
			appendConvertedFrames(dst, dataPtr, packetFrames, input)
		}
		if err := captureClientReleaseBuffer(captureClient, packetFrames); err != nil {
			return err
		}
	}
}

func appendSilentFrames(dst *[]byte, frames uint32) {
	if frames == 0 {
		return
	}
	silence := make([]byte, int(frames)*2)
	*dst = append(*dst, silence...)
}

func appendConvertedFrames(dst *[]byte, dataPtr uintptr, frames uint32, input wasapiInputFormat) {
	channels := int(input.channels)
	if channels <= 0 {
		channels = 1
	}
	totalSamples := int(frames) * channels
	bytesPerSample := int((input.bitsPerSample + 7) / 8)
	if bytesPerSample <= 0 {
		bytesPerSample = 2
	}

	if input.isFloat {
		samples := unsafe.Slice((*float32)(unsafe.Pointer(dataPtr)), totalSamples)
		for frame := 0; frame < int(frames); frame++ {
			mono := float32(0)
			for channel := 0; channel < channels; channel++ {
				mono += samples[frame*channels+channel]
			}
			mono /= float32(channels)
			mono = clampFloat32(mono)
			value := int16(math.Round(float64(mono * 32767)))
			*dst = append(*dst, byte(value), byte(uint16(value)>>8))
		}
		return
	}

	raw := unsafe.Slice((*byte)(unsafe.Pointer(dataPtr)), int(frames)*channels*bytesPerSample)
	for frame := 0; frame < int(frames); frame++ {
		acc := 0.0
		for channel := 0; channel < channels; channel++ {
			offset := (frame*channels + channel) * bytesPerSample
			acc += pcmSampleAsFloat64(raw[offset:offset+bytesPerSample], input.bitsPerSample)
		}
		mono := clampFloat64(acc / float64(channels))
		value := int16(math.Round(mono * 32767))
		*dst = append(*dst, byte(value), byte(uint16(value)>>8))
	}
}

func pcmSampleAsFloat64(data []byte, bits uint16) float64 {
	switch bits {
	case 8:
		return (float64(int(data[0])) - 128.0) / 128.0
	case 16:
		v := int16(uint16(data[0]) | uint16(data[1])<<8)
		return float64(v) / 32768.0
	case 24:
		v := int32(data[0]) | int32(data[1])<<8 | int32(data[2])<<16
		if v&0x800000 != 0 {
			v |= ^0xFFFFFF
		}
		return float64(v) / 8388608.0
	case 32:
		v := int32(uint32(data[0]) | uint32(data[1])<<8 | uint32(data[2])<<16 | uint32(data[3])<<24)
		return float64(v) / 2147483648.0
	default:
		return 0
	}
}

func clampFloat32(v float32) float32 {
	if v > 1 {
		return 1
	}
	if v < -1 {
		return -1
	}
	return v
}

func clampFloat64(v float64) float64 {
	if v > 1 {
		return 1
	}
	if v < -1 {
		return -1
	}
	return v
}

func parseWASAPIInputFormat(formatPtr uintptr) (wasapiInputFormat, error) {
	base := (*waveFormatEx)(unsafe.Pointer(formatPtr))
	result := wasapiInputFormat{
		channels:      base.Channels,
		samplesPerSec: base.SamplesPerSec,
		bitsPerSample: base.BitsPerSample,
		validBits:     base.BitsPerSample,
	}
	switch base.FormatTag {
	case waveFormatPCM:
		return result, nil
	case waveFormatIEEEFloat:
		result.isFloat = true
		return result, nil
	case waveFormatExtensibleTag:
		extBytes := unsafe.Slice((*byte)(unsafe.Pointer(formatPtr)), 18+int(base.Size))
		if len(extBytes) < 40 {
			return wasapiInputFormat{}, fmt.Errorf("unsupported extensible format payload size %d", len(extBytes))
		}
		result.validBits = binary.LittleEndian.Uint16(extBytes[18:20])
		result.channelMask = binary.LittleEndian.Uint32(extBytes[20:24])
		subFormat := parseGUID(extBytes[24:40])
		if guidEqual(subFormat, ksDataFormatSubtypePCM) {
			return result, nil
		}
		if guidEqual(subFormat, ksDataFormatSubtypeFloat) {
			result.isFloat = true
			return result, nil
		}
		return wasapiInputFormat{}, fmt.Errorf("unsupported mix subformat 0x%X", subFormat.Data1)
	default:
		return wasapiInputFormat{}, fmt.Errorf("unsupported mix format tag 0x%X", base.FormatTag)
	}
}

func defaultWASAPIAutoConvertFormatSpecs() []struct {
	channels uint16
	rate     uint32
	bits     uint16
} {
	return []struct {
		channels uint16
		rate     uint32
		bits     uint16
	}{
		{2, 44100, 16},
		{1, 44100, 16},
		{2, 48000, 16},
		{1, 48000, 16},
		{1, 16000, 16},
		{2, 16000, 16},
	}
}

func wasapiInitAttempts() []wasapiInitAttempt {
	return []wasapiInitAttempt{
		{duration: 0},
		{duration: 10_000_000},
	}
}

func negotiateWASAPISharedFormat(audioClient uintptr, requestedFormatPtr uintptr, streamFlags uintptr) (wasapiNegotiatedFormat, error) {
	actualFormatPtr, needsFree, err := audioClientNegotiateSharedFormat(audioClient, requestedFormatPtr)
	if err != nil {
		return wasapiNegotiatedFormat{}, err
	}
	input, err := parseWASAPIInputFormat(actualFormatPtr)
	if err != nil {
		if needsFree {
			coTaskMemFree(actualFormatPtr)
		}
		return wasapiNegotiatedFormat{}, err
	}
	if err := initializeWASAPISharedClient(audioClient, actualFormatPtr, streamFlags); err != nil {
		if needsFree {
			coTaskMemFree(actualFormatPtr)
		}
		return wasapiNegotiatedFormat{}, err
	}
	return wasapiNegotiatedFormat{
		formatPtr: actualFormatPtr,
		input:     input,
		output:    outputFormatFromInput(input),
		needsFree: needsFree,
	}, nil
}

func releaseWASAPINegotiatedFormat(format wasapiNegotiatedFormat) {
	if format.needsFree {
		coTaskMemFree(format.formatPtr)
	}
}

func audioClientNegotiateSharedFormat(audioClient uintptr, requestedFormatPtr uintptr) (uintptr, bool, error) {
	closestPtr, hr, err := audioClientIsFormatSupported(audioClient, requestedFormatPtr)
	if err != nil {
		return 0, false, err
	}
	switch hr {
	case 0:
		return requestedFormatPtr, false, nil
	case hresultSFalse:
		if closestPtr == 0 {
			return 0, false, fmt.Errorf("IAudioClient::IsFormatSupported returned closest-match without format")
		}
		return closestPtr, true, nil
	default:
		if closestPtr != 0 {
			coTaskMemFree(closestPtr)
		}
		return 0, false, fmt.Errorf("IAudioClient::IsFormatSupported: unexpected HRESULT 0x%X", uint32(hr))
	}
}

func initializeWASAPISharedClient(audioClient uintptr, formatPtr uintptr, streamFlags uintptr) error {
	base := (*waveFormatEx)(unsafe.Pointer(formatPtr))
	var firstErr error
	for _, attempt := range wasapiInitAttempts() {
		if err := audioClientInitializeShared(audioClient, formatPtr, attempt.duration, streamFlags); err == nil {
			return nil
		} else if firstErr == nil {
			firstErr = err
		}
	}
	return fmt.Errorf("%w (fmt=0x%X ch=%d rate=%d bits=%d durations=%s)",
		firstErr, base.FormatTag, base.Channels, base.SamplesPerSec, base.BitsPerSample, describeWASAPIInitAttempts())
}

func describeWASAPIInitAttempts() string {
	attempts := wasapiInitAttempts()
	parts := make([]string, 0, len(attempts))
	for _, attempt := range attempts {
		parts = append(parts, fmt.Sprintf("%d", attempt.duration))
	}
	return strings.Join(parts, ",")
}

func parseGUID(raw []byte) guid {
	return guid{
		Data1: binary.LittleEndian.Uint32(raw[0:4]),
		Data2: binary.LittleEndian.Uint16(raw[4:6]),
		Data3: binary.LittleEndian.Uint16(raw[6:8]),
		Data4: [8]byte(raw[8:16]),
	}
}

func outputFormatFromInput(input wasapiInputFormat) waveFormatEx {
	return waveFormatEx{
		FormatTag:      waveFormatPCM,
		Channels:       1,
		SamplesPerSec:  input.samplesPerSec,
		BitsPerSample:  16,
		BlockAlign:     2,
		AvgBytesPerSec: input.samplesPerSec * 2,
	}
}

func withCOM(fn func() error) error {
	if err := coInitialize(); err != nil {
		return err
	}
	defer coUninitialize()
	return fn()
}

func coInitialize() error {
	hr, _, _ := procCoInitializeEx.Call(0, coinitMultithreaded)
	if hr != 0 && hr != 1 {
		return hresultError("CoInitializeEx", hr)
	}
	return nil
}

func coUninitialize() {
	procCoUninitialize.Call()
}

func newMMDeviceEnumerator() (uintptr, error) {
	var enumerator uintptr
	hr, _, _ := procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsidMMDeviceEnumerator)),
		0,
		clsctxAll,
		uintptr(unsafe.Pointer(&iidIMMDeviceEnumerator)),
		uintptr(unsafe.Pointer(&enumerator)),
	)
	if hr != 0 {
		return 0, hresultError("CoCreateInstance(MMDeviceEnumerator)", hr)
	}
	if enumerator == 0 {
		return 0, fmt.Errorf("CoCreateInstance(MMDeviceEnumerator): null interface")
	}
	return enumerator, nil
}

func wasapiListEndpoints(enumerator uintptr) ([]DeviceInfo, string, error) {
	defaultID, _ := defaultCaptureEndpointID(enumerator)
	collection, err := mmEnumerateActiveEndpoints(enumerator)
	if err != nil {
		return nil, "", err
	}
	defer releaseCOM(collection)

	count, err := mmCollectionCount(collection)
	if err != nil {
		return nil, "", err
	}
	devices := make([]DeviceInfo, 0, count)
	for i := uint32(0); i < count; i++ {
		device, err := mmCollectionDevice(collection, i)
		if err != nil {
			return nil, "", err
		}
		info, infoErr := mmEndpointInfo(device, i)
		releaseCOM(device)
		if infoErr != nil {
			return nil, "", infoErr
		}
		devices = append(devices, info)
	}
	return devices, defaultID, nil
}

func defaultCaptureEndpointID(enumerator uintptr) (string, error) {
	device, err := mmDefaultCaptureEndpoint(enumerator)
	if err != nil {
		return "", err
	}
	defer releaseCOM(device)
	return mmDeviceID(device)
}

func mmEnumerateActiveEndpoints(enumerator uintptr) (uintptr, error) {
	var collection uintptr
	hr, _, _ := syscall.Syscall6(
		comVtbl(enumerator, mmDeviceEnumEndpoints),
		4,
		enumerator,
		eDataFlowCapture,
		deviceStateActive,
		uintptr(unsafe.Pointer(&collection)),
		0,
		0,
	)
	if hr != 0 {
		return 0, hresultError("IMMDeviceEnumerator::EnumAudioEndpoints", hr)
	}
	return collection, nil
}

func mmDefaultCaptureEndpoint(enumerator uintptr) (uintptr, error) {
	var device uintptr
	hr, _, _ := syscall.Syscall6(
		comVtbl(enumerator, mmDeviceGetDefault),
		4,
		enumerator,
		eDataFlowCapture,
		eroleMultimedia,
		uintptr(unsafe.Pointer(&device)),
		0,
		0,
	)
	if hr != 0 {
		return 0, hresultError("IMMDeviceEnumerator::GetDefaultAudioEndpoint", hr)
	}
	return device, nil
}

func mmDeviceByID(enumerator uintptr, endpointID string) (uintptr, error) {
	ptr, err := syscall.UTF16PtrFromString(endpointID)
	if err != nil {
		return 0, err
	}
	var device uintptr
	hr, _, _ := syscall.Syscall(
		comVtbl(enumerator, mmDeviceGetByID),
		3,
		enumerator,
		uintptr(unsafe.Pointer(ptr)),
		uintptr(unsafe.Pointer(&device)),
	)
	if hr != 0 {
		return 0, hresultError("IMMDeviceEnumerator::GetDevice", hr)
	}
	return device, nil
}

func mmCollectionCount(collection uintptr) (uint32, error) {
	var count uint32
	hr, _, _ := syscall.Syscall(
		comVtbl(collection, mmCollectionGetCount),
		2,
		collection,
		uintptr(unsafe.Pointer(&count)),
		0,
	)
	if hr != 0 {
		return 0, hresultError("IMMDeviceCollection::GetCount", hr)
	}
	return count, nil
}

func mmCollectionDevice(collection uintptr, index uint32) (uintptr, error) {
	var device uintptr
	hr, _, _ := syscall.Syscall(
		comVtbl(collection, mmCollectionItem),
		3,
		collection,
		uintptr(index),
		uintptr(unsafe.Pointer(&device)),
	)
	if hr != 0 {
		return 0, hresultError("IMMDeviceCollection::Item", hr)
	}
	return device, nil
}

func mmEndpointInfo(device uintptr, id uint32) (DeviceInfo, error) {
	name, err := mmDeviceFriendlyName(device)
	if err != nil {
		return DeviceInfo{}, err
	}
	endpointID, err := mmDeviceID(device)
	if err != nil {
		return DeviceInfo{}, err
	}
	return DeviceInfo{ID: id, Name: name, EndpointID: endpointID}, nil
}

func mmDeviceFriendlyName(device uintptr) (string, error) {
	var store uintptr
	hr, _, _ := syscall.Syscall(
		comVtbl(device, mmDeviceOpenPropertyStore),
		3,
		device,
		stgmRead,
		uintptr(unsafe.Pointer(&store)),
	)
	if hr != 0 {
		return "", hresultError("IMMDevice::OpenPropertyStore", hr)
	}
	defer releaseCOM(store)

	var pv propVariant
	hr, _, _ = syscall.Syscall(
		comVtbl(store, propertyStoreGetValue),
		3,
		store,
		uintptr(unsafe.Pointer(&pkeyDeviceFriendlyName)),
		uintptr(unsafe.Pointer(&pv)),
	)
	if hr != 0 {
		return "", hresultError("IPropertyStore::GetValue", hr)
	}
	defer procPropVariantClear.Call(uintptr(unsafe.Pointer(&pv)))
	if pv.Value == 0 {
		return "", fmt.Errorf("IPropertyStore::GetValue: empty friendly name")
	}
	return utf16PtrToString(pv.Value), nil
}

func mmDeviceID(device uintptr) (string, error) {
	var raw uintptr
	hr, _, _ := syscall.Syscall(
		comVtbl(device, mmDeviceGetID),
		2,
		device,
		uintptr(unsafe.Pointer(&raw)),
		0,
	)
	if hr != 0 {
		return "", hresultError("IMMDevice::GetId", hr)
	}
	defer coTaskMemFree(raw)
	if raw == 0 {
		return "", fmt.Errorf("IMMDevice::GetId: empty id")
	}
	return utf16PtrToString(raw), nil
}

func mmDeviceActivateAudioClient(device uintptr) (uintptr, error) {
	var audioClient uintptr
	hr, _, _ := syscall.Syscall6(
		comVtbl(device, mmDeviceActivate),
		5,
		device,
		uintptr(unsafe.Pointer(&iidIAudioClient)),
		clsctxAll,
		0,
		uintptr(unsafe.Pointer(&audioClient)),
		0,
	)
	if hr != 0 {
		return 0, hresultError("IMMDevice::Activate(IAudioClient)", hr)
	}
	if audioClient == 0 {
		return 0, fmt.Errorf("IMMDevice::Activate(IAudioClient): null interface")
	}
	return audioClient, nil
}

func audioClientGetMixFormat(audioClient uintptr) (uintptr, error) {
	var formatPtr uintptr
	hr, _, _ := syscall.Syscall(
		comVtbl(audioClient, audioClientGetMixFormatMethod),
		2,
		audioClient,
		uintptr(unsafe.Pointer(&formatPtr)),
		0,
	)
	if hr != 0 {
		return 0, hresultError("IAudioClient::GetMixFormat", hr)
	}
	return formatPtr, nil
}

func audioClientIsFormatSupported(audioClient uintptr, formatPtr uintptr) (uintptr, uintptr, error) {
	var closestPtr uintptr
	hr, _, _ := syscall.Syscall6(
		comVtbl(audioClient, audioClientIsFormatSupportedMethod),
		4,
		audioClient,
		uintptr(audclntShareModeShared),
		formatPtr,
		uintptr(unsafe.Pointer(&closestPtr)),
		0,
		0,
	)
	if hr != 0 && hr != hresultSFalse {
		if closestPtr != 0 {
			coTaskMemFree(closestPtr)
		}
		return 0, hr, hresultError("IAudioClient::IsFormatSupported", hr)
	}
	return closestPtr, hr, nil
}

func audioClientInitializeShared(audioClient uintptr, formatPtr uintptr, hnsBufferDuration int64, streamFlags uintptr) error {
	hr, _, _ := syscall.SyscallN(
		comVtbl(audioClient, audioClientInitializeMethod),
		audioClient,
		uintptr(audclntShareModeShared),
		streamFlags,
		uintptr(hnsBufferDuration),
		0,
		formatPtr,
		0,
	)
	if hr != 0 {
		return hresultError("IAudioClient::Initialize", hr)
	}
	return nil
}

func audioClientGetCaptureClient(audioClient uintptr) (uintptr, error) {
	var captureClient uintptr
	hr, _, _ := syscall.Syscall(
		comVtbl(audioClient, audioClientGetServiceMethod),
		3,
		audioClient,
		uintptr(unsafe.Pointer(&iidIAudioCaptureClient)),
		uintptr(unsafe.Pointer(&captureClient)),
	)
	if hr != 0 {
		return 0, hresultError("IAudioClient::GetService(IAudioCaptureClient)", hr)
	}
	if captureClient == 0 {
		return 0, fmt.Errorf("IAudioClient::GetService(IAudioCaptureClient): null interface")
	}
	return captureClient, nil
}

func wasapiStartClient(audioClient uintptr) error {
	hr, _, _ := syscall.Syscall(comVtbl(audioClient, audioClientStartMethod), 1, audioClient, 0, 0)
	if hr != 0 {
		return hresultError("IAudioClient::Start", hr)
	}
	return nil
}

func wasapiStopClient(audioClient uintptr) {
	if audioClient != 0 {
		syscall.Syscall(comVtbl(audioClient, audioClientStopMethod), 1, audioClient, 0, 0)
	}
}

func captureClientGetNextPacketSize(captureClient uintptr) (uint32, error) {
	var frames uint32
	hr, _, _ := syscall.Syscall(
		comVtbl(captureClient, captureClientGetNextPacketSizeMethod),
		2,
		captureClient,
		uintptr(unsafe.Pointer(&frames)),
		0,
	)
	if hr != 0 {
		return 0, hresultError("IAudioCaptureClient::GetNextPacketSize", hr)
	}
	return frames, nil
}

func captureClientGetBuffer(captureClient uintptr) (uintptr, uint32, uint32, error) {
	var data uintptr
	var frames uint32
	var flags uint32
	hr, _, _ := syscall.Syscall6(
		comVtbl(captureClient, captureClientGetBufferMethod),
		6,
		captureClient,
		uintptr(unsafe.Pointer(&data)),
		uintptr(unsafe.Pointer(&frames)),
		uintptr(unsafe.Pointer(&flags)),
		0,
		0,
	)
	if hr != 0 {
		return 0, 0, 0, hresultError("IAudioCaptureClient::GetBuffer", hr)
	}
	return data, frames, flags, nil
}

func captureClientReleaseBuffer(captureClient uintptr, frames uint32) error {
	hr, _, _ := syscall.Syscall(
		comVtbl(captureClient, captureClientReleaseBufferMethod),
		2,
		captureClient,
		uintptr(frames),
		0,
	)
	if hr != 0 {
		return hresultError("IAudioCaptureClient::ReleaseBuffer", hr)
	}
	return nil
}

func utf16PtrToString(ptr uintptr) string {
	if ptr == 0 {
		return ""
	}
	p16 := (*uint16)(unsafe.Pointer(ptr))
	buf := make([]uint16, 0, 64)
	for {
		v := *p16
		if v == 0 {
			break
		}
		buf = append(buf, v)
		p16 = (*uint16)(unsafe.Pointer(uintptr(unsafe.Pointer(p16)) + unsafe.Sizeof(uint16(0))))
	}
	return syscall.UTF16ToString(buf)
}

func releaseCOM(obj uintptr) {
	if obj != 0 {
		syscall.Syscall(comVtbl(obj, 2), 1, obj, 0, 0)
	}
}

func coTaskMemFree(ptr uintptr) {
	if ptr != 0 {
		procCoTaskMemFree.Call(ptr)
	}
}

func comVtbl(obj uintptr, method int) uintptr {
	return *(*uintptr)(unsafe.Pointer(*(*uintptr)(unsafe.Pointer(obj)) + uintptr(method)*unsafe.Sizeof(uintptr(0))))
}

func guidEqual(a, b guid) bool {
	return a == b
}

func hresultError(op string, hr uintptr) error {
	return fmt.Errorf("%s: 0x%X", op, uint32(hr))
}

func wasapiErrorSuggestsEndpointReset(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "IAudioClient::Initialize: 0x80070057")
}
