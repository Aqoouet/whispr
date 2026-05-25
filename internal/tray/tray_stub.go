//go:build !windows

package tray

import "fmt"

type UI struct{}

func New() (*UI, error) {
	return &UI{}, nil
}

func (u *UI) SetStatus(status string) {
	fmt.Println("status:", status)
}

func (u *UI) Notify(title string, body string) {
	fmt.Printf("%s: %s\n", title, body)
}

func (u *UI) WaitForExit() <-chan struct{} {
	ch := make(chan struct{})
	return ch
}

func (u *UI) Close() error {
	return nil
}
