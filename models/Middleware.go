package models

import "github.com/gofiber/fiber/v2"

type Middleware interface {
	GetIngressHooks() []*RouteHook
	GetEgressHooks() []*RouteHook
}

type RouteHook struct {
	Route      string
	Method     string
	SkipOrigin bool
	Handler    func(c *fiber.Ctx, body []byte) error
}
