//go:build !e2e

package keychain

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// impl names the storage backend; tests assert it per build tag.
const impl = "security"

// doHas reports whether a key exists in the login keychain.
func doHas(name string) bool {
	return exec.Command("security", "find-generic-password", "-a", name, "-s", service).Run() == nil
}

func doGet(name string) (string, error) {
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

func doSet(name, key string) error {
	cmd := exec.Command("security", "-i")
	cmd.Stdin = strings.NewReader(fmt.Sprintf("add-generic-password -U -a %s -s %s -w %s\n", name, service, key))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("security: %s", strings.TrimSpace(redact(string(out), key)))
	}
	return nil
}

func doDelete(name string) error {
	return exec.Command("security", "delete-generic-password", "-a", name, "-s", service).Run()
}
