//go:build !windows

package hotkey

import "fmt"

type Listener struct{}

func Register(string) (*Listener, error) {
	return nil, fmt.Errorf("global hotkeys are only supported on Windows")
}

func (l *Listener) Events() <-chan struct{} {
	return nil
}

func (l *Listener) Close() error {
	return nil
}
