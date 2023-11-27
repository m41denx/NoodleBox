package main

import (
	"fmt"
	"github.com/cradio/NoodleBox/models"
	"github.com/cradio/NoodleBox/utils"
	"github.com/gofiber/fiber/v2"
	"regexp"
)

type MiddlewareProvider struct {
	app     *fiber.App
	ingress []*models.RouteHook
	egress  []*models.RouteHook
	target  fiber.Handler
}

func NewMiddlewareProvider(app *fiber.App) *MiddlewareProvider {
	return &MiddlewareProvider{
		app:     app,
		ingress: make([]*models.RouteHook, 0),
		egress:  make([]*models.RouteHook, 0),
	}
}

func (m *MiddlewareProvider) RegisterMiddleware(mw models.Middleware) {
	m.ingress = append(m.ingress, mw.GetIngressHooks()...)
	m.egress = append(m.egress, mw.GetEgressHooks()...)
}

func (m *MiddlewareProvider) SetTarget(target fiber.Handler) {
	m.target = target
}

func (m *MiddlewareProvider) RegisterRoutes() {
	for _, hook := range m.ingress {
		m.app.Use(wrap(hook))
	}
	m.app.Use(m.target)
	fmt.Println(m.target)
	for _, hook := range m.egress {
		m.app.Use(wrap(hook))
	}
}

func wrap(hook *models.RouteHook) fiber.Handler {
	fmt.Println("Wrapped", hook.Route, hook.Method)
	return func(c *fiber.Ctx) error {
		c.Locals("metrics").(*utils.GoMetrics).NewStep("RegexRouting")
		if b, _ := regexp.MatchString(
			fmt.Sprintf("^%s$", hook.Route),
			c.Path(),
		); !b {
			return c.Next()
		}
		if c.Method() != hook.Method && hook.Method != "*" {
			return c.Next()
		}

		//fmt.Println("Running", hook.Route, hook.Method, hook.SkipOrigin)

		r := hook.Handler(c, nil)
		if hook.SkipOrigin {
			return r
		}
		return c.Next()
	}
}
