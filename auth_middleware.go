package main

import (
	"bytes"
	_ "embed"
	"encoding/gob"
	"errors"
	"github.com/akrylysov/pogreb"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"log"
	"regexp"
	"time"
)

type FastUserConfig struct {
	Uname           string
	MoodleSession   string
	HasSubscription bool
}

type AuthMiddleware struct {
	cache   *pogreb.DB //NoodleSession -> {Uname, MoodleSession, HasSubscription}
	website string
}

// NewAuthMiddleware returns new Middleware for Moodle Authentication and Sub Management
func NewAuthMiddleware(website string) *AuthMiddleware {
	// Initialize cache
	db, err := pogreb.Open("sessions.db", nil)
	if err != nil {
		log.Fatalln(err)
	}

	return &AuthMiddleware{
		cache:   db,
		website: website,
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
	var noodleUser User
	var user FastUserConfig
	tx := DB.Where(&User{Username: uname}).First(&noodleUser)
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
			return
		}
		user.MoodleSession = moodleSession

		Subbed := (time.Now().UnixNano() - noodleUser.Subscription.UnixNano()) > 0
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
	usr := User{
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
		HasSubscription: false, // Like man you just registered, ofc there is no subscription
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
	LoginSess := string(initResp.Header.PeekCookie("MoodleSession"))

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
		SetResponse(initResp).String()
	if len(errs) > 0 {
		return "", errs[0]
	}
	if code != 200 {
		log.Println("MOODLE LOGIN(2) Not 200 WTF: ", code)
	}

	// Real MoodleSession that will be valid and returned to the client
	MoodleSess := string(initResp.Header.PeekCookie("MoodleSession"))

	return MoodleSess, nil
}

func (mid *AuthMiddleware) handle(c *fiber.Ctx) error {

}
