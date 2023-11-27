package middlewares

import (
	"bytes"
	_ "embed"
	"encoding/gob"
	"errors"
	"github.com/akrylysov/pogreb"
	"github.com/cradio/NoodleBox/models"
	"github.com/cradio/NoodleBox/utils"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/valyala/fasthttp"
	"gorm.io/gorm"
	"log"
	"regexp"
	"strings"
	"time"
)

// AuthMiddleware allows seamless authorization independent of device
type AuthMiddleware struct {
	db            *gorm.DB
	cache         *pogreb.DB //NoodleSession -> {Uname, MoodleSession, HasSubscription}
	website       string
	sessionseeker map[string]string //uname -> noodlesession
}

// NewAuthMiddleware returns new Middleware for Moodle Authentication and Sub Management
func NewAuthMiddleware(website string, gormDB *gorm.DB) *AuthMiddleware {
	// Initialize cache
	db, err := pogreb.Open("sessions", nil)
	if err != nil {
		log.Fatalln(err)
	}

	return &AuthMiddleware{
		db:            gormDB,
		cache:         db,
		website:       website,
		sessionseeker: make(map[string]string),
	}
}

// region Hooks

func (mid *AuthMiddleware) GetIngressHooks() []*models.RouteHook {
	return []*models.RouteHook{
		{
			Route:      "/login/.*",
			Method:     "POST",
			SkipOrigin: true,
			Handler:    mid.HandlerAuth,
		},
		{
			Route:      "/.*",
			Method:     "*",
			SkipOrigin: false,
			Handler:    mid.HandlerBefore,
		},
	}
}

func (mid *AuthMiddleware) GetEgressHooks() []*models.RouteHook {
	return []*models.RouteHook{
		{
			Route:      "/.*",
			Method:     "*",
			SkipOrigin: false,
			Handler:    mid.HandlerAfter,
		},
	}
}

//endregion

// region Inlines

type FastUserConfig struct {
	Uname           string
	MoodleSession   string
	HasSubscription bool
}

// endregion

// region AuthMiddleware Internals

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
	user := mid.getUserBySession(sess)
	if len(user.Uname) == 0 {
		return "", errors.New("no cached user")
	}
	var noodleUser models.User
	tx := mid.db.Where(&models.User{Username: user.Uname}).First(&noodleUser)
	if tx.RowsAffected == 0 {
		return "", errors.New("no cached user")
	}
	moodSess, err := mid.GetMoodleSessionForCreds(noodleUser.Username, noodleUser.Password, useragent)
	if err != nil {
		return "", err
	}
	user.MoodleSession = moodSess
	buf := bytes.NewBuffer(nil)
	if err = gob.NewEncoder(buf).Encode(user); err != nil {
		return
	}
	return sess, mid.cache.Put([]byte(sess), buf.Bytes())
}

func (mid *AuthMiddleware) doAuth(uname string, password string, useragent string) (session string, err error) {
	var noodleUser models.User
	tx := mid.db.Where(&models.User{Username: uname}).First(&noodleUser)
	if tx.RowsAffected == 0 {
		// Fresh user
		return mid.createSession(uname, password, useragent)
	}
	if noodleUser.Password == password {
		// Valid password
		sess, ok := mid.sessionseeker[uname]
		if !ok {
			// No NoodleSession
			return mid.createSession(uname, password, useragent)
		}
		usr := mid.getUserBySession(sess)
		if usr.Uname == "" {
			return "", errors.New("no cached user")
		}
		if mid.isSessionExpired(usr.MoodleSession, useragent) {
			return mid.refreshSession(sess, useragent)
		}
		return sess, nil
	}
	// Invalid password or user changed password or idk
	return mid.createSession(uname, password, useragent)
}

// createSession makes new NoodleSession if none provided
func (mid *AuthMiddleware) createSession(uname string, password string, useragent string) (session string, err error) {
	// Retrieve creds for authentication
	var noodleUser models.User
	var user FastUserConfig
	tx := mid.db.Where(&models.User{Username: uname}).First(&noodleUser)
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
	mid.sessionseeker[uname] = session
	return session, mid.cache.Put([]byte(session), buf.Bytes())
}

func (mid *AuthMiddleware) isSessionExpired(moodleSession string, useragent string) bool {
	c := fiber.AcquireClient()
	c.UserAgent = useragent

	// Get initial MoodleSession and logintoken
	initResp := fiber.AcquireResponse()
	defer fiber.ReleaseResponse(initResp)
	a := c.Get(mid.website+"/user/preferences.php").Cookie("MoodleSession", moodleSession)
	if err := a.Do(a.Request(), initResp); err != nil {
		log.Println(err)
		return true
	}
	loc := initResp.Header.Peek("Location")
	if loc != nil && string(loc) == "https://edu.vsu.ru/login/index.php" {
		return true
	}
	return false
}

func (mid *AuthMiddleware) getSessKey(moodleSession string, useragent string) string {
	c := fiber.AcquireClient()
	c.UserAgent = useragent

	// Get initial MoodleSession and logintoken
	initResp := fiber.AcquireResponse()
	defer fiber.ReleaseResponse(initResp)
	code, body, err := c.Get(mid.website+"/user/preferences.php").Cookie("MoodleSession", moodleSession).String()
	if err != nil {
		log.Println(err)
		return ""
	}
	if code != 200 {
		return ""
	}
	preg := regexp.MustCompile("sesskey\":\"([a-zA-Z0-9]+)\"")
	found := preg.FindStringSubmatch(body)
	if len(found) < 2 {
		return ""
	}
	return found[1]
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
	tx := mid.db.Create(&usr)
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

	// Extract logintoken from login page (regex, capture groups)
	preg := regexp.MustCompile("logintoken\".+value=\"(.+)\"")
	found := preg.FindStringSubmatch(body)
	if len(found) < 2 {
		return "", errors.New("logintoken not found")
	}
	LoginToken := found[1]

	log.Println("=== Fetched login page", LoginSess, LoginToken)

	// Attempt login with provided creds and newly obtained logintoken
	resp := fiber.AcquireResponse()
	defer fiber.ReleaseResponse(resp)
	data := &fiber.Args{}
	data.Set("logintoken", LoginToken)
	data.Set("username", uname)
	data.Set("password", password)

	code, _, errs = c.Post(mid.website+"/login/index.php").
		Form(data).Cookie("MoodleSession", LoginSess).
		SetResponse(resp).String()
	log.Println("===", data.String(), LoginSess)
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

// endregion

func (mid *AuthMiddleware) HandlerBefore(c *fiber.Ctx, body *[]byte) error {
	// There are 3 zones:
	//   [ / ] is public
	//   [ /login/ ] POST is for auth only
	//   everything else needs authentication

	if c.Path() == "/login/index.php" {
		return nil
	}

	c.Locals("metrics").(*utils.GoMetrics).NewStep("SessionPatcher")

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

			//c.Locals("sess_id", mid.getSessKey(user.MoodleSession, "NoodleBox/Hasty Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko) Chrome/89.207.132.170 Safari/537.36"))
			return nil
		}
	}
	return nil
}

func (mid *AuthMiddleware) HandlerAfter(c *fiber.Ctx, body *[]byte) error {
	c.Locals("metrics").(*utils.GoMetrics).NewStep("SessionRefresher")
	loc := string(c.Response().Header.Peek("Location"))
	att := c.Locals("relogin_att")
	attx := ""
	if att != nil {
		attx = att.(string)
	}
	log.Println("PATH", c.Path())
	if strings.HasSuffix(loc, "/login/index.php") && len(attx) == 0 {
		c.Response().Reset()
		// Looks like moodleSession expired
		session := c.Cookies("NoodleSession")
		useragent := c.GetReqHeaders()["User-Agent"]
		if len(useragent) == 0 {
			useragent = []string{"NoodleBox/Hasty Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko) Chrome/89.207.132.170 Safari/537.36"}
		}
		_, err := mid.refreshSession(session, useragent[0])
		if err != nil {
			log.Println(err)
		}
		c.Locals("relogin_att", "yes")
		return c.RestartRouting()
	}
	// Everything is fine (probably)

	return nil
}

func (mid *AuthMiddleware) HandlerAuth(c *fiber.Ctx, body *[]byte) error {
	c.Locals("metrics").(*utils.GoMetrics).NewStep("Auth")
	// If we are trying to log in
	username := c.FormValue("username")
	password := c.FormValue("password")
	useragent := c.GetReqHeaders()["User-Agent"]
	if len(useragent) == 0 {
		useragent = []string{"NoodleBox/Hasty Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko) Chrome/89.207.132.170 Safari/537.36"}
	}
	if len(username) > 0 && len(password) > 0 {
		noodleSession, err := mid.doAuth(username, password, useragent[0])
		BadgeVisibility := "none"
		user := mid.getUserBySession(noodleSession)
		if user.HasSubscription {
			BadgeVisibility = "block"
		}
		if err == nil {
			err = errors.New("С возвращением!")
		}
		c.Cookie(&fiber.Cookie{Name: "NoodleSession", Value: noodleSession, Path: "/", MaxAge: 2592000})
		return c.Render("assets/AuthConfirmed", fiber.Map{
			"Badge": BadgeVisibility,
			"Msg":   err.Error(),
		})
	}
	return nil
}
