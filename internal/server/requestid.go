package server

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"net/http"
)

const requestHeader = "X-Spinoza-Request"

type requestKey struct{}

var requestNames = base32.NewEncoding("abcdefghijklmnopqrstuvwxyz234567").WithPadding(base32.NoPadding)

func newRequestID() string {
	var raw [8]byte
	_, _ = rand.Read(raw[:])
	return requestNames.EncodeToString(raw[:])
}

func withRequestID(r *http.Request, id string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), requestKey{}, id))
}

func requestIDOf(ctx context.Context) string {
	held, ok := ctx.Value(requestKey{}).(string)
	if !ok {
		return ""
	}
	return held
}
