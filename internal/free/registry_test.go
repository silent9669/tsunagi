package free

import (
	"regexp"
	"testing"
)

var checkedRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

func TestRegistryComplete(t *testing.T) {
	for _, e := range Registry() {
		if e.Signup == "" || e.KeyURL == "" || e.Limits == "" || e.Source == "" {
			t.Errorf("%s: Signup/KeyURL/Limits/Source must be set", e.ID)
		}
		switch e.Data {
		case Private, Trains, Unknown:
		default:
			t.Errorf("%s: bad Data %q", e.ID, e.Data)
		}
		if !checkedRe.MatchString(e.Checked) {
			t.Errorf("%s: bad Checked %q", e.ID, e.Checked)
		}
		if e.Access != "" && e.Access != Keyless {
			t.Errorf("%s: bad Access %q", e.ID, e.Access)
		}
		if _, dup := Excluded[e.ID]; dup {
			t.Errorf("%s: in both Registry and Excluded", e.ID)
		}
	}
}

func TestOnlyKiloKeyless(t *testing.T) {
	for _, e := range Registry() {
		if e.Access == Keyless && e.ID != "kilo" {
			t.Errorf("%s: keyless but not kilo", e.ID)
		}
	}
	var kilo *Entry
	for i, e := range Registry() {
		if e.ID == "kilo" {
			kilo = &Registry()[i]
		}
	}
	if kilo == nil {
		t.Fatal("no kilo entry")
	}
	if kilo.Access != Keyless || !kilo.FromCatalog || kilo.Mode != ModelsMode {
		t.Fatalf("kilo = %+v", *kilo)
	}
	if kilo.Data != Trains || kilo.Source != "https://kilo.ai/docs/getting-started/using-kilo-for-free" {
		t.Fatalf("kilo = %+v", *kilo)
	}
}

func TestNewEntries(t *testing.T) {
	byID := map[string]Entry{}
	for _, e := range Registry() {
		byID[e.ID] = e
	}
	r, ok := byID["reflection"]
	if !ok || r.Name != "Reflection AI (beta)" || r.Data != Unknown || !r.FromCatalog {
		t.Fatalf("reflection = %+v", r)
	}
	if r.Source != "https://reflection.ai/legal/beta-pricing-information" {
		t.Fatalf("reflection source = %q", r.Source)
	}
	y, ok := byID["requesty"]
	if !ok || y.Name != "Requesty" || y.Data != Unknown || !y.FromCatalog {
		t.Fatalf("requesty = %+v", y)
	}
	if y.Source != "https://docs.requesty.ai/features/free-models" {
		t.Fatalf("requesty source = %q", y.Source)
	}
}
