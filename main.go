package main

import (
	"bytes"
	"embed"
	_ "embed"
	"github.com/cradio/NoodleBox/old"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/template/html"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"io"
	"log"
	"net/http"
	"net/url"
)

var DB *gorm.DB

//go:embed assets/*
var Assets embed.FS

var Ignore = []string{"image/png", "image/jpeg"}

var URL = "vsu.noodlebox.ru"

func main() {
	DB, err := gorm.Open(sqlite.Open("RDB.sql"), &gorm.Config{})
	if err != nil {
		log.Fatalln(err)
	}
	DB.AutoMigrate(new(User))
	DB.AutoMigrate(new(Transaction))

	engine := html.NewFileSystem(http.FS(Assets), ".html")

	app := fiber.New(fiber.Config{
		Views: engine,
	})

	originServerURL, _ := url.Parse("https://edu.vsu.ru")

	_ = &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		r.Host = originServerURL.Host
		r.URL.Host = originServerURL.Host
		r.URL.Scheme = originServerURL.Scheme
		r.RequestURI = ""
		r.Header.Set("Referer", "https://edu.vsu.ru")
		r.Header.Set("Host", "edu.vsu.ru")
		r.Header.Set("Origin", "https://edu.vsu.ru")
		r.Header.Del("Accept-Encoding")

		if r.URL.Path == "/login/index.php" && r.Method == "POST" {
			old.AuthNoodle(w, r)
			return
		}

		log.Println(r.URL.String())
		origResp, err := http.DefaultClient.Do(r)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			io.WriteString(w, err.Error())
			return
		}
		w.Header().Set("Content-Type", origResp.Header.Get("Content-Type"))
		if origResp.Header.Get("Set-Cookie") != "" {
			for _, gg := range origResp.Header.Values("Set-Cookie") {
				w.Header().Add("Set-Cookie", gg)
			}
		}
		if contains(Ignore, origResp.Header.Get("Content-Type")) {
			log.Println("Stomped: " + r.URL.String())
			_, err = io.Copy(w, origResp.Body)
			log.Println(err)
			return
		}
		log.Println(w.Header())
		body, _ := io.ReadAll(origResp.Body)
		body = bytes.ReplaceAll(body, []byte("https://edu.vsu.ru/"), []byte("/"))
		body = bytes.ReplaceAll(body, []byte("edu.vsu.ru"), []byte(URL))
		//body = bytes.ReplaceAll(body, []byte("https"), []byte("http"))

		w.WriteHeader(origResp.StatusCode)

		if bytes.Contains(body, []byte("usertext mr-1")) {
			bodyX := bytes.SplitN(body, []byte("usertext mr-1\">"), 2)
			bodyY := bytes.SplitN(bodyX[1], []byte("<"), 2)
			w.Write(bodyX[0])
			io.WriteString(w, "usertext mr-1\">noodle<")
			w.Write(bodyY[1])
		} else {
			w.Write(body)
		}
	})

	http.ListenAndServe("0.0.0.0:8080", nil)
}

func contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}

	return false
}
