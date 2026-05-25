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
