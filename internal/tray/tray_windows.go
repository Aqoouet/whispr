//go:build windows

package tray

import (
	"fmt"
	"syscall"
	"unsafe"
)

const (
	wsOverlappedWindow = 0x00CF0000
	swShow             = 5
	wmDestroy          = 0x0002
	wmCommand          = 0x0111
	cwUseDefault       = 0x80000000
)

var (
	user32               = syscall.NewLazyDLL("user32.dll")
	kernel32             = syscall.NewLazyDLL("kernel32.dll")
	procDefWindowProc    = user32.NewProc("DefWindowProcW")
	procRegisterClassEx  = user32.NewProc("RegisterClassExW")
	procCreateWindowEx   = user32.NewProc("CreateWindowExW")
	procShowWindow       = user32.NewProc("ShowWindow")
	procUpdateWindow     = user32.NewProc("UpdateWindow")
	procGetMessage       = user32.NewProc("GetMessageW")
	procDispatchMessage  = user32.NewProc("DispatchMessageW")
	procTranslateMessage = user32.NewProc("TranslateMessage")
	procPostQuitMessage  = user32.NewProc("PostQuitMessage")
	procSetWindowText    = user32.NewProc("SetWindowTextW")
	procCreateControl    = user32.NewProc("CreateWindowExW")
	procGetModuleHandle  = kernel32.NewProc("GetModuleHandleW")
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

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}

type UI struct {
	statusCh chan string
	exitCh   chan struct{}
}

func New() (*UI, error) {
	ui := &UI{
		statusCh: make(chan string, 8),
		exitCh:   make(chan struct{}),
	}
	go ui.loop()
	return ui, nil
}

func (u *UI) SetStatus(status string) {
	select {
	case u.statusCh <- status:
	default:
	}
}

func (u *UI) Notify(title string, body string) {
	_ = title
	_ = body
}

func (u *UI) WaitForExit() <-chan struct{} {
	return u.exitCh
}

func (u *UI) Close() error {
	return nil
}

func (u *UI) loop() {
	className, _ := syscall.UTF16PtrFromString("CorpDictationWindow")
	title, _ := syscall.UTF16PtrFromString("CorpDictation")
	hinst, _, _ := procGetModuleHandle.Call(0)
	wc := wndClassEx{
		Size:      uint32(unsafe.Sizeof(wndClassEx{})),
		WndProc:   syscall.NewCallback(windowProc),
		Instance:  hinst,
		ClassName: className,
	}
	if r1, _, err := procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc))); r1 == 0 {
		panic(fmt.Errorf("RegisterClassExW failed: %w", err))
	}
	hwnd, _, err := procCreateWindowEx.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(title)),
		wsOverlappedWindow,
		cwUseDefault, cwUseDefault, 340, 140,
		0, 0, hinst, 0,
	)
	if hwnd == 0 {
		panic(fmt.Errorf("CreateWindowExW failed: %w", err))
	}
	procCreateControl.Call(
		0,
		uintptr(unsafe.Pointer(mustUTF16("STATIC"))),
		uintptr(unsafe.Pointer(mustUTF16("Status: Idle"))),
		0x50000000,
		20, 20, 280, 24,
		hwnd, 0, hinst, 0,
	)
	procCreateControl.Call(
		0,
		uintptr(unsafe.Pointer(mustUTF16("BUTTON"))),
		uintptr(unsafe.Pointer(mustUTF16("Exit"))),
		0x50010000,
		20, 60, 80, 24,
		hwnd, 1, hinst, 0,
	)
	procShowWindow.Call(hwnd, swShow)
	procUpdateWindow.Call(hwnd)

	go func() {
		for status := range u.statusCh {
			procSetWindowText.Call(hwnd, uintptr(unsafe.Pointer(mustUTF16("CorpDictation - "+status))))
		}
	}()

	var m msg
	for {
		r1, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r1) <= 0 {
			close(u.exitCh)
			return
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func windowProc(hwnd uintptr, msg uint32, wParam uintptr, lParam uintptr) uintptr {
	switch msg {
	case wmCommand:
		if wParam == 1 {
			procPostQuitMessage.Call(0)
			return 0
		}
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	r1, _, _ := procDefWindowProc.Call(hwnd, uintptr(msg), wParam, lParam)
	return r1
}

func mustUTF16(v string) *uint16 {
	p, err := syscall.UTF16PtrFromString(v)
	if err != nil {
		panic(err)
	}
	return p
}
