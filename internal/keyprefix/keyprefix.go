// Package keyprefix guesses which provider an API key belongs to from its
// prefix. Only unambiguous prefixes ever return a name; the key itself is
// never logged or returned.
package keyprefix

import (
	"errors"
	"strings"
)

// ErrClaudeKey marks a Claude key: Claude uses the user's own login.
var ErrClaudeKey = errors.New("Claude uses your own login, not a key")

var prefixes = []struct {
	prefix string
	name   string
}{
	{"sk-or-v1-", "openrouter"},
	{"gsk_", "groq"},
	{"nvapi-", "nvidia"},
	{"xai-", "xai"},
	{"csk-", "cerebras"},
	{"pplx-", "perplexity"},
	{"fw_", "fireworks"},
	{"hf_", "huggingface"},
	{"sk-proj-", "openai"},
}

// Detect returns the provider names a key's prefix unambiguously names.
// A Claude key (sk-ant-...) and anything unrecognised return none.
func Detect(key string) []string {
	key = strings.TrimSpace(key)
	if key == "" || strings.HasPrefix(key, "sk-ant-") {
		return nil
	}
	if strings.HasPrefix(key, "AIza") && len(key) == 39 {
		return []string{"google"}
	}
	for _, p := range prefixes {
		if strings.HasPrefix(key, p.prefix) {
			return []string{p.name}
		}
	}
	return nil
}

// Check is Detect with the Claude refusal as an error.
func Check(key string) ([]string, error) {
	if strings.HasPrefix(strings.TrimSpace(key), "sk-ant-") {
		return nil, ErrClaudeKey
	}
	return Detect(key), nil
}
