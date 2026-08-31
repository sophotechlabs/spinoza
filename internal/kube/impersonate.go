package kube

import (
	"errors"
	"fmt"
	"net/http"

	"k8s.io/client-go/transport"

	"github.com/sophotechlabs/spinoza/internal/auth"
)

var errUnusableName = errors.New("carries characters an impersonation header cannot")

type impersonating struct {
	next http.RoundTripper
}

func Impersonating(next http.RoundTripper) http.RoundTripper {
	return impersonating{next: next}
}

func (im impersonating) RoundTrip(req *http.Request) (*http.Response, error) {
	who, ok := auth.ActingAs(req.Context())
	if !ok {
		return im.next.RoundTrip(req)
	}
	if !headerSafe(who.User) {
		return nil, fmt.Errorf("the signed-in user %q %w", who.User, errUnusableName)
	}
	cloned := req.Clone(req.Context())
	cloned.Header.Set(transport.ImpersonateUserHeader, who.User)
	cloned.Header.Del(transport.ImpersonateGroupHeader)
	for _, group := range who.Groups {
		if !headerSafe(group) {
			continue
		}
		cloned.Header.Add(transport.ImpersonateGroupHeader, group)
	}
	return im.next.RoundTrip(cloned)
}

func headerSafe(value string) bool {
	if value == "" {
		return false
	}
	for _, letter := range value {
		if letter < 0x20 || letter > 0x7e {
			return false
		}
	}
	return true
}
