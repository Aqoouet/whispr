//go:build !windows

package paste

func CtrlV() error {
	return nil
}
