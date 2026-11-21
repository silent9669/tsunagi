package free

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// ProbeMaxAge is how long a probe result stays usable for combo gating.
const ProbeMaxAge = 7 * 24 * time.Hour

// Result is the stored outcome of probing one combo candidate. Err is a
// short human-readable failure reason; it never contains credentials.
type Result struct {
	Target string    `json:"target"` // "provider/model"
	OK     bool      `json:"ok"`
	Tools  bool      `json:"tools"` // the reply contained a tool call
	Millis int64     `json:"ms"`
	At     time.Time `json:"at"`
	Err    string    `json:"err,omitempty"`
}

// ProbePath is the probe results cache; "" when there is no cache directory.
func ProbePath() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "tsunagi", "probe.json")
}

// LoadProbes returns the stored probe results keyed by target.
func LoadProbes() map[string]Result {
	var doc struct {
		Results map[string]Result `json:"results"`
	}
	b, err := os.ReadFile(ProbePath())
	if err != nil || json.Unmarshal(b, &doc) != nil {
		return map[string]Result{}
	}
	return doc.Results
}

// SaveProbes stores probe results atomically with 0600.
func SaveProbes(results map[string]Result) error {
	path := ProbePath()
	if path == "" {
		return nil
	}
	b, err := json.Marshal(map[string]any{"results": results})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
