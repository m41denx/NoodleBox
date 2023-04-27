package main

import (
	"embed"
	"github.com/glebarez/sqlite"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/proxy"
	"github.com/gofiber/template/html"
	"gorm.io/gorm"
	"log"
	"net/http"
)

var DB *gorm.DB

//go:embed assets/*
var Assets embed.FS

var Ignore = []string{"image/png", "image/jpeg"}

var URL = "127.0.0.1:8080"

// var URL = "vsu.noodlebox.ru"
var Origin = "https://edu.vsu.ru"

func main() {
	var err error
	DB, err = gorm.Open(sqlite.Open("amongus.db"), &gorm.Config{})
	if err != nil {
		log.Fatalln(err)
	}
	log.Println(DB.AutoMigrate(&User{}), DB.AutoMigrate(&Transaction{}))

	log.Println(Assets.ReadDir("assets"))
	engine := html.NewFileSystem(http.FS(Assets), ".html")

	app := fiber.New(fiber.Config{
		Views: engine,
	})

	authMiddleware := NewAuthMiddleware(Origin)
	app.Use(logger.New())
	app.Use(authMiddleware.HandlerBefore)
	app.Use(func(c *fiber.Ctx) error {
		// Inline middlewares
		handler := proxy.Forward(Origin + c.OriginalURL())
		handler(c)
		c.Next()
		return nil
	})
	app.Use(PatchMiddlewareHandler)
	app.Use(cors.New())
	app.Listen(":8080")
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

func contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}

	return false
}
