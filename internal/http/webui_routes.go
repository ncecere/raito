package http

import (
	"io/fs"
	"mime"
	"path"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v2"

	webui "raito/frontend"
)

func registerWebUIRoutes(app *fiber.App) {
	if !webui.Enabled() {
		return
	}

	distFS, err := fs.Sub(webui.FS(), "dist")
	if err != nil {
		return
	}

	indexHTML, err := fs.ReadFile(distFS, "index.html")
	if err != nil {
		return
	}

	serveIndex := func(c *fiber.Ctx) error {
		c.Set("Cache-Control", "no-cache")
		c.Type("html", "utf-8")
		return c.Send(indexHTML)
	}

	app.Get("/", serveIndex)

	app.Get("/*", func(c *fiber.Ctx) error {
		requestPath := c.Path()

		// Don't hijack API routes; let Fiber return a proper 404 for unknown endpoints.
		switch {
		case strings.HasPrefix(requestPath, "/v1/"),
			strings.HasPrefix(requestPath, "/admin/"),
			strings.HasPrefix(requestPath, "/auth/"),
			requestPath == "/healthz",
			requestPath == "/metrics":
			return fiber.ErrNotFound
		}

		cleaned := path.Clean(requestPath)
		cleaned = strings.TrimPrefix(cleaned, "/")

		if cleaned == "" || cleaned == "." {
			return serveIndex(c)
		}

		// If the request looks like it targets a file (has an extension),
		// serve it directly from dist/. Otherwise, fall back to index.html
		// for SPA routing.
		ext := filepath.Ext(cleaned)
		if ext == "" {
			return serveIndex(c)
		}

		payload, err := fs.ReadFile(distFS, cleaned)
		if err != nil {
			return fiber.ErrNotFound
		}

		// Vite outputs fingerprinted assets; cache those aggressively.
		if strings.HasPrefix(cleaned, "assets/") {
			c.Set("Cache-Control", "public, max-age=31536000, immutable")
		}

		if ct := mime.TypeByExtension(ext); ct != "" {
			c.Set("Content-Type", ct)
		} else {
			c.Type(ext)
		}

		return c.Send(payload)
	})
}
