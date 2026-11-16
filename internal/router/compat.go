package router

import (
	"net/url"
	"strings"

	"github.com/silent9669/tsunagi/internal/provider"
	"github.com/silent9669/tsunagi/internal/translate"
)

func has(s string, subs ...string) bool {
	s = strings.ToLower(s)
	for _, x := range subs {
		if strings.Contains(s, x) {
			return true
		}
	}
	return false
}

func boolOr(v *bool, def bool) bool {
	if v != nil {
		return *v
	}
	return def
}

// chatOptions works out the quirks of a chat endpoint. Explicit settings in the
// provider's compat block win; the rest is detected from the provider and model
// name, following what opencode learned about each family.
func chatOptions(p provider.Provider, model string) translate.ChatOptions {
	host := ""
	if u, err := url.Parse(p.BaseURL); err == nil {
		host = u.Host
	}
	c := p.Compat
	o := translate.DefaultChatOptions

	// OpenAI's newer chat models reject max_tokens; most others still want it.
	o.MaxTokensField = "max_tokens"
	if has(host, "openai.com", "openai.azure.com") {
		o.MaxTokensField = "max_completion_tokens"
	}
	if c.MaxTokensField != "" {
		o.MaxTokensField = c.MaxTokensField
	}

	// DeepSeek-style thinking models want the reasoning field on every earlier
	// assistant message.
	o.ReasoningField = c.ReasoningField
	o.RequireReasoning = boolOr(c.RequireReasoning, c.ReasoningField != "" || has(model, "deepseek") || has(p.Name, "deepseek"))

	o.StreamUsage = boolOr(c.StreamUsage, true)

	mistral := has(model, "mistral", "codestral", "devstral", "magistral", "ministral", "pixtral") || has(p.Name, "mistral")
	o.AssistantAfterTool = boolOr(c.AssistantAfterTool, mistral)
	o.ShortToolIDs = c.ToolIDs == "alnum9" || (c.ToolIDs == "" && mistral)

	o.CacheAnthropic = c.CacheFormat == "anthropic"
	return o
}
