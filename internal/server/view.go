package server

import (
	"net/http"
	"sync"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
)

const ViewDesktop = "desktop"

const ViewBrowser = "browser"

const noWindow = "this build has no desktop window"

const browserNeverCame = "the browser did not open spinoza; the window is staying"

const defaultIdleGrace = 5 * time.Second

const defaultBrowserAwait = 10 * time.Second

type Window interface {
	Show()
	Hide()
}

type BrowserOpener func() error

type views struct {
	mu      sync.Mutex
	desktop int
	browser int
	hidden  bool
	armed   bool
	given   bool
	timer   *time.Timer
	waiting []chan struct{}
	onIdle  func()
	grace   time.Duration
	await   time.Duration
}

func (v *views) settle(grace, await time.Duration) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.grace = grace
	v.await = await
}

func (v *views) patience() time.Duration {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.await
}

func ViewScript(kind string) string {
	return "<script>window.__SPINOZA_VIEW__=" + scriptValue(kind) + ";</script>"
}

func StartScript(view, context string) string {
	if view == "" && context == "" {
		return ""
	}
	return "<script>window.__SPINOZA_START__={view:" + scriptValue(view) +
		",context:" + scriptValue(context) + "};</script>"
}

func viewOf(r *http.Request) string {
	if r.URL.Query().Get("view") == ViewDesktop {
		return ViewDesktop
	}
	return ViewBrowser
}

func (v *views) live() int {
	if v.hidden {
		return v.browser
	}
	return v.desktop + v.browser
}

func (v *views) opened(kind string) {
	v.mu.Lock()
	if kind == ViewDesktop {
		v.desktop++
	} else {
		v.browser++
		for _, waiter := range v.waiting {
			close(waiter)
		}
		v.waiting = nil
	}
	v.armed = true
	v.reconsider()
	v.mu.Unlock()
}

func (v *views) closed(kind string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if kind == ViewDesktop && v.desktop > 0 {
		v.desktop--
	}
	if kind != ViewDesktop && v.browser > 0 {
		v.browser--
	}
	v.reconsider()
}

func (v *views) reconsider() {
	if !v.armed || v.given {
		return
	}
	if v.live() > 0 {
		v.stopTimer()
		return
	}
	if v.timer != nil {
		return
	}
	v.timer = time.AfterFunc(v.grace, v.giveUp)
}

func (v *views) giveUp() {
	v.mu.Lock()
	v.timer = nil
	gone := v.armed && !v.given && v.live() == 0
	onIdle := v.onIdle
	if gone {
		v.given = true
	}
	v.mu.Unlock()
	if !gone || onIdle == nil {
		return
	}
	onIdle()
}

func (v *views) stopTimer() {
	if v.timer == nil {
		return
	}
	v.timer.Stop()
	v.timer = nil
}

func (v *views) hide() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.hidden = true
	v.reconsider()
}

func (v *views) show() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.hidden = false
	v.reconsider()
}

func (v *views) isHidden() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.hidden
}

func (v *views) awaitBrowser() chan struct{} {
	v.mu.Lock()
	defer v.mu.Unlock()
	waiter := make(chan struct{})
	if v.browser > 0 {
		close(waiter)
		return waiter
	}
	v.waiting = append(v.waiting, waiter)
	return waiter
}

func (v *views) stopWaiting(waiter chan struct{}) {
	v.mu.Lock()
	defer v.mu.Unlock()
	for at, held := range v.waiting {
		if held != waiter {
			continue
		}
		v.waiting = append(v.waiting[:at], v.waiting[at+1:]...)
		return
	}
}

func (s *Server) UseWindow(window Window) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.window = window
}

func (s *Server) UseBrowser(open BrowserOpener) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.browser = open
}

func (s *Server) UseIdleExit(quit func()) {
	s.views.mu.Lock()
	defer s.views.mu.Unlock()
	s.views.onIdle = quit
}

func (s *Server) desktopWindow() Window {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.window
}

func (s *Server) browserOpener() BrowserOpener {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.browser
}

func (s *Server) readView(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, api.ViewState{
		Window: s.desktopWindow() != nil,
		Hidden: s.views.isHidden(),
	})
}

func (s *Server) toBrowser(w http.ResponseWriter, r *http.Request) {
	window := s.desktopWindow()
	open := s.browserOpener()
	if window == nil || open == nil {
		writeError(w, http.StatusNotImplemented, noWindow)
		return
	}
	waiter := s.views.awaitBrowser()
	defer s.views.stopWaiting(waiter)
	openErr := open()
	if openErr != nil {
		writeError(w, http.StatusInternalServerError, openErr.Error())
		return
	}
	select {
	case <-waiter:
	case <-time.After(s.views.patience()):
		writeJSON(w, api.ViewSwitch{Reason: browserNeverCame})
		return
	case <-r.Context().Done():
		writeJSON(w, api.ViewSwitch{Reason: browserNeverCame})
		return
	}
	window.Hide()
	s.views.hide()
	writeJSON(w, api.ViewSwitch{Switched: true})
}

func (s *Server) toDesktop(w http.ResponseWriter, r *http.Request) {
	window := s.desktopWindow()
	if window == nil {
		writeError(w, http.StatusNotImplemented, noWindow)
		return
	}
	window.Show()
	s.views.show()
	writeJSON(w, api.ViewSwitch{Switched: true})
}
