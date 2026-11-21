//go:build e2e

package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// openBrowser records the URL instead of opening a real browser.
func openBrowser(url string) error {
	dir := os.Getenv("TSUNAGI_E2E_DIR")
	if dir == "" {
		return fmt.Errorf("TSUNAGI_E2E_DIR not set")
	}
	f, err := os.OpenFile(filepath.Join(dir, "opened.txt"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintln(f, url)
	return err
}
