//go:build windows

package tray

import (
	"bytes"
	"encoding/binary"
	"runtime"
	"sync"

	"github.com/getlantern/systray"
)

type UI struct {
	exitCh  chan struct{}
	readyCh chan struct{}
	quit    sync.Once
}

func New() (*UI, error) {
	u := &UI{
		exitCh:  make(chan struct{}),
		readyCh: make(chan struct{}),
	}
	go func() {
		runtime.LockOSThread()
		systray.Run(u.onReady, u.onExit)
	}()
	<-u.readyCh
	return u, nil
}

func (u *UI) onReady() {
	systray.SetIcon(buildIcon())
	systray.SetTooltip("CorpDictation: Idle")
	exit := systray.AddMenuItem("Exit", "Exit CorpDictation")
	go func() {
		<-exit.ClickedCh
		u.Close()
	}()
	close(u.readyCh)
}

func (u *UI) onExit() {
	close(u.exitCh)
}

func (u *UI) SetStatus(status string) {
	systray.SetTooltip("CorpDictation: " + status)
}

func (u *UI) Notify(_, _ string) {}

func (u *UI) WaitForExit() <-chan struct{} {
	return u.exitCh
}

func (u *UI) Close() error {
	u.quit.Do(systray.Quit)
	return nil
}

func buildIcon() []byte {
	buf := &bytes.Buffer{}
	binary.Write(buf, binary.LittleEndian, uint16(0)) // reserved
	binary.Write(buf, binary.LittleEndian, uint16(1)) // type=icon
	binary.Write(buf, binary.LittleEndian, uint16(1)) // count=1
	const (
		w, h      = 16, 16
		pixBytes  = w * h * 4
		maskBytes = h * 4
		imageSize = 40 + pixBytes + maskBytes
	)
	buf.WriteByte(w)
	buf.WriteByte(h)
	buf.WriteByte(0) // colorCount
	buf.WriteByte(0) // reserved
	binary.Write(buf, binary.LittleEndian, uint16(1))         // planes
	binary.Write(buf, binary.LittleEndian, uint16(32))        // bitCount
	binary.Write(buf, binary.LittleEndian, uint32(imageSize)) // size
	binary.Write(buf, binary.LittleEndian, uint32(6+16))      // offset
	binary.Write(buf, binary.LittleEndian, uint32(40))        // biSize
	binary.Write(buf, binary.LittleEndian, int32(w))          // biWidth
	binary.Write(buf, binary.LittleEndian, int32(h*2))        // biHeight (doubled)
	binary.Write(buf, binary.LittleEndian, uint16(1))         // biPlanes
	binary.Write(buf, binary.LittleEndian, uint16(32))        // biBitCount
	binary.Write(buf, binary.LittleEndian, uint32(0))         // biCompression
	binary.Write(buf, binary.LittleEndian, uint32(0))         // biSizeImage
	binary.Write(buf, binary.LittleEndian, int32(0))          // biXPels
	binary.Write(buf, binary.LittleEndian, int32(0))          // biYPels
	binary.Write(buf, binary.LittleEndian, uint32(0))         // biClrUsed
	binary.Write(buf, binary.LittleEndian, uint32(0))         // biClrImportant
	pixel := []byte{0xFF, 0x7B, 0x00, 0xFF}                  // BGRA: solid blue
	for i := 0; i < w*h; i++ {
		buf.Write(pixel)
	}
	buf.Write(make([]byte, maskBytes)) // AND mask: fully opaque
	return buf.Bytes()
}
