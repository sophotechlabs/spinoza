package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

const SessionCookie = "spinoza_session"

const maxCookieBytes = 3800

type sessionClaims struct {
	Expires int64    `json:"e"`
	Groups  []string `json:"g,omitempty"`
	Issued  int64    `json:"i"`
	Role    string   `json:"r"`
	Session string   `json:"s"`
	User    string   `json:"u"`
}

type session struct {
	who     Identity
	issued  time.Time
	expires time.Time
}

type sessions struct {
	secret []byte
	ttl    time.Duration
	maxAge time.Duration
	secure bool
	now    func() time.Time
}

func newSessions(secret []byte, ttl, maxAge time.Duration, secure bool) *sessions {
	return &sessions{secret: secret, ttl: ttl, maxAge: maxAge, secure: secure, now: time.Now}
}

func NewSecret() []byte {
	secret := make([]byte, 32)
	_, _ = rand.Read(secret)
	return secret
}

func newSessionID() string {
	return strings.ToLower(rand.Text())
}

func (ss *sessions) sign(payload []byte) string {
	mac := hmac.New(sha256.New, ss.secret)
	_, _ = mac.Write(payload)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (ss *sessions) encode(who Identity, issued time.Time) (string, error) {
	claims := sessionClaims{
		User:    who.User,
		Groups:  who.Groups,
		Role:    who.Role,
		Session: who.Session,
		Issued:  issued.Unix(),
		Expires: ss.now().Add(ss.ttl).Unix(),
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	body := base64.RawURLEncoding.EncodeToString(payload)
	return body + "." + ss.sign([]byte(body)), nil
}

func (ss *sessions) decode(value string) (session, bool) {
	body, signature, found := strings.Cut(value, ".")
	if !found {
		return session{}, false
	}
	if !hmac.Equal([]byte(signature), []byte(ss.sign([]byte(body)))) {
		return session{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return session{}, false
	}
	var claims sessionClaims
	unmarshalErr := json.Unmarshal(payload, &claims)
	if unmarshalErr != nil {
		return session{}, false
	}
	if ss.now().Unix() >= claims.Expires {
		return session{}, false
	}
	who := Identity{
		User:    claims.User,
		Groups:  claims.Groups,
		Role:    claims.Role,
		Session: claims.Session,
	}
	if who.Anonymous() {
		return session{}, false
	}
	return session{
		who:     who,
		issued:  time.Unix(claims.Issued, 0),
		expires: time.Unix(claims.Expires, 0),
	}, true
}

func (ss *sessions) issue(w http.ResponseWriter, who Identity, issued time.Time) error {
	value, err := ss.encode(who, issued)
	if err != nil {
		return err
	}
	if len(value) > maxCookieBytes {
		return errCookieTooBig
	}
	http.SetCookie(w, ss.cookie(SessionCookie, value, ss.ttl))
	return nil
}

func (ss *sessions) clear(w http.ResponseWriter) {
	http.SetCookie(w, ss.cookie(SessionCookie, "", -time.Hour))
}

//nolint:gosec // Secure follows the public url, so a lab served over plain http still keeps its session
func (ss *sessions) cookie(name, value string, age time.Duration) *http.Cookie {
	return &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   ss.secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(age.Seconds()),
	}
}

func (ss *sessions) read(r *http.Request) (session, bool) {
	cookie, err := r.Cookie(SessionCookie)
	if err != nil {
		return session{}, false
	}
	return ss.decode(cookie.Value)
}

func (ss *sessions) halfSpent(expires time.Time) bool {
	return expires.Sub(ss.now()) < ss.ttl/2
}

func (ss *sessions) renewable(issued time.Time) bool {
	return ss.now().Sub(issued) < ss.maxAge
}

func (ss *sessions) stash(w http.ResponseWriter, name string, payload any, age time.Duration) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	encoded := base64.RawURLEncoding.EncodeToString(body)
	value := encoded + "." + ss.sign([]byte(encoded))
	if len(value) > maxCookieBytes {
		return errCookieTooBig
	}
	http.SetCookie(w, ss.cookie(name, value, age))
	return nil
}

func (ss *sessions) unstash(r *http.Request, into any) bool {
	return ss.unstashNamed(r, flowCookie, into)
}

func (ss *sessions) unstashNamed(r *http.Request, name string, into any) bool {
	cookie, err := r.Cookie(name)
	if err != nil {
		return false
	}
	body, signature, found := strings.Cut(cookie.Value, ".")
	if !found {
		return false
	}
	if !hmac.Equal([]byte(signature), []byte(ss.sign([]byte(body)))) {
		return false
	}
	payload, decodeErr := base64.RawURLEncoding.DecodeString(body)
	if decodeErr != nil {
		return false
	}
	return json.Unmarshal(payload, into) == nil
}

func (ss *sessions) drop(w http.ResponseWriter, name string) {
	http.SetCookie(w, ss.cookie(name, "", -time.Hour))
}
