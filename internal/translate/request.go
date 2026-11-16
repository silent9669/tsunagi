// Package translate converts between the Anthropic Messages API and the
// OpenAI Responses API, so Claude Code can talk to Responses-only providers.
//
// Thinking blocks are never produced: their signatures only mean something to
// Anthropic, and a fake one would break the session when you switch back to
// Claude. Thinking blocks in the history are dropped on the way out.
package translate

import (
	"encoding/json"
	"fmt"
	"strings"
)

type aReq struct {
	System     json.RawMessage `json:"system"`
	Messages   []aMsg          `json:"messages"`
	Tools      []aTool         `json:"tools"`
	ToolChoice json.RawMessage `json:"tool_choice"`
	MaxTokens  int             `json:"max_tokens"`
	Stream     bool            `json:"stream"`
	Thinking   *struct {
		Type string `json:"type"`
	} `json:"thinking"`
}

type aMsg struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type aTool struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type aBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	Source    *struct {
		Type      string `json:"type"`
		MediaType string `json:"media_type"`
		Data      string `json:"data"`
		URL       string `json:"url"`
	} `json:"source"`
}

// ToResponses turns an Anthropic /v1/messages body into a Responses body.
// It reports whether the client asked for a stream.
func ToResponses(body []byte, model string) ([]byte, bool, error) {
	var in aReq
	if err := json.Unmarshal(body, &in); err != nil {
		return nil, false, fmt.Errorf("invalid request body: %w", err)
	}
	out := map[string]any{
		"model":  model,
		"stream": in.Stream,
		"store":  false,
	}
	if s := blocksText(in.System); s != "" {
		out["instructions"] = s
	}
	if in.MaxTokens > 0 {
		out["max_output_tokens"] = in.MaxTokens
	}
	if in.Thinking != nil && in.Thinking.Type != "disabled" {
		out["reasoning"] = map[string]any{"effort": "medium"}
	}

	var items []any
	for _, m := range in.Messages {
		items = append(items, convertMessage(m)...)
	}
	out["input"] = items

	var tools []any
	for _, t := range in.Tools {
		if (t.Type != "" && t.Type != "custom") || len(t.InputSchema) == 0 {
			continue // server-side tools (web search etc.) have no equivalent here
		}
		tools = append(tools, map[string]any{
			"type": "function", "name": t.Name, "description": t.Description,
			"parameters": t.InputSchema, "strict": false,
		})
	}
	if len(tools) > 0 {
		out["tools"] = tools
		if tc := convertToolChoice(in.ToolChoice); tc != nil {
			out["tool_choice"] = tc
		}
	}
	b, err := json.Marshal(out)
	return b, in.Stream, err
}

func convertToolChoice(raw json.RawMessage) any {
	var tc struct {
		Type string `json:"type"`
		Name string `json:"name"`
	}
	if json.Unmarshal(raw, &tc) != nil {
		return nil
	}
	switch tc.Type {
	case "auto":
		return "auto"
	case "any":
		return "required"
	case "none":
		return "none"
	case "tool":
		return map[string]any{"type": "function", "name": tc.Name}
	}
	return nil
}

// convertMessage maps one Anthropic message to Responses input items, keeping
// the order of its blocks.
func convertMessage(m aMsg) []any {
	var s string
	if json.Unmarshal(m.Content, &s) == nil {
		return []any{textItem(m.Role, s)}
	}
	var blocks []aBlock
	if json.Unmarshal(m.Content, &blocks) != nil {
		return nil
	}
	var items []any
	var parts []any // pending text/image parts of the current message
	flush := func() {
		if len(parts) > 0 {
			items = append(items, map[string]any{"role": m.Role, "content": parts})
			parts = nil
		}
	}
	textType := "input_text"
	if m.Role == "assistant" {
		textType = "output_text"
	}
	for _, b := range blocks {
		switch b.Type {
		case "text":
			if b.Text != "" {
				parts = append(parts, map[string]any{"type": textType, "text": b.Text})
			}
		case "image":
			if b.Source != nil && m.Role == "user" {
				u := b.Source.URL
				if b.Source.Type == "base64" {
					u = "data:" + b.Source.MediaType + ";base64," + b.Source.Data
				}
				parts = append(parts, map[string]any{"type": "input_image", "image_url": u})
			}
		case "tool_use":
			flush()
			args := string(b.Input)
			if args == "" || args == "null" {
				args = "{}"
			}
			items = append(items, map[string]any{
				"type": "function_call", "call_id": b.ID, "name": b.Name, "arguments": args,
			})
		case "tool_result":
			flush()
			items = append(items, map[string]any{
				"type": "function_call_output", "call_id": b.ToolUseID, "output": blocksText(b.Content),
			})
		}
		// thinking, redacted_thinking and anything else: dropped
	}
	flush()
	return items
}

func textItem(role, text string) any {
	typ := "input_text"
	if role == "assistant" {
		typ = "output_text"
	}
	return map[string]any{"role": role, "content": []any{map[string]any{"type": typ, "text": text}}}
}

// blocksText reads a string, or an array of text blocks, as plain text.
func blocksText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var blocks []aBlock
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		if b.Type == "text" && b.Text != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n\n")
}
