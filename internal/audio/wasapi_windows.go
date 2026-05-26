//go:build windows

package audio

import (
	"fmt"
	"strings"
	"unsafe"

	"github.com/go-ole/go-ole"
	"github.com/moutend/go-wca/pkg/wca"
)

type wasapiSession struct {
	enumerator     *wca.IMMDeviceEnumerator
	device         *wca.IMMDevice
	audioClient    *wca.IAudioClient
	captureClient  *wca.IAudioCaptureClient
	selected       DeviceInfo
	selectedDetail string
}

func (s *wasapiSession) release() {
	s.releaseActiveHandles()
	if s.enumerator != nil {
		s.enumerator.Release()
		s.enumerator = nil
	}
}

func openWASAPI(options Options) (*wasapiSession, error) {
	s := &wasapiSession{}
	if err := wca.CoCreateInstance(
		wca.CLSID_MMDeviceEnumerator, 0, wca.CLSCTX_ALL,
		wca.IID_IMMDeviceEnumerator, &s.enumerator,
	); err != nil {
		return nil, fmt.Errorf("create device enumerator: %w", err)
	}

	devices, err := enumerateCaptureDevices(s.enumerator)
	if err != nil {
		s.release()
		return nil, err
	}
	defaultDevice, err := defaultCaptureDeviceInfo(s.enumerator)
	if err != nil {
		s.release()
		return nil, err
	}
	targets, err := planOpenTargets(devices, defaultDevice, options)
	if err != nil {
		s.release()
		return nil, err
	}
	failures := make([]string, 0, len(targets))
	for _, target := range targets {
		if err := s.openTarget(target); err == nil {
			return s, nil
		} else {
			failures = append(failures, err.Error())
		}
	}
	s.release()
	return nil, fmt.Errorf("capture backend wasapi failed: %s", strings.Join(failures, "; "))
}

func (s *wasapiSession) openTarget(target openTarget) error {
	s.releaseActiveHandles()
	s.selected = target.Device
	s.selectedDetail = target.Detail

	var err error
	s.device, err = openSelectedCaptureDevice(s.enumerator, target.Device)
	if err != nil {
		return captureStageError("open_endpoint", target, err)
	}
	if err := s.device.Activate(
		wca.IID_IAudioClient, wca.CLSCTX_ALL, nil, &s.audioClient,
	); err != nil {
		s.releaseActiveHandles()
		return captureStageError("activate_audio_client", target, err)
	}
	wfx := &wca.WAVEFORMATEX{
		WFormatTag:      wca.WAVE_FORMAT_PCM,
		NChannels:       1,
		NSamplesPerSec:  16000,
		NAvgBytesPerSec: 32000,
		NBlockAlign:     2,
		WBitsPerSample:  16,
	}
	streamFlags := uint32(wca.AUDCLNT_STREAMFLAGS_AUTOCONVERTPCM | wca.AUDCLNT_STREAMFLAGS_SRC_DEFAULT_QUALITY)
	bufDur := wca.REFERENCE_TIME(200 * 10000)
	if err := s.audioClient.Initialize(
		wca.AUDCLNT_SHAREMODE_SHARED, streamFlags, bufDur, 0, wfx, nil,
	); err != nil {
		s.releaseActiveHandles()
		return captureStageError("initialize_audio_client", target, err)
	}
	if err := s.audioClient.GetService(wca.IID_IAudioCaptureClient, &s.captureClient); err != nil {
		s.releaseActiveHandles()
		return captureStageError("get_capture_client", target, err)
	}
	if err := s.audioClient.Start(); err != nil {
		s.releaseActiveHandles()
		return captureStageError("start_audio_client", target, err)
	}
	return nil
}

func (s *wasapiSession) releaseActiveHandles() {
	if s.captureClient != nil {
		s.captureClient.Release()
		s.captureClient = nil
	}
	if s.audioClient != nil {
		s.audioClient.Release()
		s.audioClient = nil
	}
	if s.device != nil {
		s.device.Release()
		s.device = nil
	}
}

func captureStageError(stage string, target openTarget, err error) error {
	return fmt.Errorf("stage=%s device=%q endpoint=%q selection=%s: %w", stage, target.Device.Name, target.Device.EndpointID, target.Detail, err)
}

func enumerateCaptureDevices(enumerator *wca.IMMDeviceEnumerator) ([]DeviceInfo, error) {
	var collection *wca.IMMDeviceCollection
	if err := enumerator.EnumAudioEndpoints(wca.ECapture, wca.DEVICE_STATE_ACTIVE, &collection); err != nil {
		return nil, fmt.Errorf("enumerate capture endpoints: %w", err)
	}
	defer collection.Release()

	var count uint32
	if err := collection.GetCount(&count); err != nil {
		return nil, fmt.Errorf("count capture endpoints: %w", err)
	}
	devices := make([]DeviceInfo, 0, count)
	for i := uint32(0); i < count; i++ {
		var device *wca.IMMDevice
		if err := collection.Item(i, &device); err != nil {
			return nil, fmt.Errorf("open capture endpoint #%d: %w", i, err)
		}
		info, err := captureDeviceInfo(device, i)
		device.Release()
		if err != nil {
			return nil, err
		}
		devices = append(devices, info)
	}
	return devices, nil
}

func defaultCaptureDeviceInfo(enumerator *wca.IMMDeviceEnumerator) (DeviceInfo, error) {
	var device *wca.IMMDevice
	if err := enumerator.GetDefaultAudioEndpoint(wca.ECapture, wca.EConsole, &device); err != nil {
		return DeviceInfo{}, fmt.Errorf("get default capture endpoint: %w", err)
	}
	defer device.Release()
	return captureDeviceInfo(device, 0)
}

func openSelectedCaptureDevice(enumerator *wca.IMMDeviceEnumerator, selected DeviceInfo) (*wca.IMMDevice, error) {
	var collection *wca.IMMDeviceCollection
	if err := enumerator.EnumAudioEndpoints(wca.ECapture, wca.DEVICE_STATE_ACTIVE, &collection); err != nil {
		return nil, fmt.Errorf("enumerate capture endpoints: %w", err)
	}
	defer collection.Release()

	var count uint32
	if err := collection.GetCount(&count); err != nil {
		return nil, fmt.Errorf("count capture endpoints: %w", err)
	}
	for i := uint32(0); i < count; i++ {
		var device *wca.IMMDevice
		if err := collection.Item(i, &device); err != nil {
			return nil, fmt.Errorf("open capture endpoint #%d: %w", i, err)
		}
		id, err := captureEndpointID(device)
		if err != nil {
			device.Release()
			return nil, err
		}
		if id == selected.EndpointID {
			return device, nil
		}
		device.Release()
	}
	return nil, fmt.Errorf("capture endpoint not found: %q", selected.Name)
}

func captureDeviceInfo(device *wca.IMMDevice, id uint32) (DeviceInfo, error) {
	endpointID, err := captureEndpointID(device)
	if err != nil {
		return DeviceInfo{}, err
	}
	name, err := captureFriendlyName(device)
	if err != nil {
		return DeviceInfo{}, err
	}
	if name == "" {
		name = endpointID
	}
	return DeviceInfo{ID: id, Name: name, EndpointID: endpointID}, nil
}

func captureEndpointID(device *wca.IMMDevice) (string, error) {
	var endpointID string
	if err := device.GetId(&endpointID); err != nil {
		return "", fmt.Errorf("read capture endpoint id: %w", err)
	}
	return endpointID, nil
}

func captureFriendlyName(device *wca.IMMDevice) (string, error) {
	var store *wca.IPropertyStore
	if err := device.OpenPropertyStore(wca.STGM_READ, &store); err != nil {
		return "", fmt.Errorf("open capture endpoint property store: %w", err)
	}
	defer store.Release()

	var value wca.PROPVARIANT
	if err := store.GetValue(&wca.PKEY_Device_FriendlyName, &value); err != nil {
		return "", fmt.Errorf("read capture endpoint friendly name: %w", err)
	}
	defer ole.VariantClear(&value.VARIANT)
	return strings.TrimSpace(value.String()), nil
}

func drainPackets(acc *wca.IAudioCaptureClient, buf *[]byte) error {
	for {
		var frames uint32
		if err := acc.GetNextPacketSize(&frames); err != nil {
			return fmt.Errorf("next packet size: %w", err)
		}
		if frames == 0 {
			return nil
		}
		var (
			data  *byte
			flags uint32
		)
		if err := acc.GetBuffer(&data, &frames, &flags, nil, nil); err != nil {
			return fmt.Errorf("read packet buffer: %w", err)
		}
		if flags&wca.AUDCLNT_BUFFERFLAGS_SILENT == 0 && frames > 0 {
			n := int(frames) * 2
			src := (*[1 << 28]byte)(unsafe.Pointer(data))[:n:n]
			*buf = append(*buf, src...)
		}
		acc.ReleaseBuffer(frames)
	}
}
