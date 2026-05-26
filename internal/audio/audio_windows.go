//go:build windows

package audio

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/go-ole/go-ole"
)

type Recorder struct {
	options Options
	active  bool
	stopCh  chan struct{}
	doneCh  chan struct{}
	samples []byte
}

func NewRecorder(o Options) (*Recorder, error) {
	return &Recorder{options: o}, nil
}

func EnumerateDevices(_ Options) ([]DeviceInfo, error) {
	return []DeviceInfo{{ID: 0, Name: "Default"}}, nil
}

func (r *Recorder) Start() error {
	if r.active {
		return fmt.Errorf("recording already active")
	}
	r.stopCh = make(chan struct{})
	r.doneCh = make(chan struct{})
	startedCh := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
			if oleErr, ok := err.(*ole.OleError); !ok || oleErr.Code() != 1 {
				startedCh <- fmt.Errorf("CoInitializeEx: %w", err)
				return
			}
		}
		defer ole.CoUninitialize()
		sess, err := openWASAPI()
		if err != nil {
			startedCh <- err
			return
		}
		defer sess.release()
		if err := sess.audioClient.Start(); err != nil {
			startedCh <- fmt.Errorf("audio client start: %w", err)
			return
		}
		startedCh <- nil
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		var buf []byte
		for {
			select {
			case <-r.stopCh:
				sess.audioClient.Stop()
				r.samples = buf
				close(r.doneCh)
				return
			case <-ticker.C:
				drainPackets(sess.captureClient, &buf)
			}
		}
	}()
	if err := <-startedCh; err != nil {
		return err
	}
	r.active = true
	return nil
}

func (r *Recorder) Stop() (string, error) {
	if !r.active {
		return "", fmt.Errorf("recording is not active")
	}
	close(r.stopCh)
	<-r.doneCh
	r.active = false
	f, err := os.CreateTemp("", "whispr-*.wav")
	if err != nil {
		return "", fmt.Errorf("create temp wav: %w", err)
	}
	defer f.Close()
	if err := writeWAV(f, r.samples); err != nil {
		os.Remove(f.Name())
		return "", fmt.Errorf("write wav: %w", err)
	}
	return f.Name(), nil
}

func (r *Recorder) Close() error {
	if r.active {
		_, _ = r.Stop()
	}
	return nil
}

func EnsureWAVPath(path string) (string, error) {
	return path, nil
}

func (r *Recorder) ActiveBackendDescription() string {
	return "backend=wasapi"
}

func writeWAV(f *os.File, samples []byte) error {
	const (
		channels    = 1
		sampleRate  = 16000
		bitsPerSamp = 16
		blockAlign  = channels * bitsPerSamp / 8
		byteRate    = sampleRate * blockAlign
	)
	dataLen := uint32(len(samples))
	w := &bytes.Buffer{}
	w.WriteString("RIFF")
	binary.Write(w, binary.LittleEndian, uint32(36+dataLen))
	w.WriteString("WAVE")
	w.WriteString("fmt ")
	binary.Write(w, binary.LittleEndian, uint32(16))
	binary.Write(w, binary.LittleEndian, uint16(1))
	binary.Write(w, binary.LittleEndian, uint16(channels))
	binary.Write(w, binary.LittleEndian, uint32(sampleRate))
	binary.Write(w, binary.LittleEndian, uint32(byteRate))
	binary.Write(w, binary.LittleEndian, uint16(blockAlign))
	binary.Write(w, binary.LittleEndian, uint16(bitsPerSamp))
	w.WriteString("data")
	binary.Write(w, binary.LittleEndian, dataLen)
	if _, err := f.Write(w.Bytes()); err != nil {
		return err
	}
	_, err := f.Write(samples)
	return err
}
