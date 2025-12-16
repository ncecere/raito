//go:build embedwebui

package webui

import (
	"embed"
	"io/fs"
)

// content holds the built Vite assets under dist/.
//
// NOTE: This requires running `bun run build` in raito/frontend before compiling
// with `-tags embedwebui` (the Dockerfile handles this automatically).
//
//go:embed dist/* dist/assets/*
var content embed.FS

func Enabled() bool {
	return true
}

func FS() fs.FS {
	return content
}
