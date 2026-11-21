//go:build e2e

package keychain

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// impl names the storage backend; tests assert it per build tag.
const impl = "file"

// fileStore keeps keys as 0600 JSON files under $TSUNAGI_E2E_DIR/keys so the
// e2e build never touches the real keychain.
type fileStore struct {
	mu   sync.Mutex
	dir  string
	file string
}

var store = func() *fileStore {
	dir := os.Getenv("TSUNAGI_E2E_DIR")
	if dir == "" {
		dir = os.TempDir()
	}
	return &fileStore{dir: dir, file: filepath.Join(dir, "keys.json")}
}()

func (s *fileStore) load() map[string]string {
	out := map[string]string{}
	b, err := os.ReadFile(s.file)
	if err != nil {
		return out
	}
	json.Unmarshal(b, &out)
	return out
}

func (s *fileStore) save(m map[string]string) error {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	tmp := s.file + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.file)
}

func doHas(name string) bool {
	store.mu.Lock()
	defer store.mu.Unlock()
	_, ok := store.load()[name]
	return ok
}

func doGet(name string) (string, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if k, ok := store.load()[name]; ok {
		return k, nil
	}
	return "", ErrNotFound
}

func doSet(name, key string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	m := store.load()
	m[name] = key
	return store.save(m)
}

func doDelete(name string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	m := store.load()
	if _, ok := m[name]; !ok {
		return errors.New("no key stored")
	}
	delete(m, name)
	return store.save(m)
}
