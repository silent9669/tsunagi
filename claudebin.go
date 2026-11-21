//go:build !e2e

package main

import "os/exec"

// claudeBin finds the claude binary.
func claudeBin() (string, error) { return exec.LookPath("claude") }
