//go:build windows

package clipboard

import (
	"fmt"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

const (
	cfUnicodeText = 13
	gmemMoveable  = 0x0002
)

var (
	user32             = syscall.NewLazyDLL("user32.dll")
	kernel32           = syscall.NewLazyDLL("kernel32.dll")
	procOpenClipboard  = user32.NewProc("OpenClipboard")
	procCloseClipboard = user32.NewProc("CloseClipboard")
	procEmptyClipboard = user32.NewProc("EmptyClipboard")
	procSetClipboard   = user32.NewProc("SetClipboardData")
	procGlobalAlloc    = kernel32.NewProc("GlobalAlloc")
	procGlobalLock     = kernel32.NewProc("GlobalLock")
	procGlobalUnlock   = kernel32.NewProc("GlobalUnlock")
)

func SetText(text string) error {
	r1, _, err := procOpenClipboard.Call(0)
	if r1 == 0 {
		return fmt.Errorf("OpenClipboard failed: %w", err)
	}
	defer procCloseClipboard.Call()

	if r1, _, err = procEmptyClipboard.Call(); r1 == 0 {
		return fmt.Errorf("EmptyClipboard failed: %w", err)
	}

	utf := utf16.Encode([]rune(text + "\x00"))
	size := uintptr(len(utf) * 2)
	handle, _, err := procGlobalAlloc.Call(gmemMoveable, size)
	if handle == 0 {
		return fmt.Errorf("GlobalAlloc failed: %w", err)
	}
	ptr, _, err := procGlobalLock.Call(handle)
	if ptr == 0 {
		return fmt.Errorf("GlobalLock failed: %w", err)
	}
	copy(unsafe.Slice((*uint16)(unsafe.Pointer(ptr)), len(utf)), utf)
	procGlobalUnlock.Call(handle)

	r1, _, err = procSetClipboard.Call(cfUnicodeText, handle)
	if r1 == 0 {
		return fmt.Errorf("SetClipboardData failed: %w", err)
	}
	return nil
}
