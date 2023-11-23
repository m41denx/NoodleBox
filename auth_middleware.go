package main

import (
	"bytes"
	_ "embed"
	"encoding/gob"
	"errors"
	"github.com/akrylysov/pogreb"
	"github.com/cradio/NoodleBox/models"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/valyala/fasthttp"
	"log"
	"regexp"
	"strings"
	"time"
)

type FastUserConfig struct {
	Uname           string
	MoodleSession   string
	HasSubscription bool
}

type AuthMiddleware struct {
	cache         *pogreb.DB //NoodleSession -> {Uname, MoodleSession, HasSubscription}
	website       string
	sessionseeker map[string]string //uname -> noodlesession
}

// NewAuthMiddleware returns new Middleware for Moodle Authentication and Sub Management
func NewAuthMiddleware(website string) *AuthMiddleware {
	// Initialize cache
	db, err := pogreb.Open("sessions", nil)
	if err != nil {
		log.Fatalln(err)
	}

	return &AuthMiddleware{
		cache:         db,
		website:       website,
		sessionseeker: make(map[string]string),
	}
}

// getUserBySession returns FastUserConfig (empty if none found)
func (mid *AuthMiddleware) getUserBySession(sess string) FastUserConfig {
	u, err := mid.cache.Get([]byte(sess))
	if err != nil {
		return FastUserConfig{}
	}
	var usr FastUserConfig
	gob.NewDecoder(bytes.NewBuffer(u)).Decode(&usr)
	return usr
}

func (mid *AuthMiddleware) refreshSession(sess string, useragent string) (session string, err error) {
	usr := mid.getUserBySession(sess)
	if len(usr.Uname) == 0 {
		return "", errors.New("no cached user")
	}
	return mid.createSession(usr.Uname, "", useragent)
}

// createSession makes new NoodleSession if none provided
func (mid *AuthMiddleware) createSession(uname string, password string, useragent string) (session string, err error) {
	// Retrieve creds for authentication
	var noodleUser models.User
	var user FastUserConfig
	tx := DB.Where(&models.User{Username: uname}).First(&noodleUser)
	if tx.RowsAffected == 0 {
		if len(password) == 0 {
			// that means we called invalid refreshSession
			return "", errors.New("No password provided")
		}
		// User doesn't exist yet
		user, err = mid.newUser(uname, password, useragent)
		if err != nil {
			return "", err
		}

	} else {
		user.Uname = noodleUser.Username
		// Get session and also check subscription status because why not
		moodleSession, err := mid.GetMoodleSessionForCreds(noodleUser.Username, noodleUser.Password, useragent)
		if err != nil {
			return "", err
		}
		user.MoodleSession = moodleSession

		Subbed := (noodleUser.Subscription.UnixNano() - time.Now().UnixNano()) > 0
		user.HasSubscription = Subbed
	}

	session = uuid.NewString()

	// Push back to cache
	buf := bytes.NewBuffer(nil)
	if err = gob.NewEncoder(buf).Encode(user); err != nil {
		return
	}
	return session, mid.cache.Put([]byte(session), buf.Bytes())
}

// newUser creates new user for Noodle system and retrieves MoodleSession
func (mid *AuthMiddleware) newUser(uname string, password string, useragent string) (user FastUserConfig, err error) {
	// Check if user is real using Moodle itself
	session, err := mid.GetMoodleSessionForCreds(uname, password, useragent)
	if err != nil {
		return
	}

	// One Piece is real!
	usr := models.User{
		Username: uname,
		Password: password,
	}
	tx := DB.Create(&usr)
	if err = tx.Error; err != nil {
		return
	}
	return FastUserConfig{
		Uname:           uname,
		MoodleSession:   session,
		HasSubscription: false, // Like man, you just registered, ofc there is no subscription
	}, nil
}

func (mid *AuthMiddleware) GetMoodleSessionForCreds(uname string, password string, useragent string) (sess string, err error) {
	c := fiber.AcquireClient()
	c.UserAgent = useragent

	// Get initial MoodleSession and logintoken
	initResp := fiber.AcquireResponse()
	defer fiber.ReleaseResponse(initResp)
	code, body, errs := c.Get(mid.website + "/login/index.php").SetResponse(initResp).String()
	if len(errs) > 0 {
		return "", errs[0]
	}
	if code != 200 {
		log.Println("MOODLE LOGIN(1) Not 200 WTF")
	}
	// MoodleSession will be needed to authenticate
	cock := fasthttp.Cookie{}
	cock.ParseBytes(initResp.Header.PeekCookie("MoodleSession"))
	LoginSess := string(cock.Value())
	//initResp.Header.VisitAllCookie()
	log.Println(LoginSess)

	// Extract logintoken from login page (regex, capture groups)
	preg := regexp.MustCompile("logintoken\".+value=\"(.+)\"")
	found := preg.FindStringSubmatch(body)
	if len(found) < 2 {
		return "", errors.New("logintoken not found")
	}
	LoginToken := found[1]

	// Attempt login with provided creds and newly obtained logintoken
	resp := fiber.AcquireResponse()
	defer fiber.ReleaseResponse(resp)
	data := &fiber.Args{}
	data.Set("logintoken", LoginToken)
	data.Set("username", uname)
	data.Set("password", password)

	code, body, errs = c.Post(mid.website+"/login/index.php").
		Form(data).Cookie("MoodleSession", LoginSess).
		SetResponse(resp).String()
	if len(errs) > 0 {
		return "", errs[0]
	}
	if code != 200 {
		log.Println("MOODLE LOGIN(2) Not 200 WTF: ", code, string(resp.Header.Peek("Location")))
	}

	// Real MoodleSession that will be valid and returned to the client
	cock = fasthttp.Cookie{}
	cock.ParseBytes(resp.Header.PeekCookie("MoodleSession"))
	MoodleSess := string(cock.Value())
	log.Println(MoodleSess)

	return MoodleSess, nil
}

func (mid *AuthMiddleware) HandlerBefore(c *fiber.Ctx) error {
	// There are 3 zones:
	//   [ / ] is public
	//   [ /login/ ] POST is for auth only
	//   everything else needs authentication

	// Check if session is present
	session := c.Cookies("NoodleSession")
	c.Request().Header.DelCookie("MoodleSession") // Just in case
	var user FastUserConfig
	if len(session) > 0 {
		log.Println("Session OK", session, mid.cache.Count())
		// If cookie is set
		user = mid.getUserBySession(session)
		//fiber set request variable
		c.Locals("uname", user.Uname)
		if len(user.Uname) > 0 {
			log.Printf("%+v\n", user)
			// If session exists, we set appropriate cookie and proceed
			c.Request().Header.SetCookie("MoodleSession", user.MoodleSession)
			return c.Next()
		}
	}

	// If we are trying to log in
	if strings.HasPrefix(c.Path(), "/login/") && c.Method() == "POST" {
		username := c.FormValue("username")
		password := c.FormValue("password")
		useragent := c.GetReqHeaders()["User-Agent"]
		if len(username) > 0 && len(password) > 0 {
			noodleSession, err := mid.createSession(username, password, useragent[0])
			BadgeVisibility := "none"
			user = mid.getUserBySession(noodleSession)
			if user.HasSubscription {
				BadgeVisibility = "block"
			}
			if err == nil {
				err = errors.New("Благодарим за использование наших услуг")
			}
			c.Cookie(&fiber.Cookie{Name: "NoodleSession", Value: noodleSession, Path: "/", MaxAge: 2592000})
			return c.Render("assets/AuthConfirmed", fiber.Map{
				"Badge": BadgeVisibility,
				"Msg":   err.Error(),
			})
		}
	}
	return c.Next()
}
