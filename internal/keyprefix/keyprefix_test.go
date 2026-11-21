package keyprefix

import (
	"errors"
	"testing"
)

func TestDetect(t *testing.T) {
	for _, tc := range []struct {
		key  string
		want []string
	}{
		{"sk-or-v1-abc123", []string{"openrouter"}},
		{"gsk_AbC123", []string{"groq"}},
		{"nvapi-xyz", []string{"nvidia"}},
		{"xai-abc", []string{"xai"}},
		{"csk-abc", []string{"cerebras"}},
		{"pplx-abc", []string{"perplexity"}},
		{"fw_abc", []string{"fireworks"}},
		{"hf_abc", []string{"huggingface"}},
		{"sk-proj-abc", []string{"openai"}},
		{"oc_sk_abc", []string{"opencode-go", "opencode"}},
		{"sk-sp-abc", []string{"alibaba-coding-plan"}},
		{"sk-cp-abc", []string{"minimax"}},
		{"  gsk_padded  ", []string{"groq"}},
		{"", nil},
		{"sk-", nil},
		{"sk", nil},
		{"sk-ant-oat01-secret", nil},
		{"AIza-too-short", nil},
		{"gsk", nil},
		{"something-else-entirely", nil},
	} {
		got := Detect(tc.key)
		if len(got) != len(tc.want) {
			t.Errorf("Detect(%q) = %v, want %v", tc.key, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("Detect(%q) = %v, want %v", tc.key, got, tc.want)
				break
			}
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
