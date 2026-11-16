package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
)

// The key index lists provider names that have a key in the keychain. It holds
// names only, so `tsunagi providers` can show them without probing the
// keychain once per catalogue entry.
func keyedPath() string {
	p := Path()
	if p == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(p), "keys.json")
}

func Keyed() []string {
	b, err := os.ReadFile(keyedPath())
	if err != nil {
		return nil
	}
	var names []string
	json.Unmarshal(b, &names)
	return names
}

func SetKeyed(name string, on bool) error {
	set := map[string]bool{}
	for _, n := range Keyed() {
		set[n] = true
	}
	if on == set[name] {
		return nil
	}
	if on {
		set[name] = true
	} else {
		delete(set, name)
	}
	names := make([]string, 0, len(set))
	for n := range set {
		names = append(names, n)
	}
	sort.Strings(names)
	b, _ := json.Marshal(names)
	if err := os.MkdirAll(filepath.Dir(keyedPath()), 0o755); err != nil {
		return err
	}
	return os.WriteFile(keyedPath(), b, 0o644)
}
