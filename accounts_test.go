package main

import (
	"reflect"
	"testing"

	"github.com/silent9669/tsunagi/internal/config"
)

func TestParseAs(t *testing.T) {
	rest, label, err := parseAs([]string{"openai", "--as", "work"})
	if err != nil || label != "work" || !reflect.DeepEqual(rest, []string{"openai"}) {
		t.Fatalf("rest=%v label=%q err=%v", rest, label, err)
	}
	if _, label, _ := parseAs([]string{"openai", "--as", "default"}); label != "" {
		t.Fatalf("default label must be empty, got %q", label)
	}
	for _, bad := range [][]string{{"x", "--as"}, {"x", "--as", "Bad Label"}, {"x", "--as", "a.b"}} {
		if _, _, err := parseAs(bad); err == nil {
			t.Errorf("%v accepted", bad)
		}
	}
}

func TestAccountsOfPutsDefaultFirst(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, a := range []string{"openai.work", "openai.alt", "openai", "groq", "openai-compat"} {
		config.SetKeyed(a, true, "")
	}
	got := accountsOf("openai")
	if !reflect.DeepEqual(got, []string{"openai", "openai.alt", "openai.work"}) {
		t.Fatalf("accounts = %v (groq and openai-compat are other providers)", got)
	}
	if baseOf("openai.work") != "openai" || labelOf("openai.work") != "work" || labelOf("openai") != "" {
		t.Fatal("baseOf/labelOf")
	}
}
