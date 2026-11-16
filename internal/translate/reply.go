package translate

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type usage struct {
	InputTokens        int `json:"input_tokens"`
	OutputTokens       int `json:"output_tokens"`
	InputTokensDetails struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"input_tokens_details"`
}

// anthropicUsage splits cached tokens out, as Anthropic reports them.
func (u usage) anthropic() map[string]int {
	return map[string]int{
		"input_tokens":            u.InputTokens - u.InputTokensDetails.CachedTokens,
		"output_tokens":           u.OutputTokens,
		"cache_read_input_tokens": u.InputTokensDetails.CachedTokens,
	}
}

type rItem struct {
	Type      string `json:"type"`
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
	Content   []struct {
		Type    string `json:"type"`
		Text    string `json:"text"`
		Refusal string `json:"refusal"`
	} `json:"content"`
}

type rResp struct {
	ID                string  `json:"id"`
	Status            string  `json:"status"`
	Output            []rItem `json:"output"`
	Usage             usage   `json:"usage"`
	IncompleteDetails struct {
		Reason string `json:"reason"`
	} `json:"incomplete_details"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (r rResp) stopReason(sawTool bool) string {
	switch {
	case sawTool:
		return "tool_use"
	case r.IncompleteDetails.Reason == "max_output_tokens":
		return "max_tokens"
	}
	return "end_turn"
}

// ReplyToAnthropic turns a non-streamed Responses body into an Anthropic message.
func ReplyToAnthropic(body []byte, model string) ([]byte, error) {
	var r rResp
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("invalid upstream reply: %w", err)
	}
	content := []any{}
	sawTool := false
	for _, it := range r.Output {
		switch it.Type {
		case "message":
			for _, c := range it.Content {
				if t := c.Text + c.Refusal; t != "" {
					content = append(content, map[string]any{"type": "text", "text": t})
				}
			}
		case "function_call":
			sawTool = true
			input := json.RawMessage(it.Arguments)
			if !json.Valid(input) {
				input = json.RawMessage("{}")
			}
			content = append(content, map[string]any{
				"type": "tool_use", "id": it.CallID, "name": it.Name, "input": input,
			})
		}
	}
	return json.Marshal(map[string]any{
		"id": "msg_" + r.ID, "type": "message", "role": "assistant", "model": model,
		"content": content, "stop_reason": r.stopReason(sawTool), "stop_sequence": nil,
		"usage": r.Usage.anthropic(),
	})
}

// ErrorToAnthropic wraps an upstream error body in the Anthropic error shape.
func ErrorToAnthropic(status int, body []byte) []byte {
	msg := strings.TrimSpace(string(body))
	var e struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &e) == nil && e.Error.Message != "" {
		msg = e.Error.Message
	}
	if len(msg) > 2000 {
		msg = msg[:2000]
	}
	typ := "api_error"
	switch {
	case status == 400 || status == 413 || status == 422:
		typ = "invalid_request_error"
	case status == 401:
		typ = "authentication_error"
	case status == 403:
		typ = "permission_error"
	case status == 404:
		typ = "not_found_error"
	case status == 429:
		typ = "rate_limit_error"
	}
	b, _ := json.Marshal(map[string]any{
		"type": "error", "error": map[string]string{"type": typ, "message": msg},
	})
	return b
}

type block struct {
	idx  int
	tool bool
	open bool
	sent bool // tool block: any argument bytes sent
}

type streamer struct {
	w       io.Writer
	flush   func()
	model   string
	started bool
	done    bool
	next    int
	sawTool bool
	blocks  map[int]*block // by Responses output_index
}

// StreamToAnthropic reads Responses server-sent events and writes the
// equivalent Anthropic events, flushing after each one.
func StreamToAnthropic(r io.Reader, w io.Writer, flush func(), model string) error {
	s := &streamer{w: w, flush: flush, model: model, blocks: map[int]*block{}}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), 16<<20)
	for sc.Scan() && !s.done {
		line := sc.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		if err := s.handle([]byte(data)); err != nil {
			return err
		}
	}
	if err := sc.Err(); err != nil && !s.done {
		s.fail("upstream stream broke: " + err.Error())
		return err
	}
	if !s.done {
		s.fail("upstream stream ended before the reply was complete")
	}
	return nil
}

func (s *streamer) emit(event string, v any) {
	b, _ := json.Marshal(v)
	fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", event, b)
	s.flush()
}

func (s *streamer) start(id string) {
	if s.started {
		return
	}
	s.started = true
	s.emit("message_start", map[string]any{"type": "message_start", "message": map[string]any{
		"id": "msg_" + id, "type": "message", "role": "assistant", "model": s.model,
		"content": []any{}, "stop_reason": nil, "stop_sequence": nil,
		"usage": map[string]int{"input_tokens": 0, "output_tokens": 0},
	}})
}

func (s *streamer) open(outIdx int, content map[string]any, tool bool) *block {
	b := &block{idx: s.next, tool: tool, open: true}
	s.next++
	s.blocks[outIdx] = b
	s.emit("content_block_start", map[string]any{"type": "content_block_start", "index": b.idx, "content_block": content})
	return b
}

func (s *streamer) close(b *block) {
	if b != nil && b.open {
		b.open = false
		s.emit("content_block_stop", map[string]any{"type": "content_block_stop", "index": b.idx})
	}
}

func (s *streamer) fail(msg string) {
	s.done = true
	s.start("err")
	s.emit("error", map[string]any{"type": "error", "error": map[string]string{"type": "api_error", "message": "tsunagi: " + msg}})
}

func (s *streamer) handle(data []byte) error {
	var ev struct {
		Type        string `json:"type"`
		OutputIndex int    `json:"output_index"`
		Delta       string `json:"delta"`
		Message     string `json:"message"`
		Item        rItem  `json:"item"`
		Response    rResp  `json:"response"`
	}
	if err := json.Unmarshal(data, &ev); err != nil {
		return nil // ignore a line we cannot read
	}
	switch ev.Type {
	case "response.created":
		s.start(ev.Response.ID)

	case "response.output_item.added":
		s.start("")
		if ev.Item.Type == "function_call" {
			s.sawTool = true
			s.open(ev.OutputIndex, map[string]any{
				"type": "tool_use", "id": ev.Item.CallID, "name": ev.Item.Name, "input": map[string]any{},
			}, true)
		}

	case "response.output_text.delta", "response.refusal.delta":
		s.start("")
		b := s.blocks[ev.OutputIndex]
		if b == nil {
			b = s.open(ev.OutputIndex, map[string]any{"type": "text", "text": ""}, false)
		}
		s.emit("content_block_delta", map[string]any{"type": "content_block_delta", "index": b.idx,
			"delta": map[string]any{"type": "text_delta", "text": ev.Delta}})

	case "response.function_call_arguments.delta":
		if b := s.blocks[ev.OutputIndex]; b != nil && b.tool {
			b.sent = true
			s.emit("content_block_delta", map[string]any{"type": "content_block_delta", "index": b.idx,
				"delta": map[string]any{"type": "input_json_delta", "partial_json": ev.Delta}})
		}

	case "response.output_item.done":
		b := s.blocks[ev.OutputIndex]
		if b != nil && b.tool && !b.sent && ev.Item.Arguments != "" {
			b.sent = true
			s.emit("content_block_delta", map[string]any{"type": "content_block_delta", "index": b.idx,
				"delta": map[string]any{"type": "input_json_delta", "partial_json": ev.Item.Arguments}})
		}
		s.close(b)

	case "response.completed", "response.incomplete":
		s.start(ev.Response.ID)
		for _, b := range s.blocks {
			s.close(b)
		}
		s.emit("message_delta", map[string]any{"type": "message_delta",
			"delta": map[string]any{"stop_reason": ev.Response.stopReason(s.sawTool), "stop_sequence": nil},
			"usage": ev.Response.Usage.anthropic()})
		s.emit("message_stop", map[string]any{"type": "message_stop"})
		s.done = true

	case "response.failed":
		msg := "upstream reported a failed response"
		if e := ev.Response.Error; e != nil && e.Message != "" {
			msg = e.Message
		}
		s.fail(msg)

	case "error":
		msg := ev.Message
		if msg == "" {
			msg = "upstream error"
		}
		s.fail(msg)
	}
	return nil
}
