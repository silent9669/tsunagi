package config

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"sort"
)

// The key index maps provider names that have a key in the keychain to the
// endpoint host the key was saved for ("" when not pinned). It holds no
// secrets. It lets `tsunagi providers` list keyed catalogue entries without
// probing the keychain once per entry, and it pins a catalogue provider's key
// to the host it was saved for: the catalogue is downloaded, so a changed
// endpoint must not receive an old key.
func keyedPath() string {
	p := Path()
	if p == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(p), "keys.json")
}

func readKeyed() map[string]string {
	m := map[string]string{}
	b, err := os.ReadFile(keyedPath())
	if err != nil {
		return m
	}
	if json.Unmarshal(b, &m) != nil {
		var legacy []string // first version stored a plain list
		if json.Unmarshal(b, &legacy) == nil {
			for _, n := range legacy {
				m[n] = ""
			}
		}
	}
	return m
}

func Keyed() []string {
	var names []string
	for n := range readKeyed() {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Pin returns the host a provider's key was saved for, or "".
func Pin(name string) string { return readKeyed()[name] }

// Host extracts the host part used for pinning.
func Host(baseURL string) string {
	u, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	return u.Host
}

// SetKeyed records that name has a key, pinned to host when host is not empty.
func SetKeyed(name string, on bool, host string) error {
	m := readKeyed()
	if cur, ok := m[name]; ok == on && cur == host {
		return nil
	}
	if on {
		m[name] = host
	} else {
		delete(m, name)
	}
	b, _ := json.Marshal(m)
	if err := os.MkdirAll(filepath.Dir(keyedPath()), 0o700); err != nil {
		return err
	}
	return os.WriteFile(keyedPath(), b, 0o600)
}
