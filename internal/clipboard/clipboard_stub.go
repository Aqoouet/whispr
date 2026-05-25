//go:build !windows

package clipboard

import "fmt"

func SetText(text string) error {
	return fmt.Errorf("clipboard not supported on this platform: %d bytes ready", len(text))
}
