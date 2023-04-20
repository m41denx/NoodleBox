package old

import (
	_ "embed"
	"github.com/cradio/NoodleBox"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

//go:embed assets/AuthConfirmed.html
var AuthConfirmHTML string

var NoodleSessions url.Values

func AuthNoodle(w http.ResponseWriter, r *http.Request) {
	uclient := http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	// get MoodleSession and logintoken
	initReq, err := http.NewRequest("GET", "https://edu.vsu.ru/login/index.php", nil)
	resp, err := uclient.Do(initReq)
	if err != nil {
		io.WriteString(w, strings.ReplaceAll(AuthConfirmHTML, "#msg#", "CONN ERR:"+err.Error()))
		return
	}
	Body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		io.WriteString(w, strings.ReplaceAll(AuthConfirmHTML, "#msg#", "READ ERR:"+err.Error()))
		return
	}
	Cookies := main.ParseRequest(resp.Header.Get("Set-Cookie"), ";")
	Session := strings.TrimSpace(Cookies.Get("MoodleSession"))
	log.Println("AuthSession: " + Session)
	preg := regexp.MustCompile("logintoken\".+value=\"(.+)\"")

	found := preg.FindStringSubmatch(string(Body))
	if len(found) < 2 {
		io.WriteString(w, strings.ReplaceAll(AuthConfirmHTML, "#msg#", "TOKEN NOT FOUND"))
		return
	}
	LoginToken := found[1]

	// Get user provided creds
	reqAuthData, err := io.ReadAll(r.Body)
	if err != nil {
		io.WriteString(w, strings.ReplaceAll(AuthConfirmHTML, "#msg#", "READ ERR:"+err.Error()))
		return
	}
	reqAuthValues, _ := url.ParseQuery(string(reqAuthData))

	authData := url.Values{}
	authData.Set("logintoken", LoginToken)
	authData.Set("username", reqAuthValues.Get("username"))
	authData.Set("password", reqAuthValues.Get("password"))

	x, _ := url.QueryUnescape(authData.Encode())
	authBody := strings.NewReader(x)
	authReq, err := http.NewRequest("POST", "https://edu.vsu.ru/login/index.php", authBody)
	authReq.Header.Set("Cookie", "MoodleSession="+Session)
	authReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	authReq.Header.Set("Content-Length", strconv.Itoa(len(x)))
	log.Println(x, authReq.Header)
	authResp, err := uclient.Do(authReq)
	if err != nil {
		log.Println(err)
	}
	au := authResp.Header.Get("Set-Cookie")
	newCookies := main.ParseRequest(au, ";")
	newSession := newCookies.Get("MoodleSession")
	log.Println(newSession)
	w.Header().Set("Set-Cookie", "MoodleSession="+newSession+"; path=/")
	// СЕССИЯ ПРИХОДИТ И ОНА ВАЛИДНА
	io.WriteString(w, strings.ReplaceAll(AuthConfirmHTML, "#msg#", "Вход успешен"))
	return
}

type NoodleSession struct {
	uname         string
	password      string
	moodleSession string
	NoodleCookie  string
}

func GetNoodleSession(r *http.Request) NoodleSession {
	Cookies := main.ParseRequest(r.Header.Get("Cookie"), ";")
	Session := Cookies.Get("NoodleSession")
	if Session == "" {
		return NoodleSession{}
	}
	return NoodleSession{}
}
