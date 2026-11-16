// Package provider lists the model providers tsunagi can route to.
package provider

import "strings"

type Dialect string

const (
	Anthropic       Dialect = "anthropic"
	OpenAIChat      Dialect = "openai-chat"
	OpenAIResponses Dialect = "openai-responses"
)

type Provider struct {
	Name    string  `json:"name"`
	BaseURL string  `json:"base_url"`
	Dialect Dialect `json:"dialect"`
	// Local providers need no key.
	Local bool `json:"local,omitempty"`
	// Auth names a sign-in flow ("chatgpt") used instead of a key.
	Auth string `json:"auth,omitempty"`
	// Untrusted marks providers whose endpoint came from the downloaded
	// catalogue. The router refuses to connect them to loopback or private
	// addresses.
	Untrusted bool `json:"-"`
}

// Ready reports whether the router can talk to the provider today.
func (p Provider) Ready() bool {
	return p.Dialect == Anthropic || p.Dialect == OpenAIChat || p.Dialect == OpenAIResponses
}

func Builtin() []Provider {
	return []Provider{
		{Name: "opencode", BaseURL: "https://opencode.ai/inference/anthropic", Dialect: Anthropic},
		{Name: "opencode-go", BaseURL: "https://opencode.ai/inference/go/anthropic", Dialect: Anthropic},
		{Name: "openrouter", BaseURL: "https://openrouter.ai/api", Dialect: Anthropic},
		{Name: "zai", BaseURL: "https://api.z.ai/api/anthropic", Dialect: Anthropic},
		{Name: "deepseek", BaseURL: "https://api.deepseek.com/anthropic", Dialect: Anthropic},
		{Name: "moonshot", BaseURL: "https://api.moonshot.ai/anthropic", Dialect: Anthropic},
		{Name: "minimax", BaseURL: "https://api.minimax.io/anthropic", Dialect: Anthropic},
		{Name: "qwen", BaseURL: "https://dashscope-intl.aliyuncs.com/apps/anthropic", Dialect: Anthropic},
		{Name: "fireworks", BaseURL: "https://api.fireworks.ai/inference", Dialect: Anthropic},
		{Name: "llamacpp", BaseURL: "http://127.0.0.1:8080", Dialect: Anthropic, Local: true},
		{Name: "chatgpt", BaseURL: "https://api.openai.com/v1", Dialect: OpenAIResponses, Auth: "chatgpt"},
		{Name: "openai", BaseURL: "https://api.openai.com/v1", Dialect: OpenAIResponses},
		{Name: "xai", BaseURL: "https://api.x.ai/v1", Dialect: OpenAIResponses},
		{Name: "groq", BaseURL: "https://api.groq.com/openai/v1", Dialect: OpenAIChat},
		{Name: "together", BaseURL: "https://api.together.xyz/v1", Dialect: OpenAIChat},
		{Name: "mistral", BaseURL: "https://api.mistral.ai/v1", Dialect: OpenAIChat},
	}
}

// DialectForPackage maps an npm or runtime package name, as models.dev and
// opencode name them, to the Dialect tsunagi speaks to that provider.
func DialectForPackage(pkg string) (Dialect, bool) {
	p := strings.ToLower(pkg)
	switch {
	case strings.Contains(p, "anthropic"):
		return Anthropic, true
	case strings.Contains(p, "responses"), p == "@ai-sdk/openai", strings.HasSuffix(p, "providers/openai"):
		return OpenAIResponses, true
	case strings.Contains(p, "openai-compatible"), strings.HasSuffix(p, "providers/openai/chat"),
		strings.HasSuffix(p, "ai-sdk/groq"), strings.HasSuffix(p, "ai-sdk/mistral"),
		strings.HasSuffix(p, "ai-sdk/togetherai"), strings.HasSuffix(p, "ai-sdk/cerebras"),
		strings.HasSuffix(p, "ai-sdk/deepinfra"):
		return OpenAIChat, true
	}
	return "", false
}
