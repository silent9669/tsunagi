package plans

import (
	"strings"
	"testing"
	"time"

	"github.com/silent9669/tsunagi/internal/provider"
)

func TestRegistryComplete(t *testing.T) {
	builtin := map[string]bool{}
	for _, p := range provider.Builtin() {
		builtin[p.Name] = true
	}
	seen := map[string]bool{}
	for _, e := range Registry() {
		for f, v := range map[string]string{
			"Name": e.Name, "Price": e.Price, "Signup": e.Signup,
			"KeyURL": e.KeyURL, "Terms": e.Terms, "Source": e.Source,
		} {
			if v == "" {
				t.Errorf("%s: %s empty", e.ID, f)
			}
		}
		if !strings.HasPrefix(e.Signup, "https://") || !strings.HasPrefix(e.KeyURL, "https://") ||
			!strings.HasPrefix(e.Source, "https://") {
			t.Errorf("%s: urls must be https", e.ID)
		}
		if _, err := time.Parse("2006-01-02", e.Checked); err != nil {
			t.Errorf("%s: Checked %q: %v", e.ID, e.Checked, err)
		}
		if !provider.ValidName(e.ID) {
			t.Errorf("%s: not a valid provider name", e.ID)
		}
		if !builtin[e.ID] {
			t.Errorf("%s: missing from provider.Builtin()", e.ID)
		}
		if seen[e.ID] {
			t.Errorf("%s: duplicated", e.ID)
		}
		seen[e.ID] = true
	}
}

func TestLooksLikePlan(t *testing.T) {
	for _, id := range []string{"foo-coding-plan", "x-token-plan", "y-code-plan"} {
		if !LooksLikePlan(id) {
			t.Errorf("%s should look like a plan", id)
		}
	}
	for _, id := range []string{"groq", "openrouter", "planner"} {
		if LooksLikePlan(id) {
			t.Errorf("%s should not look like a plan", id)
		}
	}
}
