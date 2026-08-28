package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

func serving(t *testing.T, body string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server
}

const release = `{"tag_name":"v1.15.0","html_url":"https://github.com/sophotechlabs/spinoza/releases/tag/v1.15.0"}`

func TestANewerReleaseIsOfferedWithTheCommandThatInstallsIt(t *testing.T) {
	checker := New("v1.14.1", serving(t, release).URL)

	status := checker.Status(context.Background())

	if !status.Available {
		t.Fatalf("status = %+v, want a newer release offered", status)
	}
	if status.Latest != "v1.15.0" {
		t.Fatalf("latest = %q, want the published tag", status.Latest)
	}
	if status.Command != Command {
		t.Fatalf("command = %q, want the one the website gives out", status.Command)
	}
	if !strings.HasPrefix(status.URL, "https://github.com/") {
		t.Fatalf("url = %q, want a link to the release", status.URL)
	}
}

func TestTheReleaseYouAreOnIsNotAnUpdate(t *testing.T) {
	checker := New("v1.15.0", serving(t, release).URL)

	status := checker.Status(context.Background())

	if status.Available {
		t.Fatalf("status = %+v, want nothing offered to somebody already on it", status)
	}
	if !status.Checked {
		t.Fatal("the check did happen and should say so")
	}
	if status.Command != "" {
		t.Fatalf("command = %q, want none when there is nothing to install", status.Command)
	}
}

func TestAReleaseOlderThanYoursIsNotOffered(t *testing.T) {
	checker := New("v2.0.0", serving(t, release).URL)

	if checker.Status(context.Background()).Available {
		t.Fatal("an older release was offered as an update")
	}
}

// v1.10.0 is above v1.9.0. Sorted as text it is below it, which is the mistake
// this is here to keep out.
func TestVersionsAreComparedAsNumbersNotText(t *testing.T) {
	checker := New("v1.9.0", serving(t, `{"tag_name":"v1.10.0","html_url":"u"}`).URL)

	if !checker.Status(context.Background()).Available {
		t.Fatal("v1.10.0 was read as older than v1.9.0")
	}
}

func TestEveryPartOfTheVersionCounts(t *testing.T) {
	cases := []struct {
		current string
		latest  string
		want    bool
	}{
		{current: "v1.14.1", latest: "v2.0.0", want: true},
		{current: "v1.14.1", latest: "v1.15.0", want: true},
		{current: "v1.14.1", latest: "v1.14.2", want: true},
		{current: "v1.14.1", latest: "v1.14.1", want: false},
		{current: "v1.14.1", latest: "v1.14.0", want: false},
		{current: "v1.14.1", latest: "v1.13.9", want: false},
		{current: "v2.0.0", latest: "v1.99.99", want: false},
	}
	for _, one := range cases {
		if got := newer(one.latest, one.current); got != one.want {
			t.Errorf("newer(%s, %s) = %v, want %v", one.latest, one.current, got, one.want)
		}
	}
}

// A release candidate is a thing somebody chose to try, not a thing to be told
// about.
func TestAPreReleaseIsNotOffered(t *testing.T) {
	checker := New("v1.14.1", serving(t, `{"tag_name":"v2.0.0-rc.1","html_url":"u"}`).URL)

	if checker.Status(context.Background()).Available {
		t.Fatal("a release candidate was offered")
	}
}

// A build from a working copy carries a commit, and no commit is newer or older
// than a version. Saying so beats comparing it to one.
func TestABuildThatIsNotAReleaseIsNotCompared(t *testing.T) {
	var asked atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		asked.Add(1)
		_, _ = w.Write([]byte(release))
	}))
	t.Cleanup(server.Close)
	checker := New("7e5d198b36b1", server.URL)

	status := checker.Status(context.Background())

	if status.Available || status.Checked {
		t.Fatalf("status = %+v, want no comparison for a build with no version", status)
	}
	if status.Reason == "" {
		t.Fatal("nothing was said about why the check did not happen")
	}
	if asked.Load() != 0 {
		t.Fatalf("asked %d times, want a build with no version not to ask at all", asked.Load())
	}
}

func TestDevIsNotAVersionEither(t *testing.T) {
	if released("dev") {
		t.Fatal("dev was taken for a release")
	}
	if released("v1.2") {
		t.Fatal("a two-part version was taken for a release")
	}
	if released("v1.2.x") {
		t.Fatal("a version with a letter in it was taken for a release")
	}
	if released("v1.2.-3") {
		t.Fatal("a negative part was taken for a release")
	}
}

// Nothing about the check is worth interrupting anybody over, so a refusal, a
// timeout or nonsense in the answer all come back as no update and a reason.
func TestAnAnswerThatCannotBeReadIsNotAFailure(t *testing.T) {
	refusing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(refusing.Close)
	cases := map[string]string{
		"refused":   refusing.URL,
		"not json":  serving(t, "<html>rate limited</html>").URL,
		"nowhere":   "http://127.0.0.1:1/latest",
		"not a url": "://",
	}
	for name, endpoint := range cases {
		t.Run(name, func(t *testing.T) {
			status := New("v1.14.1", endpoint).Status(context.Background())
			if status.Available || status.Checked {
				t.Fatalf("status = %+v, want nothing claimed", status)
			}
			if status.Reason == "" {
				t.Fatal("no reason was given for the check not landing")
			}
		})
	}
}

// The version does not change while spinoza is open, and a tool that asks the
// internet on a timer is doing something nobody asked it to.
func TestTheQuestionIsAskedOnce(t *testing.T) {
	var asked atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		asked.Add(1)
		_, _ = w.Write([]byte(release))
	}))
	t.Cleanup(server.Close)
	checker := New("v1.14.1", server.URL)

	for range 5 {
		checker.Status(context.Background())
	}

	if asked.Load() != 1 {
		t.Fatalf("asked %d times, want once for the whole run", asked.Load())
	}
}

// A running spinoza asks spinoza.tech, which is where the project decides what
// it is told to install. The endpoint argument exists so tests can point
// elsewhere; nothing a person passes on the command line reaches it.
func TestARunningSpinozaAsksTheProjectsOwnEndpoint(t *testing.T) {
	if got := New("v1.0.0", "").endpoint; got != Endpoint {
		t.Fatalf("endpoint = %q, want %q", got, Endpoint)
	}
	if !strings.HasPrefix(Endpoint, "https://spinoza.tech/") {
		t.Fatalf("endpoint = %q, want it to go through the project's own site", Endpoint)
	}
}

// Answers are read up to a limit, so that whatever is on the other end cannot
// hand spinoza a gigabyte.
func TestAnEnormousAnswerIsCutOffRatherThanRead(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v1.15.0","html_url":"`))
		filler := strings.Repeat("x", 4096)
		for range (maxAnswer / len(filler)) + 8 {
			_, _ = w.Write([]byte(filler))
		}
		_, _ = w.Write([]byte(`"}`))
	}))
	t.Cleanup(server.Close)

	status := New("v1.14.1", server.URL).Status(context.Background())

	if status.Available {
		t.Fatalf("status = %+v, want a truncated answer to be unreadable rather than believed", status)
	}
}

// The endpoint learns which release asked, and on what, from the user-agent.
func TestTheRequestSaysWhichReleaseIsAsking(t *testing.T) {
	seen := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- r.Header.Get("User-Agent")
		_, _ = w.Write([]byte(release))
	}))
	t.Cleanup(server.Close)

	New("v1.14.1", server.URL).Status(context.Background())

	got := <-seen
	if !strings.HasPrefix(got, "spinoza/v1.14.1 (") {
		t.Fatalf("user-agent = %q, want the release and the platform", got)
	}
	if !strings.Contains(got, runtime.GOOS) {
		t.Fatalf("user-agent = %q, want the operating system in it", got)
	}
}
