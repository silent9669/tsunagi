// Package keychain stores provider keys in the macOS login keychain through
// the `security` tool. Keys never appear in argv: writes go through stdin.
// The e2e build swaps the storage backend (see filebackend_e2e.go).
package keychain

import (
	"errors"
	"regexp"
	"strings"
)

const service = "tsunagi"

var ErrNotFound = errors.New("no key stored")

// Characters real API keys use. Anything else is refused, so the key can be
// written to `security -i` without quoting.
var validKey = regexp.MustCompile(`^[A-Za-z0-9._~+/=:-]{8,512}$`)

var validName = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*$`)

// Has reports whether a key exists, without reading it.
func Has(name string) bool { return doHas(name) }

func Get(name string) (string, error) { return doGet(name) }

func Set(name, key string) error {
	if !validKey.MatchString(key) {
		return errors.New("key has unexpected characters or length")
	}
	if !validName.MatchString(name) {
		return errors.New("bad provider name")
	}
	return doSet(name, key)
}

func Delete(name string) error { return doDelete(name) }

func redact(s, key string) string { return strings.ReplaceAll(s, key, "***") }
