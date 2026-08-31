package auth

import "errors"

var errCookieTooBig = errors.New("the session cookie would be too large; your identity provider returns more groups than a cookie holds")

var errNoProvider = errors.New("spinoza was not started with an identity provider")

var errStateMismatch = errors.New("the login did not come back with the state spinoza sent; start again")

var errNoIDToken = errors.New("the identity provider returned no id token")

var errNonceMismatch = errors.New("the id token carries a nonce spinoza did not send")

var errNoUsername = errors.New("the id token carries none of the claims spinoza reads a username from")
