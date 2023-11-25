package main

import (
	"bytes"
	"github.com/cradio/NoodleBox/models"
	"github.com/gofiber/fiber/v2"
	"log"
	"strings"
)

func PatchMiddlewareHandler(c *fiber.Ctx) error {
	body, err := c.Response().BodyUncompressed()
	if err != nil {
		log.Println(err)
		return err
	}

	body = bytes.ReplaceAll(body, []byte("https://edu.vsu.ru/"), []byte("/"))
	body = bytes.ReplaceAll(body, []byte("edu.vsu.ru"), []byte(URL))
	if URL == "127.0.0.1:8000" {
		body = bytes.ReplaceAll(body, []byte("https"), []byte("http"))
	}

	uname := ""
	if c.Locals("uname") != nil {
		uname = c.Locals("uname").(string)
	}

	body = bytes.ReplaceAll(body, []byte("</head>"), []byte("<link rel=\"stylesheet\" href=\"/_api/styles/"+uname+"\" /></head>"))

	c.Response().Header.Del("Content-Encoding")
	if pk := string(c.Response().Header.Peek("Location")); strings.Contains(pk, "edu.vsu.ru") {
		log.Println("LOC", pk)
		c.Response().Header.Set("Location", strings.ReplaceAll(pk, "https://edu.vsu.ru/", "/"))
	}
	//if c.Path() == "/login/index.php" {
	//	c.Response().Header.Del("Location")
	//}
	c.Send(body)
	return c.Next()
}

func CustomStyles(c *fiber.Ctx) error {
	usr := models.User{}
	DB.Where(models.User{Username: c.Params("uname")}).First(&usr)
	c.Set("Content-Type", "text/css")
	return c.SendString(usr.CustomCSS)
}
