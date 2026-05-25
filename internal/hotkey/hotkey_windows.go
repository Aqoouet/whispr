//go:build windows

package hotkey

import (
	"fmt"
	"strings"
	"syscall"
	"unsafe"
)

const (
	modAlt     = 0x0001
	modControl = 0x0002
	wmHotKey   = 0x0312
	wmQuit     = 0x0012
	vkSpace    = 0x20
)

var (
	user32             = syscall.NewLazyDLL("user32.dll")
	kernel32           = syscall.NewLazyDLL("kernel32.dll")
	procRegisterHotKey = user32.NewProc("RegisterHotKey")
	procUnregister     = user32.NewProc("UnregisterHotKey")
	procGetMessage     = user32.NewProc("GetMessageW")
	procPostThreadMsg  = user32.NewProc("PostThreadMessageW")
	procGetCurrentTID  = kernel32.NewProc("GetCurrentThreadId")
)

type point struct {
	X int32
	Y int32
}

type msg struct {
	HWnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
}

type Listener struct {
	events   chan struct{}
	done     chan struct{}
	threadID uint32
}

func Register(spec string) (*Listener, error) {
	mods, key, err := parse(spec)
	if err != nil {
		return nil, err
	}
	l := &Listener{
		events: make(chan struct{}, 4),
		done:   make(chan struct{}),
	}
	started := make(chan error, 1)
	go l.loop(mods, key, started)
	if err := <-started; err != nil {
		return nil, err
	}
	return l, nil
}

func (l *Listener) Events() <-chan struct{} {
	return l.events
}

func (l *Listener) Close() error {
	if l.threadID != 0 {
		_, _, _ = procPostThreadMsg.Call(uintptr(l.threadID), wmQuit, 0, 0)
	}
	<-l.done
	return nil
}

func (l *Listener) loop(mods uint32, key uint32, started chan<- error) {
	defer close(l.done)
	tid, _, _ := procGetCurrentTID.Call()
	l.threadID = uint32(tid)

	r1, _, err := procRegisterHotKey.Call(0, 1, uintptr(mods), uintptr(key))
	if r1 == 0 {
		started <- fmt.Errorf("RegisterHotKey failed: %w", err)
		return
	}
	started <- nil
	defer procUnregister.Call(0, 1)

	var m msg
	for {
		r1, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r1) <= 0 {
			return
		}
		switch m.Message {
		case wmHotKey:
			select {
			case l.events <- struct{}{}:
			default:
			}
		case wmQuit:
			return
		}
	}
}

func parse(spec string) (uint32, uint32, error) {
	spec = strings.ToLower(strings.TrimSpace(spec))
	if spec != "ctrl+alt+space" {
		return 0, 0, fmt.Errorf("unsupported hotkey %q, only Ctrl+Alt+Space is implemented", spec)
	}
	return modControl | modAlt, vkSpace, nil
}
