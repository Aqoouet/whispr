//go:build !windows

package app

func requireRuntimeDLLs(opts Options) bool {
	return opts.InputPath == ""
}
