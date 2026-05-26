//go:build windows

package hotkey

import "golang.design/x/hotkey"

type Listener struct {
	hk     *hotkey.Hotkey
	events chan struct{}
}

func Register(_ string) (*Listener, error) {
	hk := hotkey.New([]hotkey.Modifier{hotkey.ModCtrl, hotkey.ModAlt}, hotkey.KeySpace)
	if err := hk.Register(); err != nil {
		return nil, err
	}
	l := &Listener{hk: hk, events: make(chan struct{}, 4)}
	go func() {
		for range hk.Keydown() {
			select {
			case l.events <- struct{}{}:
			default:
			}
		}
	}()
	return l, nil
}

func (l *Listener) Events() <-chan struct{} {
	return l.events
}

func (l *Listener) Close() error {
	return l.hk.Unregister()
}
