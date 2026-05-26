//go:build windows

package clipboard

import "github.com/atotto/clipboard"

func SetText(text string) error {
	return clipboard.WriteAll(text)
}
