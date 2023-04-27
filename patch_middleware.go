package main

import (
	"bytes"
	"github.com/gofiber/fiber/v2"
	"log"
)

func PatchMiddlewareHandler(c *fiber.Ctx) error {
	body, err := c.Response().BodyUncompressed()
	if err != nil {
		log.Println(err)
		return err
	}

	body = bytes.ReplaceAll(body, []byte("https://edu.vsu.ru/"), []byte("/"))
	body = bytes.ReplaceAll(body, []byte("edu.vsu.ru"), []byte(URL))
	body = bytes.ReplaceAll(body, []byte("https"), []byte("http"))

	c.Response().Header.Del("Content-Encoding")
	c.Send(body)
	return c.Next()
}
