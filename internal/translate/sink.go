package translate

import (
	"encoding/json"
	"fmt"
	"io"
)

type block struct {
	idx  int
	tool bool
	open bool
	sent bool // tool block: any argument bytes sent
}

var textBlock = map[string]any{"type": "text", "text": ""}

func toolBlock(id, name string) map[string]any {
	return map[string]any{"type": "tool_use", "id": id, "name": name, "input": map[string]any{}}
}

// sink writes Anthropic server-sent events. Both stream translators use it.
type sink struct {
	w       io.Writer
	flush   func()
	model   string
	started bool
	done    bool
	next    int
}

func (s *sink) emit(event string, v any) {
	b, _ := json.Marshal(v)
	fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", event, b)
	s.flush()
}

func (s *sink) start(id string) {
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

func (s *sink) openBlock(content map[string]any, tool bool) *block {
	s.start("")
	b := &block{idx: s.next, tool: tool, open: true}
	s.next++
	s.emit("content_block_start", map[string]any{"type": "content_block_start", "index": b.idx, "content_block": content})
	return b
}

func (s *sink) close(b *block) {
	if b != nil && b.open {
		b.open = false
		s.emit("content_block_stop", map[string]any{"type": "content_block_stop", "index": b.idx})
	}
}

func (s *sink) textDelta(b *block, text string) {
	s.emit("content_block_delta", map[string]any{"type": "content_block_delta", "index": b.idx,
		"delta": map[string]any{"type": "text_delta", "text": text}})
}

func (s *sink) argsDelta(b *block, partial string) {
	b.sent = true
	s.emit("content_block_delta", map[string]any{"type": "content_block_delta", "index": b.idx,
		"delta": map[string]any{"type": "input_json_delta", "partial_json": partial}})
}

func (s *sink) finish(stop string, usage map[string]int) {
	s.start("")
	s.emit("message_delta", map[string]any{"type": "message_delta",
		"delta": map[string]any{"stop_reason": stop, "stop_sequence": nil}, "usage": usage})
	s.emit("message_stop", map[string]any{"type": "message_stop"})
	s.done = true
}

func (s *sink) fail(msg string) {
	s.done = true
	s.start("err")
	s.emit("error", map[string]any{"type": "error", "error": map[string]string{"type": "api_error", "message": "tsunagi: " + msg}})
}
