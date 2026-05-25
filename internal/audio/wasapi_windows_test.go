//go:build windows

package audio

import (
	"encoding/binary"
	"testing"
	"unsafe"
)

func TestParseWASAPIInputFormatExtensibleFloatMono(t *testing.T) {
	raw := make([]byte, 40)
	binary.LittleEndian.PutUint16(raw[0:2], waveFormatExtensibleTag)
	binary.LittleEndian.PutUint16(raw[2:4], 1)
	binary.LittleEndian.PutUint32(raw[4:8], 16000)
	binary.LittleEndian.PutUint32(raw[8:12], 64000)
	binary.LittleEndian.PutUint16(raw[12:14], 4)
	binary.LittleEndian.PutUint16(raw[14:16], 32)
	binary.LittleEndian.PutUint16(raw[16:18], 22)
	binary.LittleEndian.PutUint16(raw[18:20], 32)
	binary.LittleEndian.PutUint32(raw[20:24], 0x00000004)
	binary.LittleEndian.PutUint32(raw[24:28], ksDataFormatSubtypeFloat.Data1)
	binary.LittleEndian.PutUint16(raw[28:30], ksDataFormatSubtypeFloat.Data2)
	binary.LittleEndian.PutUint16(raw[30:32], ksDataFormatSubtypeFloat.Data3)
	copy(raw[32:40], ksDataFormatSubtypeFloat.Data4[:])

	got, err := parseWASAPIInputFormat(uintptrFromBytes(raw))
	if err != nil {
		t.Fatal(err)
	}
	if !got.isFloat {
		t.Fatal("isFloat = false, want true")
	}
	if got.samplesPerSec != 16000 || got.channels != 1 || got.bitsPerSample != 32 || got.validBits != 32 {
		t.Fatalf("unexpected parsed format: %#v", got)
	}
	if got.channelMask != 0x00000004 {
		t.Fatalf("channelMask = 0x%X, want 0x4", got.channelMask)
	}
}

func uintptrFromBytes(raw []byte) uintptr {
	return uintptr(unsafe.Pointer(&raw[0]))
}

func TestDefaultWASAPIAutoConvertFormatSpecsPrefers44100StereoFirst(t *testing.T) {
	got := defaultWASAPIAutoConvertFormatSpecs()
	want := []struct {
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
	if len(got) != len(want) {
		t.Fatalf("len(got) = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("spec[%d] = %#v, want %#v", i, got[i], want[i])
		}
	}
}

func TestWASAPIInitAttemptsRetriesZeroThenOneSecond(t *testing.T) {
	got := wasapiInitAttempts()
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0].duration != 0 {
		t.Fatalf("first duration = %d, want 0", got[0].duration)
	}
	if got[1].duration != 10_000_000 {
		t.Fatalf("second duration = %d, want 10000000", got[1].duration)
	}
}

func TestSelectWASAPISharedFormatKeepsRequestedWhenClosestIgnored(t *testing.T) {
	const requested = uintptr(0x1000)
	const closest = uintptr(0x2000)

	got, err := selectWASAPISharedFormat(requested, closest, hresultSFalse, false)
	if err != nil {
		t.Fatal(err)
	}
	if got.formatPtr != requested {
		t.Fatalf("formatPtr = 0x%X, want requested 0x%X", got.formatPtr, requested)
	}
	if got.needsFree {
		t.Fatal("needsFree = true, want false for requested stack format")
	}
	if got.ignoredClosestPtr != closest {
		t.Fatalf("ignoredClosestPtr = 0x%X, want 0x%X", got.ignoredClosestPtr, closest)
	}
}

func TestSelectWASAPISharedFormatUsesClosestWhenAllowed(t *testing.T) {
	const requested = uintptr(0x1000)
	const closest = uintptr(0x2000)

	got, err := selectWASAPISharedFormat(requested, closest, hresultSFalse, true)
	if err != nil {
		t.Fatal(err)
	}
	if got.formatPtr != closest {
		t.Fatalf("formatPtr = 0x%X, want closest 0x%X", got.formatPtr, closest)
	}
	if !got.needsFree {
		t.Fatal("needsFree = false, want true for COM closest-match format")
	}
	if got.ignoredClosestPtr != 0 {
		t.Fatalf("ignoredClosestPtr = 0x%X, want 0", got.ignoredClosestPtr)
	}
}

func TestSelectWASAPISharedFormatKeepsRequestedOnExactSupport(t *testing.T) {
	const requested = uintptr(0x1000)

	got, err := selectWASAPISharedFormat(requested, 0, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	if got.formatPtr != requested {
		t.Fatalf("formatPtr = 0x%X, want requested 0x%X", got.formatPtr, requested)
	}
	if got.needsFree || got.ignoredClosestPtr != 0 {
		t.Fatalf("unexpected selection metadata: %#v", got)
	}
}

func TestWASAPIErrorSuggestsEndpointReset(t *testing.T) {
	if !wasapiErrorSuggestsEndpointReset(staticErr("IAudioClient::Initialize: 0x80070057")) {
		t.Fatal("expected reset hint for E_INVALIDARG")
	}
	if wasapiErrorSuggestsEndpointReset(staticErr("IAudioClient::IsFormatSupported: 0x88890008")) {
		t.Fatal("did not expect reset hint for non-initialize failure")
	}
}

func TestPCMOutputCanUseRequestedFormatWhenClosestIgnored(t *testing.T) {
	requested := waveFormatEx{
		FormatTag:      waveFormatPCM,
		Channels:       2,
		SamplesPerSec:  44100,
		BitsPerSample:  16,
		BlockAlign:     4,
		AvgBytesPerSec: 176400,
	}
	closest := waveFormatEx{
		FormatTag:      waveFormatExtensibleTag,
		Channels:       1,
		SamplesPerSec:  48000,
		BitsPerSample:  32,
		BlockAlign:     4,
		AvgBytesPerSec: 192000,
	}

	selected, err := selectWASAPISharedFormat(
		uintptr(unsafe.Pointer(&requested)),
		uintptr(unsafe.Pointer(&closest)),
		hresultSFalse,
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	input, err := parseWASAPIInputFormat(selected.formatPtr)
	if err != nil {
		t.Fatal(err)
	}
	output := outputFormatFromInput(input)
	if output.SamplesPerSec != 44100 || output.Channels != 1 || output.BitsPerSample != 16 {
		t.Fatalf("output = %#v, want mono 44100Hz/16-bit from requested PCM", output)
	}
	if selected.ignoredClosestPtr != uintptr(unsafe.Pointer(&closest)) {
		t.Fatalf("closest was not marked ignored: %#v", selected)
	}
}

func TestOutputFormatFromInputUsesNegotiatedFormat(t *testing.T) {
	input := wasapiInputFormat{
		channels:      2,
		samplesPerSec: 44100,
		bitsPerSample: 16,
		validBits:     16,
	}
	got := outputFormatFromInput(input)
	if got.Channels != 1 {
		t.Fatalf("channels = %d, want 1", got.Channels)
	}
	if got.SamplesPerSec != 44100 {
		t.Fatalf("rate = %d, want 44100", got.SamplesPerSec)
	}
	if got.BitsPerSample != 16 {
		t.Fatalf("bits = %d, want 16", got.BitsPerSample)
	}
}

type staticErr string

func (e staticErr) Error() string { return string(e) }
