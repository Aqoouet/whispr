//go:build windows

package whisper

import (
	"fmt"
	"syscall"
	"unsafe"
)

type backend struct{}

func New() Backend {
	return backend{}
}

func (backend) CUDAAvailable(runtimeDir string) (bool, error) {
	dll, err := loadDLL(runtimeDir)
	if err != nil {
		return false, err
	}
	proc := dll.NewProc("cd_cuda_available")
	r1, _, callErr := proc.Call()
	if r1 == 0 && callErr != syscall.Errno(0) {
		return false, fmt.Errorf("cd_cuda_available failed: %w", callErr)
	}
	return r1 != 0, nil
}

func (backend) Transcribe(req Request) (string, error) {
	dll, err := loadDLL(req.RuntimeDir)
	if err != nil {
		return "", err
	}

	if err := callInit(dll, req); err != nil {
		return "", err
	}
	defer callShutdown(dll)

	buf := make([]byte, 64*1024)
	proc := dll.NewProc("cd_transcribe_wav")
	inputPtr, err := syscall.UTF16PtrFromString(req.InputWAV)
	if err != nil {
		return "", err
	}
	r1, _, callErr := proc.Call(
		uintptr(unsafe.Pointer(inputPtr)),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
	)
	if r1 == 0 {
		return "", lastError(dll, callErr)
	}
	n := 0
	for n < len(buf) && buf[n] != 0 {
		n++
	}
	return string(buf[:n]), nil
}

func loadDLL(runtimeDir string) (*syscall.LazyDLL, error) {
	if err := setRuntimeDLLDirectory(runtimeDir); err != nil {
		return nil, err
	}
	path := runtimeDir + `\corpdictation_whisper.dll`
	dll := syscall.NewLazyDLL(path)
	if err := dll.Load(); err != nil {
		return nil, fmt.Errorf("load %s: %w", path, err)
	}
	return dll, nil
}

func callInit(dll *syscall.LazyDLL, req Request) error {
	proc := dll.NewProc("cd_init")
	runtimePtr, err := syscall.UTF16PtrFromString(req.RuntimeDir)
	if err != nil {
		return err
	}
	modelPtr, err := syscall.UTF16PtrFromString(req.ModelPath)
	if err != nil {
		return err
	}
	devicePtr, err := syscall.UTF16PtrFromString(req.Device)
	if err != nil {
		return err
	}
	langPtr, err := syscall.UTF16PtrFromString(req.Language)
	if err != nil {
		return err
	}
	r1, _, callErr := proc.Call(
		uintptr(unsafe.Pointer(runtimePtr)),
		uintptr(unsafe.Pointer(modelPtr)),
		uintptr(unsafe.Pointer(devicePtr)),
		uintptr(unsafe.Pointer(langPtr)),
		uintptr(req.BeamSize),
	)
	if r1 == 0 {
		return lastError(dll, callErr)
	}
	return nil
}

func callShutdown(dll *syscall.LazyDLL) {
	_, _, _ = dll.NewProc("cd_shutdown").Call()
}

func lastError(dll *syscall.LazyDLL, callErr error) error {
	buf := make([]byte, 4096)
	r1, _, _ := dll.NewProc("cd_last_error").Call(
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
	)
	if r1 != 0 {
		n := 0
		for n < len(buf) && buf[n] != 0 {
			n++
		}
		if n > 0 {
			return fmt.Errorf("%s", string(buf[:n]))
		}
	}
	if callErr != syscall.Errno(0) {
		return callErr
	}
	return fmt.Errorf("whisper backend call failed")
}

func setRuntimeDLLDirectory(runtimeDir string) error {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	proc := kernel32.NewProc("SetDllDirectoryW")
	ptr, err := syscall.UTF16PtrFromString(runtimeDir)
	if err != nil {
		return err
	}
	r1, _, callErr := proc.Call(uintptr(unsafe.Pointer(ptr)))
	if r1 == 0 {
		return fmt.Errorf("SetDllDirectoryW failed: %w", callErr)
	}
	return nil
}
