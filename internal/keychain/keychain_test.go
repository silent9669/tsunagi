package keychain

import (
	"os"
	"strings"
	"testing"
)

// Touches the real login keychain, so it only runs on request.
func TestRoundTrip(t *testing.T) {
	if os.Getenv("TSUNAGI_KEYCHAIN_TEST") == "" {
		t.Skip("set TSUNAGI_KEYCHAIN_TEST=1")
	}
	name := "tsunagi-selftest"
	defer Delete(name)
	if err := Set(name, "sk-test_1234567890"); err != nil {
		t.Fatal(err)
	}
	if !Has(name) {
		t.Fatal("Has = false")
	}
	if got, err := Get(name); err != nil || got != "sk-test_1234567890" {
		t.Fatalf("Get = %q, %v", got, err)
	}
	if err := Set(name, "bad key with spaces"); err == nil {
		t.Fatal("accepted a key with spaces")
	}
	Delete(name)
	if _, err := Get(name); err != ErrNotFound {
		t.Fatalf("after delete: %v", err)
	}
}

func TestBlobRoundTripLong(t *testing.T) {
	if os.Getenv("TSUNAGI_KEYCHAIN_TEST") == "" {
		t.Skip("set TSUNAGI_KEYCHAIN_TEST=1")
	}
	name := "tsunagi-selftest-blob"
	defer Delete(name)
	data := []byte(`{"access_token":"` + string(make([]byte, 0)) + `x"}` + strings.Repeat(`{"k":"v"}`, 150)) // ~1.4 KB
	if err := SetBlob(name, data); err != nil {
		t.Fatal(err)
	}
	got, err := GetBlob(name)
	if err != nil || string(got) != string(data) {
		t.Fatalf("blob mismatch (err=%v, len %d vs %d)", err, len(got), len(data))
	}
}
