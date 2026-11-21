//go:build e2e

package main

import (
	"errors"
	"os"
	"os/exec"
)

// claudeBin honors TSUNAGI_E2E_CLAUDE as the claude binary path.
func claudeBin() (string, error) {
	if p := os.Getenv("TSUNAGI_E2E_CLAUDE"); p != "" {
		return p, nil
	}
	p, err := exec.LookPath("claude")
	if err != nil {
		return "", errors.New("claude not found")
	}
	return p, nil
}
