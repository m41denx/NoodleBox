package main

import (
	"embed"
	"fmt"
	"github.com/cradio/NoodleBox/middlewares"
	"github.com/cradio/NoodleBox/models"
	"github.com/cradio/NoodleBox/utils"
	"github.com/glebarez/sqlite"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cache"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/proxy"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/template/html"
	"gorm.io/gorm"
	glogger "gorm.io/gorm/logger"
	"log"
	"net/http"
	"os"
	"runtime/debug"
	"time"
)

var DB *gorm.DB

//go:embed assets/*
var Assets embed.FS

var Ignore = []string{"image/png", "image/jpeg"}

var URL = utils.GetEnv("URL", "127.0.0.1:8000")

// var URL = "vsu.noodlebox.ru"
var Origin = "https://edu.vsu.ru"

func main() {
	InitDB()

	log.Println(Assets.ReadDir("assets"))
	engine := html.NewFileSystem(http.FS(Assets), ".html")

	app := fiber.New(fiber.Config{
		Views: engine,
	})

	mw := NewMiddlewareProvider(app)
	app.Use(logger.New())
	app.Use(cors.New())
	app.Use(recover.New(recover.Config{
		StackTraceHandler: func(c *fiber.Ctx, e interface{}) {
			log.Println("\n\n\n\n", string(debug.Stack()))
		},
		EnableStackTrace: true,
	}))

	app.Use(func(c *fiber.Ctx) error {
		// Inject metrics
		metrics := utils.NewGoMetrics()
		c.Locals("metrics", metrics)
		return c.Next()
	})

	// Static cache
	cacher := cache.New(cache.Config{
		Expiration:   30 * time.Minute,
		CacheHeader:  "Noodle-Cache",
		CacheControl: false,
		Storage:      NewCacheStorage(),
		//StoreResponseHeaders: false,
		MaxBytes: 1024 * 1024 * 1024 * 4, // 4GB
	})
	app.Group("/theme/").
		Use(func(c *fiber.Ctx) error {
			c.Locals("metrics").(*utils.GoMetrics).NewStep("Cache Fetch")
			return c.Next()
		}).
		Use(cacher).
		Use(func(c *fiber.Ctx) error {
			c.Locals("metrics").(*utils.GoMetrics).NewStep("Later")
			return c.Next()
		})
	app.Group("/npm/").
		Use(func(c *fiber.Ctx) error {
			c.Locals("metrics").(*utils.GoMetrics).NewStep("Cache Fetch")
			return c.Next()
		}).
		Use(cacher).
		Use(func(c *fiber.Ctx) error {
			c.Locals("metrics").(*utils.GoMetrics).NewStep("Later")
			return c.Next()
		})

	// region API
	api := app.Group("/_api")
	api.Get("/styles/:uname", CustomStyles) // There is passthrough leak to https://edu.vsu.ru/_api/styles/

	// endregion

	app.All("/blocks/accessibility/userstyles.php", func(ctx *fiber.Ctx) error {
		return nil
	})

	// region Middlewares
	authMiddleware := middlewares.NewAuthMiddleware(Origin, DB)
	mw.RegisterMiddleware(authMiddleware)

	mw.SetTarget(func(c *fiber.Ctx) error {
		handler := proxy.Forward(Origin + c.OriginalURL())
		fmt.Println("=>", Origin+c.OriginalURL())
		if err := handler(c); err != nil {
			log.Println(err)
		}
		fmt.Println("=>", c.Response().StatusCode(), string(c.Response().Header.Peek("Location")))
		c.Next()
		return nil
	})
	// endregion

	mw.RegisterRoutes()

	//app.Use(authMiddleware.HandlerBefore)
	//app.Use(func(c *fiber.Ctx) error {
	//	// Inline middlewares
	//	if strings.HasPrefix(c.Path(), "/_api/") {
	//		return c.Next()
	//	}
	//	handler := proxy.Forward(Origin + c.OriginalURL())
	//	handler(c)
	//	c.Next()
	//	return nil
	//})
	app.Use(PatchMiddlewareHandler)

	app.Use(func(c *fiber.Ctx) error {
		m := c.Locals("metrics").(*utils.GoMetrics)
		m.Done()
		c.Set("X-Trace", m.DumpTextInline())
		fmt.Println(m.DumpText())
		return nil // Make sure nothing comes next
	})

	app.Listen(":8000")
	//
	//originServerURL, _ := url.Parse("https://edu.vsu.ru")
	//
	//_ = &http.Client{
	//	CheckRedirect: func(req *http.Request, via []*http.Request) error {
	//		return http.ErrUseLastResponse
	//	},
	//}
	//
	//http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
	//	r.Host = originServerURL.Host
	//	r.URL.Host = originServerURL.Host
	//	r.URL.Scheme = originServerURL.Scheme
	//	r.RequestURI = ""
	//	r.Header.Set("Referer", "https://edu.vsu.ru")
	//	r.Header.Set("Host", "edu.vsu.ru")
	//	r.Header.Set("Origin", "https://edu.vsu.ru")
	//	r.Header.Del("Accept-Encoding")
	//
	//	if r.URL.Path == "/login/index.php" && r.Method == "POST" {
	//		old.AuthNoodle(w, r)
	//		return
	//	}
	//
	//	log.Println(r.URL.String())
	//	origResp, err := http.DefaultClient.Do(r)
	//	if err != nil {
	//		w.WriteHeader(http.StatusInternalServerError)
	//		io.WriteString(w, err.Error())
	//		return
	//	}
	//	w.Header().Set("Content-Type", origResp.Header.Get("Content-Type"))
	//	if origResp.Header.Get("Set-Cookie") != "" {
	//		for _, gg := range origResp.Header.Values("Set-Cookie") {
	//			w.Header().Add("Set-Cookie", gg)
	//		}
	//	}
	//	if contains(Ignore, origResp.Header.Get("Content-Type")) {
	//		log.Println("Stomped: " + r.URL.String())
	//		_, err = io.Copy(w, origResp.Body)
	//		log.Println(err)
	//		return
	//	}
	//	log.Println(w.Header())
	//	body, _ := io.ReadAll(origResp.Body)
	//	body = bytes.ReplaceAll(body, []byte("https://edu.vsu.ru/"), []byte("/"))
	//	body = bytes.ReplaceAll(body, []byte("edu.vsu.ru"), []byte(URL))
	//	//body = bytes.ReplaceAll(body, []byte("https"), []byte("http"))
	//
	//	w.WriteHeader(origResp.StatusCode)
	//
	//	if bytes.Contains(body, []byte("usertext mr-1")) {
	//		bodyX := bytes.SplitN(body, []byte("usertext mr-1\">"), 2)
	//		bodyY := bytes.SplitN(bodyX[1], []byte("<"), 2)
	//		w.Write(bodyX[0])
	//		io.WriteString(w, "usertext mr-1\">noodle<")
	//		w.Write(bodyY[1])
	//	} else {
	//		w.Write(body)
	//	}
	//})
	//
	//http.ListenAndServe("0.0.0.0:8080", nil)
}

func InitDB() {
	var err error
	newLogger := glogger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags), // io writer
		glogger.Config{
			LogLevel: glogger.Info, // Log level
		},
	)
	DB, err = gorm.Open(sqlite.Open("amongus.db"), &gorm.Config{
		Logger: newLogger,
	})
	if err != nil {
		log.Fatalln(err)
	}
	log.Println(
		DB.AutoMigrate(&models.User{}),
		DB.AutoMigrate(&models.Transaction{}),
	)
}
