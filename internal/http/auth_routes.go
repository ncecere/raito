package http

import "github.com/gofiber/fiber/v2"

func registerAuthRoutes(app *fiber.App) {
	app.Post("/auth/login", loginHandler)
	app.Post("/auth/logout", logoutHandler)
	app.Get("/auth/oidc/login", oidcLoginStartHandler)
	app.Get("/auth/oidc/callback", oidcCallbackHandler)
}
