package auth

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

const testKeyID = "spinoza-test"

var testKey = sync.OnceValue(func() *rsa.PrivateKey {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	return key
})

func encodePart(value any) string {
	body, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(body)
}

func signJWT(claims map[string]any) string {
	header := map[string]any{"alg": "RS256", "typ": "JWT", "kid": testKeyID}
	signing := encodePart(header) + "." + encodePart(claims)
	sum := sha256.Sum256([]byte(signing))
	signature, err := rsa.SignPKCS1v15(rand.Reader, testKey(), crypto.SHA256, sum[:])
	if err != nil {
		panic(err)
	}
	return signing + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func jwks() map[string]any {
	pub := testKey().PublicKey
	return map[string]any{
		"keys": []map[string]any{{
			"kty": "RSA",
			"kid": testKeyID,
			"alg": "RS256",
			"use": "sig",
			"n":   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
		}},
	}
}

type fakeIDP struct {
	server    *httptest.Server
	issuer    string
	claims    map[string]any
	tokenCode int
	noIDToken bool
	noLogout  bool
	noSIDs    bool
	scopes    []string

	mu   sync.Mutex
	seen map[string]string
}

func newIDP(t *testing.T) *fakeIDP {
	t.Helper()
	idp := &fakeIDP{seen: map[string]string{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", idp.metadata)
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jwks())
	})
	mux.HandleFunc("/token", idp.token)
	idp.server = httptest.NewServer(mux)
	idp.issuer = idp.server.URL
	idp.claims = idp.standardClaims("")
	t.Cleanup(idp.server.Close)
	return idp
}

func (idp *fakeIDP) metadata(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(idp.discovery())
}

func (idp *fakeIDP) discovery() map[string]any {
	out := map[string]any{
		"issuer":                                idp.issuer,
		"authorization_endpoint":                idp.issuer + "/authorize",
		"token_endpoint":                        idp.issuer + "/token",
		"jwks_uri":                              idp.issuer + "/jwks",
		"userinfo_endpoint":                     idp.issuer + "/userinfo",
		"id_token_signing_alg_values_supported": []string{"RS256"},
		"backchannel_logout_supported":          true,
		"backchannel_logout_session_supported":  !idp.noSIDs,
	}
	if len(idp.scopes) > 0 {
		out["scopes_supported"] = idp.scopes
	}
	if !idp.noLogout {
		out["end_session_endpoint"] = idp.issuer + "/logout"
	}
	return out
}

func (idp *fakeIDP) token(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	idp.mu.Lock()
	for key := range r.Form {
		idp.seen[key] = r.Form.Get(key)
	}
	idp.mu.Unlock()
	if idp.tokenCode != 0 {
		w.WriteHeader(idp.tokenCode)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
		return
	}
	body := map[string]any{"access_token": "at", "token_type": "Bearer"}
	if !idp.noIDToken {
		body["id_token"] = signJWT(idp.claims)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

func (idp *fakeIDP) received(key string) string {
	idp.mu.Lock()
	defer idp.mu.Unlock()
	return idp.seen[key]
}

func (idp *fakeIDP) standardClaims(nonce string) map[string]any {
	return map[string]any{
		"iss":                idp.issuer,
		"aud":                "spinoza",
		"sub":                "1a2b",
		"exp":                time.Now().Add(time.Hour).Unix(),
		"iat":                time.Now().Unix(),
		"nonce":              nonce,
		"sid":                "session-7",
		"preferred_username": "alice",
		"email":              "alice@example.com",
		"groups":             []string{"platform"},
	}
}

func oidcConfig(idp *fakeIDP) OIDCConfig {
	return OIDCConfig{
		IssuerURL:      idp.issuer,
		ClientID:       "spinoza",
		ClientSecret:   "shh",
		RedirectURL:    "https://spinoza.example.com/auth/callback",
		Scopes:         DefaultScopes,
		GroupsClaim:    DefaultGroupsClaim,
		UsernameClaims: strings.Split(DefaultUsernameKeys, ","),
	}
}

func authFor(t *testing.T, idp *fakeIDP, change func(*Config)) *Authenticator {
	t.Helper()
	cfg := Config{
		Mode:          ModeOIDC,
		PublicURL:     "https://spinoza.example.com",
		SessionSecret: []byte("a-test-secret"),
		OIDC:          oidcConfig(idp),
	}
	if change != nil {
		change(&cfg)
	}
	built, err := New(t.Context(), cfg)
	if err != nil {
		t.Fatalf("building the authenticator: %v", err)
	}
	return built
}
