package router

import (
	"testing"

	"github.com/silent9669/tsunagi/internal/provider"
)

func TestChatOptionsDetection(t *testing.T) {
	yes := true
	no := false
	o := chatOptions(provider.Provider{Name: "openai", BaseURL: "https://api.openai.com/v1"}, "gpt-x")
	if o.MaxTokensField != "max_completion_tokens" {
		t.Errorf("openai max field = %s", o.MaxTokensField)
	}
	o = chatOptions(provider.Provider{Name: "deepseek", BaseURL: "https://api.deepseek.com"}, "deepseek-v4-pro")
	if o.MaxTokensField != "max_tokens" || !o.RequireReasoning {
		t.Errorf("deepseek: %+v", o)
	}
	o = chatOptions(provider.Provider{Name: "mistral", BaseURL: "https://api.mistral.ai/v1"}, "devstral-2")
	if !o.AssistantAfterTool || !o.ShortToolIDs {
		t.Errorf("mistral: %+v", o)
	}
	o = chatOptions(provider.Provider{Name: "gw", BaseURL: "http://127.0.0.1:4000/v1"}, "mistral-large")
	if !o.AssistantAfterTool {
		t.Errorf("a mistral model behind a gateway must be detected by model name: %+v", o)
	}
	o = chatOptions(provider.Provider{Name: "x", BaseURL: "https://x.example/v1", Compat: provider.Compat{
		MaxTokensField: "max_completion_tokens", StreamUsage: &no, RequireReasoning: &yes, ReasoningField: "reasoning", CacheFormat: "anthropic",
	}}, "plain")
	if o.MaxTokensField != "max_completion_tokens" || o.StreamUsage || !o.RequireReasoning || o.ReasoningField != "reasoning" || !o.CacheAnthropic {
		t.Errorf("explicit compat must win: %+v", o)
	}
	o = chatOptions(provider.Provider{Name: "plain", BaseURL: "https://x.example/v1"}, "llama")
	if o.RequireReasoning || o.AssistantAfterTool || o.ShortToolIDs || !o.StreamUsage {
		t.Errorf("plain endpoint must get no workarounds: %+v", o)
	}
}
