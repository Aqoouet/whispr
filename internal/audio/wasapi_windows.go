//go:build windows

package audio

import (
	"fmt"
	"unsafe"

	"github.com/moutend/go-wca/pkg/wca"
)

type wasapiSession struct {
	enumerator    *wca.IMMDeviceEnumerator
	device        *wca.IMMDevice
	audioClient   *wca.IAudioClient
	captureClient *wca.IAudioCaptureClient
}

func (s *wasapiSession) release() {
	if s.captureClient != nil {
		s.captureClient.Release()
	}
	if s.audioClient != nil {
		s.audioClient.Release()
	}
	if s.device != nil {
		s.device.Release()
	}
	if s.enumerator != nil {
		s.enumerator.Release()
	}
}

func openWASAPI() (*wasapiSession, error) {
	s := &wasapiSession{}
	if err := wca.CoCreateInstance(
		wca.CLSID_MMDeviceEnumerator, 0, wca.CLSCTX_ALL,
		wca.IID_IMMDeviceEnumerator, &s.enumerator,
	); err != nil {
		return nil, fmt.Errorf("create device enumerator: %w", err)
	}
	if err := s.enumerator.GetDefaultAudioEndpoint(
		wca.ECapture, wca.EConsole, &s.device,
	); err != nil {
		s.release()
		return nil, fmt.Errorf("get default capture endpoint: %w", err)
	}
	if err := s.device.Activate(
		wca.IID_IAudioClient, wca.CLSCTX_ALL, nil, &s.audioClient,
	); err != nil {
		s.release()
		return nil, fmt.Errorf("activate audio client: %w", err)
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
		s.release()
		return nil, fmt.Errorf("audio client initialize: %w", err)
	}
	if err := s.audioClient.GetService(wca.IID_IAudioCaptureClient, &s.captureClient); err != nil {
		s.release()
		return nil, fmt.Errorf("get capture client: %w", err)
	}
	return s, nil
}

func drainPackets(acc *wca.IAudioCaptureClient, buf *[]byte) {
	for {
		var frames uint32
		if err := acc.GetNextPacketSize(&frames); err != nil || frames == 0 {
			return
		}
		var (
			data  *byte
			flags uint32
		)
		if err := acc.GetBuffer(&data, &frames, &flags, nil, nil); err != nil {
			return
		}
		if flags&wca.AUDCLNT_BUFFERFLAGS_SILENT == 0 && frames > 0 {
			n := int(frames) * 2
			src := (*[1 << 28]byte)(unsafe.Pointer(data))[:n:n]
			*buf = append(*buf, src...)
		}
		acc.ReleaseBuffer(frames)
	}
}
