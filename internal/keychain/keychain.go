// Package keychain stores provider keys in the macOS login keychain through
// the `security` tool. Keys never appear in argv: writes go through stdin.
package keychain

import (
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

const service = "tsunagi"

var ErrNotFound = errors.New("no key stored")

// Characters real API keys use. Anything else is refused, so the key can be
// written to `security -i` without quoting.
var validKey = regexp.MustCompile(`^[A-Za-z0-9._~+/=:-]{8,512}$`)

// Has reports whether a key exists, without reading it.
func Has(name string) bool {
	return exec.Command("security", "find-generic-password", "-a", name, "-s", service).Run() == nil
}

func Get(name string) (string, error) {
	out, err := exec.Command("security", "find-generic-password", "-a", name, "-s", service, "-w").Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() == 44 {
			return "", ErrNotFound
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func Set(name, key string) error {
	if !validKey.MatchString(key) {
		return errors.New("key has unexpected characters or length")
	}
	if !regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*$`).MatchString(name) {
		return errors.New("bad provider name")
	}
	cmd := exec.Command("security", "-i")
	cmd.Stdin = strings.NewReader(fmt.Sprintf("add-generic-password -U -a %s -s %s -w %s\n", name, service, key))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("security: %s", strings.TrimSpace(redact(string(out), key)))
	}
	return nil
}

func Delete(name string) error {
	return exec.Command("security", "delete-generic-password", "-a", name, "-s", service).Run()
}

func redact(s, key string) string { return strings.ReplaceAll(s, key, "***") }
