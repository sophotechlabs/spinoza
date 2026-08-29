package mcp

import (
	"strings"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

// what counts as a secret-shaped name

func TestWhichNamesReadAsSecrets(t *testing.T) {
	cases := []struct {
		name  string
		field string
		want  bool
	}{
		{name: "a password", field: "PASSWORD", want: true},
		{name: "an api key", field: "api_key", want: true},
		{name: "a bearer token", field: "AUTH_TOKEN", want: true},
		{name: "a connection string", field: "DB_CONNECTION_STRING", want: true},
		{name: "a private key", field: "tls.private_key", want: true},
		{name: "a plain setting", field: "LOG_LEVEL", want: false},
		{name: "a port", field: "HTTP_PORT", want: false},
		{name: "nothing", field: "", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := looksSecret(tc.field); got != tc.want {
				t.Fatalf("looksSecret(%q) = %v, want %v", tc.field, got, tc.want)
			}
		})
	}
}

// what a scrubbed line keeps and what it loses

func TestScrubbingALine(t *testing.T) {
	cases := []struct {
		name string
		line string
		gone string
		kept string
	}{
		{
			name: "a bearer token",
			line: "GET /v1 Authorization: Bearer abcdef0123456789abcdef",
			gone: "abcdef0123456789abcdef",
			kept: "GET /v1",
		},
		{
			name: "a jwt",
			line: "session=eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9 opened",
			gone: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
			kept: "opened",
		},
		{
			name: "a password assignment",
			line: "connecting with password=hunter2000 to db",
			gone: "hunter2000",
			kept: "connecting with",
		},
		{
			name: "a token in json",
			line: `{"api_token": "s3cr3t-value-here", "level": "info"}`,
			gone: "s3cr3t-value-here",
			kept: "info",
		},
		{
			name: "a private key block",
			line: pemFixture(),
			gone: "MIIabc",
			kept: "",
		},
		{
			name: "a long base64 blob",
			line: "payload " + strings.Repeat("QUJDRA", 9),
			gone: strings.Repeat("QUJDRA", 9),
			kept: "payload",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := scrub(tc.line)
			if strings.Contains(result, tc.gone) {
				t.Fatalf("scrub kept %q in %q", tc.gone, result)
			}
			if tc.kept != "" && !strings.Contains(result, tc.kept) {
				t.Fatalf("scrub lost %q from %q", tc.kept, result)
			}
			if !strings.Contains(result, hidden) {
				t.Fatalf("scrub removed the value without saying so: %q", result)
			}
		})
	}
}

func pemFixture() string {
	head := "-----BEGIN RSA " + "PRIVATE KEY-----"
	return head + "\nMIIabc\n" + strings.Replace(head, "BEGIN", "END", 1)
}

func TestScrubbingLeavesOrdinaryTextAlone(t *testing.T) {
	line := "listening on 127.0.0.1:34115, 3 workers, level=info"

	if got := scrub(line); got != line {
		t.Fatalf("scrub changed a line with no secret in it:\n got %q\nwant %q", got, line)
	}
	if got := scrub(""); got != "" {
		t.Fatalf("scrub of nothing = %q", got)
	}
}

func TestEveryLineIsScrubbed(t *testing.T) {
	result := scrubLines([]string{"password=letmein1", "all good"})

	if strings.Contains(result[0], "letmein1") {
		t.Fatalf("the first line kept its secret: %q", result[0])
	}
	if result[1] != "all good" {
		t.Fatalf("the second line changed: %q", result[1])
	}
}

// what leaves the process for a Secret

func TestASecretGivesUpKeysAndSizesOnly(t *testing.T) {
	entries := []api.DataEntry{
		{Key: "password", Value: "hunter2", Bytes: 7},
		{Key: "keystore", Value: "AAEC", Bytes: 3, Binary: true},
	}

	result := keysOnly(entries)

	expected := []dataKey{
		{Key: "password", Bytes: 7},
		{Key: "keystore", Bytes: 3, Binary: true},
	}
	if len(result) != len(expected) {
		t.Fatalf("keys = %v, want %v", result, expected)
	}
	for i, want := range expected {
		if result[i] != want {
			t.Fatalf("key %d = %+v, want %+v", i, result[i], want)
		}
	}
}

func TestTheRawDocumentIsWithheldForASecretAndScrubbedOtherwise(t *testing.T) {
	document := "apiVersion: v1\ndata:\n  password: aHVudGVyMg==\n"

	if got := safeYAML("Secret", document); got != "" {
		t.Fatalf("a Secret gave up its document: %q", got)
	}
	scrubbed := safeYAML("ConfigMap", "env:\n  - name: TOKEN\n    value: not-a-real-value\n")
	if strings.Contains(scrubbed, "not-a-real-value") {
		t.Fatalf("a ConfigMap document kept a token: %q", scrubbed)
	}
	plain := safeYAML("Deployment", "spec:\n  replicas: 3\n")
	if plain != "spec:\n  replicas: 3\n" {
		t.Fatalf("an ordinary document was altered: %q", plain)
	}
}

func TestOnlySecretsCarrySecrets(t *testing.T) {
	if !carriesSecrets("Secret") {
		t.Fatal("a Secret must be treated as carrying secrets")
	}
	if carriesSecrets("ConfigMap") {
		t.Fatal("a ConfigMap is not withheld; its values are not secret by kind")
	}
}

func TestScrubbingNestedYAML(t *testing.T) {
	cases := []struct {
		name string
		body string
		gone string
	}{
		{
			name: "a nested token",
			body: "auth:\n  token: not-a-real-value\n",
			gone: "not-a-real-value",
		},
		{
			name: "an env pair across two lines",
			body: "env:\n  - name: DB_PASSWORD\n    value: hunter2000\n",
			gone: "hunter2000",
		},
		{
			name: "an env pair with a quoted name",
			body: `env:` + "\n" + `  - name: "API_TOKEN"` + "\n" + `    value: not-a-real-value` + "\n",
			gone: "not-a-real-value",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := scrub(tc.body); strings.Contains(got, tc.gone) {
				t.Fatalf("scrub kept %q in %q", tc.gone, got)
			}
		})
	}
}

func TestAnEnvPairWithAnInnocentNameSurvives(t *testing.T) {
	body := "env:\n  - name: LOG_LEVEL\n    value: debug\n"

	if got := scrub(body); got != body {
		t.Fatalf("scrub changed an ordinary env pair:\n got %q\nwant %q", got, body)
	}
}

func TestAParentKeyIsNotMistakenForAValue(t *testing.T) {
	body := "auth:\n  mode: oidc\n"

	if got := scrub(body); !strings.Contains(got, "mode: oidc") {
		t.Fatalf("scrub ate a value that is not a secret: %q", got)
	}
}

func TestLabelsAreScrubbedLikeEverythingElse(t *testing.T) {
	result := scrubMap(map[string]string{
		"app":          "web",
		"auth-token":   "not-a-real-value",
		"release-note": "deployed with password=letmein1",
	})

	if result["app"] != "web" {
		t.Fatalf("an ordinary label changed: %q", result["app"])
	}
	if result["auth-token"] != hidden {
		t.Fatalf("a secret-shaped label kept its value: %q", result["auth-token"])
	}
	if strings.Contains(result["release-note"], "letmein1") {
		t.Fatalf("a label value kept a secret: %q", result["release-note"])
	}
	if scrubMap(nil) != nil {
		t.Fatal("no labels became an empty map rather than nothing")
	}
}
