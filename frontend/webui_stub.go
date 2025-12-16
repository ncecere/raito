//go:build !embedwebui

package webui

import "io/fs"

func Enabled() bool {
	return false
}

func FS() fs.FS {
	return nil
}
