package translate

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// ChatOptions are the endpoint quirks the chat translator works around.
type ChatOptions struct {
	MaxTokensField     string // "max_tokens" or "max_completion_tokens"
	ReasoningField     string // set (with RequireReasoning) to send it on prior assistant messages
	RequireReasoning   bool
	StreamUsage        bool
	AssistantAfterTool bool
	ShortToolIDs       bool // 9 letters and digits
	CacheAnthropic     bool
}

// DefaultChatOptions has no workarounds.
var DefaultChatOptions = ChatOptions{MaxTokensField: "max_tokens", StreamUsage: true}

// ToChat is ToChatWith using DefaultChatOptions.
func ToChat(body []byte, model string) ([]byte, bool, error) {
	return ToChatWith(body, model, DefaultChatOptions)
}

// ToChatWith turns an Anthropic /v1/messages body into a Chat Completions body.
// It reports whether the client asked for a stream.
func ToChatWith(body []byte, model string, o ChatOptions) ([]byte, bool, error) {
	var in aReq
	if err := json.Unmarshal(body, &in); err != nil {
		return nil, false, fmt.Errorf("invalid request body: %w", err)
	}
	var raw struct {
		Temperature   *float64 `json:"temperature"`
		TopP          *float64 `json:"top_p"`
		StopSequences []string `json:"stop_sequences"`
	}
	json.Unmarshal(body, &raw)

	out := map[string]any{"model": model, "stream": in.Stream}
	if in.Stream && o.StreamUsage {
		out["stream_options"] = map[string]any{"include_usage": true}
	}
	if in.MaxTokens > 0 {
		field := o.MaxTokensField
		if field == "" {
			field = "max_tokens"
		}
		out[field] = in.MaxTokens
	}
	if raw.Temperature != nil {
		out["temperature"] = *raw.Temperature
	}
	if raw.TopP != nil {
		out["top_p"] = *raw.TopP
	}
	if len(raw.StopSequences) > 0 {
		out["stop"] = raw.StopSequences
	}

	var msgs []any
	if s := blocksText(in.System); s != "" {
		msgs = append(msgs, map[string]any{"role": "system", "content": s})
	}
	for _, m := range in.Messages {
		msgs = append(msgs, chatMessages(m, o)...)
	}
	if o.AssistantAfterTool {
		msgs = assistantAfterTool(msgs)
	}
	out["messages"] = msgs

	var tools []any
	for _, t := range in.Tools {
		if (t.Type != "" && t.Type != "custom") || len(t.InputSchema) == 0 {
			continue
		}
		tools = append(tools, map[string]any{"type": "function", "function": map[string]any{
			"name": t.Name, "description": t.Description, "parameters": t.InputSchema,
		}})
	}
	if o.CacheAnthropic {
		markCache(msgs, tools)
	}
	if len(tools) > 0 {
		out["tools"] = tools
		var tc struct {
			Type string `json:"type"`
			Name string `json:"name"`
		}
		if json.Unmarshal(in.ToolChoice, &tc) == nil {
			switch tc.Type {
			case "auto":
				out["tool_choice"] = "auto"
			case "any":
				out["tool_choice"] = "required"
			case "none":
				out["tool_choice"] = "none"
			case "tool":
				out["tool_choice"] = map[string]any{"type": "function", "function": map[string]any{"name": tc.Name}}
			}
		}
	}
	b, err := json.Marshal(out)
	return b, in.Stream, err
}

// chatMessages maps one Anthropic message to chat messages. Tool results come
// first, because chat requires them right after the assistant's tool calls.
func chatMessages(m aMsg, o ChatOptions) []any {
	var s string
	if json.Unmarshal(m.Content, &s) == nil {
		msg := map[string]any{"role": m.Role, "content": s}
		reasoningBack(msg, m.Role, o)
		return []any{msg}
	}
	var blocks []aBlock
	if json.Unmarshal(m.Content, &blocks) != nil {
		return nil
	}
	var results, parts, calls []any
	var text []string
	hasImage := false
	for _, b := range blocks {
		switch b.Type {
		case "text":
			if b.Text != "" {
				text = append(text, b.Text)
				parts = append(parts, map[string]any{"type": "text", "text": b.Text})
			}
		case "image":
			if b.Source != nil && m.Role == "user" {
				u := b.Source.URL
				if b.Source.Type == "base64" {
					u = "data:" + b.Source.MediaType + ";base64," + b.Source.Data
				}
				hasImage = true
				parts = append(parts, map[string]any{"type": "image_url", "image_url": map[string]any{"url": u}})
			}
		case "tool_use":
			args := string(b.Input)
			if args == "" || args == "null" {
				args = "{}"
			}
			calls = append(calls, map[string]any{"id": toolID(b.ID, o), "type": "function",
				"function": map[string]any{"name": b.Name, "arguments": args}})
		case "tool_result":
			results = append(results, map[string]any{"role": "tool", "tool_call_id": toolID(b.ToolUseID, o), "content": blocksText(b.Content)})
		}
	}
	msgs := results
	switch {
	case m.Role == "assistant" && (len(text) > 0 || len(calls) > 0):
		am := map[string]any{"role": "assistant", "content": strings.Join(text, "\n\n")}
		if len(text) == 0 {
			am["content"] = nil
		}
		if len(calls) > 0 {
			am["tool_calls"] = calls
		}
		reasoningBack(am, "assistant", o)
		msgs = append(msgs, am)
	case m.Role == "user" && len(parts) > 0:
		if hasImage {
			msgs = append(msgs, map[string]any{"role": "user", "content": parts})
		} else {
			msgs = append(msgs, map[string]any{"role": "user", "content": strings.Join(text, "\n\n")})
		}
	}
	return msgs
}

type cUsage struct {
	PromptTokens        int `json:"prompt_tokens"`
	CompletionTokens    int `json:"completion_tokens"`
	PromptCacheHit      int `json:"prompt_cache_hit_tokens"`
	PromptTokensDetails struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
}

func (u cUsage) anthropic() map[string]int {
	cached := u.PromptTokensDetails.CachedTokens + u.PromptCacheHit
	return map[string]int{
		"input_tokens":            u.PromptTokens - cached,
		"output_tokens":           u.CompletionTokens,
		"cache_read_input_tokens": cached,
	}
}

type cCall struct {
	Index    *int   `json:"index"`
	ID       string `json:"id"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type cChoice struct {
	Message struct {
		Content   string  `json:"content"`
		ToolCalls []cCall `json:"tool_calls"`
	} `json:"message"`
	Delta struct {
		Content   string  `json:"content"`
		ToolCalls []cCall `json:"tool_calls"`
	} `json:"delta"`
	FinishReason string `json:"finish_reason"`
}

type cResp struct {
	ID      string    `json:"id"`
	Choices []cChoice `json:"choices"`
	Usage   *cUsage   `json:"usage"`
	Error   *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func chatStop(finish string, sawTool bool) string {
	switch {
	case sawTool || finish == "tool_calls" || finish == "function_call":
		return "tool_use"
	case finish == "length":
		return "max_tokens"
	}
	return "end_turn"
}

// ChatReplyToAnthropic turns a non-streamed Chat Completions body into an
// Anthropic message.
func ChatReplyToAnthropic(body []byte, model string) ([]byte, error) {
	var r cResp
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("invalid upstream reply: %w", err)
	}
	if len(r.Choices) == 0 {
		return nil, fmt.Errorf("upstream reply has no choices")
	}
	c := r.Choices[0]
	content := []any{}
	if c.Message.Content != "" {
		content = append(content, map[string]any{"type": "text", "text": c.Message.Content})
	}
	for _, tc := range c.Message.ToolCalls {
		input := json.RawMessage(tc.Function.Arguments)
		if !json.Valid(input) {
			input = json.RawMessage("{}")
		}
		content = append(content, map[string]any{"type": "tool_use", "id": tc.ID, "name": tc.Function.Name, "input": input})
	}
	usage := map[string]int{"input_tokens": 0, "output_tokens": 0}
	if r.Usage != nil {
		usage = r.Usage.anthropic()
	}
	return json.Marshal(map[string]any{
		"id": "msg_" + r.ID, "type": "message", "role": "assistant", "model": model,
		"content": content, "stop_reason": chatStop(c.FinishReason, len(c.Message.ToolCalls) > 0),
		"stop_sequence": nil, "usage": usage,
	})
}

type chatStreamer struct {
	*sink
	text    *block
	tools   map[int]*block
	cur     *block // the block currently open
	sawTool bool
	reason  string
	usage   map[string]int
}

// ChatStreamToAnthropic reads Chat Completions server-sent events and writes
// the equivalent Anthropic events, flushing after each one.
func ChatStreamToAnthropic(r io.Reader, w io.Writer, flush func(), model string) error {
	s := &chatStreamer{sink: &sink{w: w, flush: flush, model: model}, tools: map[int]*block{}}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), 16<<20)
	for sc.Scan() && !s.done {
		line := sc.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		if data != "" {
			s.handle([]byte(data))
		}
	}
	if s.done {
		return nil
	}
	if err := sc.Err(); err != nil {
		s.fail("upstream stream broke: " + err.Error())
		return err
	}
	if s.reason == "" && !s.started {
		s.fail("upstream stream ended before the reply was complete")
		return nil
	}
	s.close(s.cur)
	usage := s.usage
	if usage == nil {
		usage = map[string]int{"input_tokens": 0, "output_tokens": 0}
	}
	s.finish(chatStop(s.reason, s.sawTool), usage)
	return nil
}

func (s *chatStreamer) switchTo(b *block) {
	if s.cur != b {
		s.close(s.cur)
	}
	s.cur = b
}

func (s *chatStreamer) handle(data []byte) {
	var ch cResp
	if json.Unmarshal(data, &ch) != nil {
		return
	}
	if ch.Error != nil {
		msg := ch.Error.Message
		if msg == "" {
			msg = "upstream error"
		}
		s.fail(msg)
		return
	}
	s.start(ch.ID)
	if ch.Usage != nil {
		s.usage = ch.Usage.anthropic()
	}
	if len(ch.Choices) == 0 {
		return
	}
	c := ch.Choices[0]
	if t := c.Delta.Content; t != "" {
		if s.text == nil || !s.text.open {
			s.switchTo(nil)
			s.text = s.openBlock(textBlock, false)
		}
		s.switchTo(s.text)
		s.textDelta(s.text, t)
	}
	for i, tc := range c.Delta.ToolCalls {
		idx := i
		if tc.Index != nil {
			idx = *tc.Index
		}
		b := s.tools[idx]
		if b == nil {
			s.switchTo(nil)
			s.sawTool = true
			b = s.openBlock(toolBlock(tc.ID, tc.Function.Name), true)
			s.tools[idx] = b
		}
		s.switchTo(b)
		if tc.Function.Arguments != "" {
			s.argsDelta(b, tc.Function.Arguments)
		}
	}
	if c.FinishReason != "" {
		s.reason = c.FinishReason
	}
}

// reasoningBack adds the empty reasoning field some endpoints insist on. The
// translator never keeps reasoning, so an empty string is all it can send.
func reasoningBack(msg map[string]any, role string, o ChatOptions) {
	if role == "assistant" && o.RequireReasoning {
		field := o.ReasoningField
		if field == "" {
			field = "reasoning_content"
		}
		msg[field] = ""
	}
}

// toolID rewrites an id to 9 letters and digits when the endpoint needs that.
// The same input always gives the same output, so calls and results still match.
func toolID(id string, o ChatOptions) string {
	if !o.ShortToolIDs {
		return id
	}
	if len(id) == 9 && strings.Trim(id, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789") == "" {
		return id
	}
	sum := sha256.Sum256([]byte(id))
	return hex.EncodeToString(sum[:])[:9]
}

// assistantAfterTool puts a short assistant turn between a tool result and a
// following user message.
func assistantAfterTool(msgs []any) []any {
	var out []any
	for i, m := range msgs {
		out = append(out, m)
		cur, _ := m.(map[string]any)
		if cur == nil || cur["role"] != "tool" || i+1 >= len(msgs) {
			continue
		}
		if next, _ := msgs[i+1].(map[string]any); next != nil && next["role"] == "user" {
			out = append(out, map[string]any{"role": "assistant", "content": "Understood."})
		}
	}
	return out
}

// markCache adds Anthropic-style cache_control markers to the system prompt,
// the last tool and the last message.
func markCache(msgs, tools []any) {
	mark := func(m map[string]any) {
		switch c := m["content"].(type) {
		case string:
			m["content"] = []any{map[string]any{"type": "text", "text": c, "cache_control": map[string]any{"type": "ephemeral"}}}
		case []any:
			if len(c) > 0 {
				if last, ok := c[len(c)-1].(map[string]any); ok {
					last["cache_control"] = map[string]any{"type": "ephemeral"}
				}
			}
		}
	}
	if len(msgs) > 0 {
		if m, ok := msgs[0].(map[string]any); ok && m["role"] == "system" {
			mark(m)
		}
		if m, ok := msgs[len(msgs)-1].(map[string]any); ok && m["role"] != "system" {
			mark(m)
		}
	}
	if len(tools) > 0 {
		if t, ok := tools[len(tools)-1].(map[string]any); ok {
			t["cache_control"] = map[string]any{"type": "ephemeral"}
		}
	}
}
