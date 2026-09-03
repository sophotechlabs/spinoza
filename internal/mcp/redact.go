package mcp

import (
	"regexp"
	"strings"

	"github.com/sophotechlabs/spinoza/internal/api"
	"gopkg.in/yaml.v3"
)

const hidden = "[redacted by spinoza]"

const rawOutputWithheld = "raw output withheld; restart with -unsafe-raw-output only if credential exposure is acceptable"

var secretish = []string{
	"password", "passwd", "secret", "token", "apikey", "api_key", "accesskey",
	"access_key", "credential", "private_key", "privatekey", "auth", "session",
	"signature", "certificate", "connectionstring", "connection_string", "dsn",
}

var exactSecretish = map[string]struct{}{
	"code":    {},
	"cookie":  {},
	"key":     {},
	"license": {},
	"uri":     {},
	"url":     {},
}

var (
	bearer     = regexp.MustCompile(`(?i)\b(bearer|basic)\s+[A-Za-z0-9._~+/=-]{8,}`)
	marker     = `(?:` + strings.Join(secretish, "|") + `)`
	assignment = regexp.MustCompile(`(?i)\b([\w.-]*` + marker + `[\w.-]*)"?[ \t]*[:=][ \t]*("?)([^\s",}]{4,})("?)`)
	envPair    = regexp.MustCompile(`(?im)^(\s*-?\s*name:\s*["']?[\w.-]*` + marker + `[\w.-]*["']?\s*\r?\n\s*value:\s*)(\S.*)$`)
	github     = regexp.MustCompile(`\b(?:gh[pousr]_|github_pat_)[A-Za-z0-9_]{20,}\b`)
	prefixed   = regexp.MustCompile(`\b(?:xox[baprs]-[A-Za-z0-9-]{10,}|(?:sk|rk)_(?:live|test)_[A-Za-z0-9]{12,}|glpat-[A-Za-z0-9_-]{12,}|AIza[A-Za-z0-9_-]{20,}|(?:AKIA|ASIA)[A-Z0-9]{16}|SG\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,})\b`)
	jwt        = regexp.MustCompile(`\beyJ[A-Za-z0-9._-]{16,}`)
	pemBlock   = regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`)
	longBlob   = regexp.MustCompile(`\b[A-Za-z0-9+/]{40,}={0,2}\b`)
)

func looksSecret(name string) bool {
	lowered := strings.ToLower(strings.TrimSpace(name))
	if _, found := exactSecretish[lowered]; found {
		return true
	}
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
	text = github.ReplaceAllString(text, hidden)
	text = prefixed.ReplaceAllString(text, hidden)
	text = jwt.ReplaceAllString(text, hidden)
	text = envPair.ReplaceAllString(text, "${1}"+hidden)
	text = assignment.ReplaceAllString(text, "$1: $2"+hidden+"$4")
	return longBlob.ReplaceAllString(text, hidden)
}

func scrubMap(pairs map[string]string) map[string]string {
	if len(pairs) == 0 {
		return nil
	}
	out := make(map[string]string, len(pairs))
	for key, value := range pairs {
		if looksSecret(key) {
			out[key] = hidden
			continue
		}
		out[key] = scrub(value)
	}
	return out
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
	return scrubStructuredYAML(document)
}

func scrubStructuredYAML(document string) string {
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(document), &root); err != nil {
		return scrub(document)
	}
	if !redactYAML(&root) {
		return scrub(document)
	}
	rendered, err := yaml.Marshal(&root)
	if err != nil {
		return scrub(document)
	}
	return scrub(string(rendered))
}

func redactYAML(node *yaml.Node) bool {
	changed := false
	if node.Kind == yaml.MappingNode {
		for at := 0; at+1 < len(node.Content); at += 2 {
			key := node.Content[at]
			value := node.Content[at+1]
			if looksSecret(key.Value) {
				value.Kind = yaml.ScalarNode
				value.Tag = "!!str"
				value.Value = hidden
				value.Content = nil
				value.Alias = nil
				changed = true
				continue
			}
			if redactYAML(value) {
				changed = true
			}
		}
		return changed
	}
	for _, child := range node.Content {
		if redactYAML(child) {
			changed = true
		}
	}
	return changed
}
