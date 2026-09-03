package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
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

type stashClaims struct {
	Expires int64           `json:"e"`
	Payload json.RawMessage `json:"p"`
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
	return newSecret(rand.Reader)
}

func newSecret(source io.Reader) []byte {
	secret, err := secretFrom(source)
	if err != nil {
		panic(err)
	}
	return secret
}

func secretFrom(source io.Reader) ([]byte, error) {
	secret := make([]byte, 32)
	if _, err := io.ReadFull(source, secret); err != nil {
		return nil, fmt.Errorf("generate session secret: %w", err)
	}
	return secret, nil
}

func newSessionID() string {
	return strings.ToLower(rand.Text())
}

func (ss *sessions) sign(payload []byte) string {
	mac := hmac.New(sha256.New, ss.secret)
	_, _ = mac.Write(payload)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (ss *sessions) seal(payload any) (string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(body)
	value := encoded + "." + ss.sign([]byte(encoded))
	if len(value) > maxCookieBytes {
		return "", errCookieTooBig
	}
	return value, nil
}

func (ss *sessions) unseal(value string, into any) bool {
	body, signature, found := strings.Cut(value, ".")
	if !found {
		return false
	}
	if !hmac.Equal([]byte(signature), []byte(ss.sign([]byte(body)))) {
		return false
	}
	payload, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return false
	}
	return json.Unmarshal(payload, into) == nil
}

func (ss *sessions) encode(who Identity, issued time.Time) (string, error) {
	now := ss.now()
	return ss.encodeUntil(who, issued, ss.expiry(issued, now))
}

func (ss *sessions) encodeUntil(who Identity, issued, expires time.Time) (string, error) {
	return ss.seal(sessionClaims{
		User:    who.User,
		Groups:  who.Groups,
		Role:    who.Role,
		Session: who.Session,
		Issued:  issued.Unix(),
		Expires: expires.Unix(),
	})
}

func (ss *sessions) expiry(issued, now time.Time) time.Time {
	expires := now.Add(ss.ttl)
	maximum := issued.Add(ss.maxAge)
	if maximum.Before(expires) {
		return maximum
	}
	return expires
}

func (ss *sessions) decode(value string) (session, bool) {
	var claims sessionClaims
	if !ss.unseal(value, &claims) {
		return session{}, false
	}
	now := ss.now()
	issued := time.Unix(claims.Issued, 0)
	if !now.Before(time.Unix(claims.Expires, 0)) || !now.Before(issued.Add(ss.maxAge)) {
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
		issued:  issued,
		expires: time.Unix(claims.Expires, 0),
	}, true
}

func (ss *sessions) issue(w http.ResponseWriter, who Identity, issued time.Time) error {
	now := ss.now()
	expires := ss.expiry(issued, now)
	value, err := ss.encodeUntil(who, issued, expires)
	if err != nil {
		return err
	}
	http.SetCookie(w, ss.cookie(SessionCookie, value, expires.Sub(now)))
	return nil
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
	value, err := ss.seal(stashClaims{Expires: ss.now().Add(age).Unix(), Payload: body})
	if err != nil {
		return err
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
	var claims stashClaims
	if !ss.unseal(cookie.Value, &claims) {
		return false
	}
	if !ss.now().Before(time.Unix(claims.Expires, 0)) {
		return false
	}
	return json.Unmarshal(claims.Payload, into) == nil
}

func (ss *sessions) drop(w http.ResponseWriter, name string) {
	http.SetCookie(w, ss.cookie(name, "", -time.Hour))
}
