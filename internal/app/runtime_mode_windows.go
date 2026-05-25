//go:build windows

package app

func requireRuntimeDLLs(Options) bool {
	return true
}
