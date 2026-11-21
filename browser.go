//go:build !e2e

package main

import "os/exec"

// openBrowser opens a URL in the user's browser.
func openBrowser(url string) error { return exec.Command("open", url).Start() }
