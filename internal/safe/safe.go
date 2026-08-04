package safe

import (
	"fmt"
	"log/slog"
	"runtime/debug"
)

func Go(what string, fn func()) {
	go Run(what, fn)
}

func Run(what string, fn func()) {
	defer Recover(what)
	fn()
}

func Recover(what string) {
	Log(what, recover())
}

func Log(what string, caught any) {
	if caught == nil {
		return
	}
	slog.Error("recovered from a panic", "in", what, "panic", fmt.Sprint(caught), "stack", string(debug.Stack()))
}
