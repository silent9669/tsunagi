package keyprefix

import (
	"errors"
	"testing"
)

func TestDetect(t *testing.T) {
	for _, tc := range []struct {
		key  string
		want string
	}{
		{"sk-or-v1-abc123", "openrouter"},
		{"gsk_AbC123", "groq"},
		{"nvapi-xyz", "nvidia"},
		{"xai-abc", "xai"},
		{"csk-abc", "cerebras"},
		{"pplx-abc", "perplexity"},
		{"fw_abc", "fireworks"},
		{"hf_abc", "huggingface"},
		{"sk-proj-abc", "openai"},
		{"  gsk_padded  ", "groq"},
		{"", ""},
		{"sk-", ""},
		{"sk", ""},
		{"sk-ant-oat01-secret", ""},
		{"AIza-too-short", ""},
		{"gsk", ""},
		{"something-else-entirely", ""},
	} {
		got := Detect(tc.key)
		if tc.want == "" {
			if len(got) != 0 {
				t.Errorf("Detect(%q) = %v, want none", tc.key, got)
			}
			continue
		}
		if len(got) != 1 || got[0] != tc.want {
			t.Errorf("Detect(%q) = %v, want [%s]", tc.key, got, tc.want)
		}
	}
}

func TestDetectGoogleLength(t *testing.T) {
	k := "AIza" + "x"
	for len(k) < 39 {
		k += "x"
	}
	if got := Detect(k); len(got) != 1 || got[0] != "google" {
		t.Fatalf("39-char AIza key = %v", got)
	}
	if got := Detect(k + "x"); len(got) != 0 {
		t.Fatalf("40-char AIza key = %v", got)
	}
}

func TestCheckRefusesClaude(t *testing.T) {
	names, err := Check("sk-ant-oat01-secret")
	if !errors.Is(err, ErrClaudeKey) || names != nil {
		t.Fatalf("names=%v err=%v", names, err)
	}
	if err.Error() != "Claude uses your own login, not a key" {
		t.Fatalf("err = %q", err)
	}
	if _, err := Check("sk-ant-"); !errors.Is(err, ErrClaudeKey) {
		t.Fatalf("bare prefix err = %v", err)
	}
	if names, err := Check("gsk_x"); err != nil || len(names) != 1 || names[0] != "groq" {
		t.Fatalf("names=%v err=%v", names, err)
	}
	if names, err := Check("nope"); err != nil || len(names) != 0 {
		t.Fatalf("names=%v err=%v", names, err)
	}
}
