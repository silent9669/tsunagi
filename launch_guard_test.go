package main

import (
	"testing"

	"github.com/silent9669/tsunagi/internal/config"
)

func TestRoutedAway(t *testing.T) {
	cfg := config.Config{
		Aliases: map[string]string{"fast": "groq/llama", "claude-haiku-5-5": "free/auto"},
		Combos:  map[string]config.Combo{"free/auto": {}, "work": {}},
	}
	f := routedAway(cfg)
	for m, want := range map[string]bool{
		"openrouter/inclusionai/ling-3.2-flash:free": true,
		"google/gemini-2.5-flash":                    true,
		"free/auto":                                  true,
		"work":                                       true,
		"fast":                                       true,
		"sonnet":                                     false,
		"opus[1m]":                                   false,
		"haiku":                                      false,
		"default":                                    false,
		"claude-sonnet-5-5":                          false,
		"claude-haiku-5-5":                           false,
		"arn:aws:bedrock:us-east-1:1:inference-profile/x": false,
		"": false,
	} {
		if got := f(m); got != want {
			t.Errorf("routedAway(%q) = %v, want %v", m, got, want)
		}
	}
}
