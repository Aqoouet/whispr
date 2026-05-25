//go:build windows

package paste

import (
	"fmt"
	"syscall"
	"unsafe"
)

const (
	inputKeyboard  = 1
	keyeventfKeyUp = 0x0002
	vkControl      = 0x11
	vkV            = 0x56
)

type keyboardInput struct {
	Vk        uint16
	Scan      uint16
	Flags     uint32
	Time      uint32
	ExtraInfo uintptr
}

type input struct {
	Type uint32
	Ki   keyboardInput
}

var (
	user32        = syscall.NewLazyDLL("user32.dll")
	procSendInput = user32.NewProc("SendInput")
)

func CtrlV() error {
	seq := []input{
		{Type: inputKeyboard, Ki: keyboardInput{Vk: vkControl}},
		{Type: inputKeyboard, Ki: keyboardInput{Vk: vkV}},
		{Type: inputKeyboard, Ki: keyboardInput{Vk: vkV, Flags: keyeventfKeyUp}},
		{Type: inputKeyboard, Ki: keyboardInput{Vk: vkControl, Flags: keyeventfKeyUp}},
	}
	r1, _, err := procSendInput.Call(
		uintptr(len(seq)),
		uintptr(unsafe.Pointer(&seq[0])),
		unsafe.Sizeof(seq[0]),
	)
	if r1 != uintptr(len(seq)) {
		return fmt.Errorf("SendInput failed: %w", err)
	}
	return nil
}
