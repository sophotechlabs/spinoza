package mcp

import (
	"regexp"
	"strings"

	"github.com/sophotechlabs/spinoza/internal/api"
)

const hidden = "[redacted by spinoza]"

var secretish = []string{
	"password", "passwd", "secret", "token", "apikey", "api_key", "accesskey",
	"access_key", "credential", "private_key", "privatekey", "auth", "session",
	"signature", "certificate", "connectionstring", "connection_string", "dsn",
}

var (
	bearer     = regexp.MustCompile(`(?i)\b(bearer|basic)\s+[A-Za-z0-9._~+/=-]{8,}`)
	marker     = `(?:` + strings.Join(secretish, "|") + `)`
	assignment = regexp.MustCompile(`(?i)\b([\w.-]*` + marker + `[\w.-]*)"?[ \t]*[:=][ \t]*("?)([^\s",}]{4,})("?)`)
	envPair    = regexp.MustCompile(`(?im)^(\s*-?\s*name:\s*["']?[\w.-]*` + marker + `[\w.-]*["']?\s*\r?\n\s*value:\s*)(\S.*)$`)
	jwt        = regexp.MustCompile(`\beyJ[A-Za-z0-9._-]{16,}`)
	pemBlock   = regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`)
	longBlob   = regexp.MustCompile(`\b[A-Za-z0-9+/]{40,}={0,2}\b`)
)

func looksSecret(name string) bool {
	lowered := strings.ToLower(name)
	for _, mark := range secretish {
		if strings.Contains(lowered, mark) {
			return true
		}
	}
	return false
}

func scrub(text string) string {
	if text == "" {
		return text
	}
	text = pemBlock.ReplaceAllString(text, hidden)
	text = bearer.ReplaceAllString(text, "$1 "+hidden)
	text = jwt.ReplaceAllString(text, hidden)
	text = envPair.ReplaceAllString(text, "${1}"+hidden)
	text = assignment.ReplaceAllString(text, "$1: $2"+hidden+"$4")
	return longBlob.ReplaceAllString(text, hidden)
}

func scrubLines(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, scrub(line))
	}
	return out
}

type dataKey struct {
	Key    string `json:"key"`
	Bytes  int    `json:"bytes"`
	Binary bool   `json:"binary,omitempty"`
}

func keysOnly(entries []api.DataEntry) []dataKey {
	out := make([]dataKey, 0, len(entries))
	for _, entry := range entries {
		out = append(out, dataKey{Key: entry.Key, Bytes: entry.Bytes, Binary: entry.Binary})
	}
	return out
}

func carriesSecrets(kind string) bool {
	return kind == "Secret"
}

func safeYAML(kind, document string) string {
	if carriesSecrets(kind) {
		return ""
	}
	return scrub(document)
}
