package translate

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func decode(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("bad json %s: %v", b, err)
	}
	return m
}

func TestRequestMapsMessagesToolsAndDropsThinking(t *testing.T) {
	in := `{
	 "model":"openai/gpt-x","max_tokens":100,"stream":true,
	 "thinking":{"type":"enabled","budget_tokens":2000},
	 "system":[{"type":"text","text":"be brief","cache_control":{"type":"ephemeral"}},{"type":"text","text":"be kind"}],
	 "tools":[{"name":"Read","description":"read","input_schema":{"type":"object"}},
	          {"type":"web_search_20250305","name":"web_search"}],
	 "tool_choice":{"type":"any"},
	 "messages":[
	  {"role":"user","content":"hi"},
	  {"role":"assistant","content":[
	    {"type":"thinking","thinking":"hmm","signature":"abc"},
	    {"type":"text","text":"reading"},
	    {"type":"tool_use","id":"toolu_1","name":"Read","input":{"path":"a"}}]},
	  {"role":"user","content":[
	    {"type":"tool_result","tool_use_id":"toolu_1","content":[{"type":"text","text":"file body"}]},
	    {"type":"text","text":"now what"}]}]}`
	out, stream, err := ToResponses([]byte(in), "gpt-x")
	if err != nil || !stream {
		t.Fatalf("err=%v stream=%v", err, stream)
	}
	m := decode(t, out)
	if m["model"] != "gpt-x" || m["store"] != false || m["instructions"] != "be brief\n\nbe kind" {
		t.Fatalf("head fields wrong: %s", out)
	}
	if m["max_output_tokens"].(float64) != 100 || m["tool_choice"] != "required" {
		t.Fatalf("limits/tool_choice wrong: %s", out)
	}
	if tools := m["tools"].([]any); len(tools) != 1 || tools[0].(map[string]any)["name"] != "Read" {
		t.Fatalf("server tool should be dropped: %s", out)
	}
	if bytes.Contains(out, []byte("signature")) || bytes.Contains(out, []byte("hmm")) {
		t.Fatalf("thinking leaked: %s", out)
	}
	items := m["input"].([]any)
	var kinds []string
	for _, it := range items {
		im := it.(map[string]any)
		if k, ok := im["type"].(string); ok {
			kinds = append(kinds, k)
		} else {
			kinds = append(kinds, im["role"].(string))
		}
	}
	want := "user,assistant,function_call,function_call_output,user"
	if strings.Join(kinds, ",") != want {
		t.Fatalf("items = %v, want %s", kinds, want)
	}
	fc := items[2].(map[string]any)
	if fc["call_id"] != "toolu_1" || fc["arguments"] != `{"path":"a"}` {
		t.Fatalf("function_call = %v", fc)
	}
	if items[3].(map[string]any)["output"] != "file body" {
		t.Fatalf("tool output = %v", items[3])
	}
	if items[1].(map[string]any)["content"].([]any)[0].(map[string]any)["type"] != "output_text" {
		t.Fatalf("assistant text must be output_text: %v", items[1])
	}
}

func sse(events ...string) string {
	var b strings.Builder
	for _, e := range events {
		b.WriteString("event: x\ndata: " + e + "\n\n")
	}
	return b.String()
}

func runStream(t *testing.T, in string) []map[string]any {
	t.Helper()
	var out bytes.Buffer
	if err := StreamToAnthropic(strings.NewReader(in), &out, func() {}, "gpt-x"); err != nil {
		t.Fatal(err)
	}
	var evs []map[string]any
	for _, blk := range strings.Split(strings.TrimSpace(out.String()), "\n\n") {
		lines := strings.SplitN(blk, "\n", 2)
		if len(lines) != 2 || !strings.HasPrefix(lines[0], "event: ") {
			t.Fatalf("bad frame %q", blk)
		}
		m := decode(t, []byte(strings.TrimPrefix(lines[1], "data: ")))
		if m["type"] != strings.TrimPrefix(lines[0], "event: ") {
			t.Fatalf("event name %q != type %v", lines[0], m["type"])
		}
		evs = append(evs, m)
	}
	return evs
}

func types(evs []map[string]any) string {
	var s []string
	for _, e := range evs {
		s = append(s, e["type"].(string))
	}
	return strings.Join(s, ",")
}

func TestStreamTextThenToolCall(t *testing.T) {
	evs := runStream(t, sse(
		`{"type":"response.created","response":{"id":"r1"}}`,
		`{"type":"response.output_item.added","output_index":0,"item":{"type":"reasoning"}}`,
		`{"type":"response.output_item.done","output_index":0,"item":{"type":"reasoning"}}`,
		`{"type":"response.output_item.added","output_index":1,"item":{"type":"message"}}`,
		`{"type":"response.output_text.delta","output_index":1,"delta":"Hel"}`,
		`{"type":"response.output_text.delta","output_index":1,"delta":"lo"}`,
		`{"type":"response.output_item.done","output_index":1,"item":{"type":"message"}}`,
		`{"type":"response.output_item.added","output_index":2,"item":{"type":"function_call","call_id":"c1","name":"Read"}}`,
		`{"type":"response.function_call_arguments.delta","output_index":2,"delta":"{\"p\":"}`,
		`{"type":"response.function_call_arguments.delta","output_index":2,"delta":"1}"}`,
		`{"type":"response.output_item.done","output_index":2,"item":{"type":"function_call","arguments":"{\"p\":1}"}}`,
		`{"type":"response.completed","response":{"id":"r1","usage":{"input_tokens":50,"output_tokens":7,"input_tokens_details":{"cached_tokens":20}}}}`,
	))
	want := "message_start,content_block_start,content_block_delta,content_block_delta,content_block_stop," +
		"content_block_start,content_block_delta,content_block_delta,content_block_stop,message_delta,message_stop"
	if types(evs) != want {
		t.Fatalf("events:\n got %s\nwant %s", types(evs), want)
	}
	if evs[1]["index"].(float64) != 0 || evs[5]["index"].(float64) != 1 {
		t.Fatalf("block indexes must be dense (reasoning skipped): %v %v", evs[1], evs[5])
	}
	tool := evs[5]["content_block"].(map[string]any)
	if tool["type"] != "tool_use" || tool["id"] != "c1" || tool["name"] != "Read" {
		t.Fatalf("tool block = %v", tool)
	}
	md := evs[9]
	if md["delta"].(map[string]any)["stop_reason"] != "tool_use" {
		t.Fatalf("stop_reason = %v", md)
	}
	u := md["usage"].(map[string]any)
	if u["input_tokens"].(float64) != 30 || u["cache_read_input_tokens"].(float64) != 20 || u["output_tokens"].(float64) != 7 {
		t.Fatalf("usage = %v", u)
	}
}

func TestStreamToolArgumentsOnlyInDoneEvent(t *testing.T) {
	evs := runStream(t, sse(
		`{"type":"response.created","response":{"id":"r"}}`,
		`{"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","call_id":"c","name":"X"}}`,
		`{"type":"response.output_item.done","output_index":0,"item":{"type":"function_call","arguments":"{\"a\":2}"}}`,
		`{"type":"response.completed","response":{"id":"r"}}`,
	))
	if got := evs[2]["delta"].(map[string]any)["partial_json"]; got != `{"a":2}` {
		t.Fatalf("partial_json = %v", got)
	}
}

func TestStreamIncompleteMaxTokens(t *testing.T) {
	evs := runStream(t, sse(
		`{"type":"response.created","response":{"id":"r"}}`,
		`{"type":"response.output_text.delta","output_index":0,"delta":"x"}`,
		`{"type":"response.incomplete","response":{"id":"r","incomplete_details":{"reason":"max_output_tokens"}}}`,
	))
	last := evs[len(evs)-2]
	if last["delta"].(map[string]any)["stop_reason"] != "max_tokens" {
		t.Fatalf("got %v", last)
	}
}

func TestStreamFailureAndEarlyEndBecomeErrorEvents(t *testing.T) {
	for name, in := range map[string]string{
		"failed": sse(`{"type":"response.created","response":{"id":"r"}}`,
			`{"type":"response.failed","response":{"error":{"message":"quota gone"}}}`),
		"cut": sse(`{"type":"response.created","response":{"id":"r"}}`,
			`{"type":"response.output_text.delta","output_index":0,"delta":"x"}`),
	} {
		evs := runStream(t, in)
		if last := evs[len(evs)-1]; last["type"] != "error" {
			t.Errorf("%s: last event = %v", name, last)
		}
	}
	evs := runStream(t, sse(`{"type":"response.failed","response":{"error":{"message":"quota gone"}}}`))
	if !strings.Contains(evs[len(evs)-1]["error"].(map[string]any)["message"].(string), "quota gone") {
		t.Fatalf("message lost: %v", evs)
	}
}

func TestReplyToAnthropic(t *testing.T) {
	in := `{"id":"r9","output":[
	  {"type":"reasoning"},
	  {"type":"message","content":[{"type":"output_text","text":"done"}]},
	  {"type":"function_call","call_id":"c","name":"Bash","arguments":"{\"cmd\":\"ls\"}"}],
	  "usage":{"input_tokens":4,"output_tokens":2}}`
	b, err := ReplyToAnthropic([]byte(in), "gpt-x")
	if err != nil {
		t.Fatal(err)
	}
	m := decode(t, b)
	c := m["content"].([]any)
	if m["stop_reason"] != "tool_use" || len(c) != 2 || c[1].(map[string]any)["input"].(map[string]any)["cmd"] != "ls" {
		t.Fatalf("got %s", b)
	}
}

func TestErrorToAnthropic(t *testing.T) {
	b := ErrorToAnthropic(429, []byte(`{"error":{"message":"slow down","type":"x"}}`))
	m := decode(t, b)
	e := m["error"].(map[string]any)
	if m["type"] != "error" || e["type"] != "rate_limit_error" || e["message"] != "slow down" {
		t.Fatalf("got %s", b)
	}
	// plain-text upstream bodies survive too
	if !strings.Contains(string(ErrorToAnthropic(502, []byte("bad gateway"))), "bad gateway") {
		t.Fatal("plain body lost")
	}
}

func TestChatRequest(t *testing.T) {
	in := `{"model":"x","max_tokens":50,"stream":true,"temperature":0.2,"stop_sequences":["END"],
	 "system":"sys",
	 "tools":[{"name":"Read","description":"d","input_schema":{"type":"object"}}],
	 "tool_choice":{"type":"tool","name":"Read"},
	 "messages":[
	  {"role":"user","content":"hi"},
	  {"role":"assistant","content":[{"type":"thinking","thinking":"hm","signature":"s"},
	    {"type":"text","text":"ok"},{"type":"tool_use","id":"t1","name":"Read","input":{"p":1}}]},
	  {"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"body"},{"type":"text","text":"next"}]}]}`
	out, stream, err := ToChat([]byte(in), "llama-x")
	if err != nil || !stream {
		t.Fatalf("err=%v stream=%v", err, stream)
	}
	if bytes.Contains(out, []byte("signature")) {
		t.Fatalf("thinking leaked: %s", out)
	}
	m := decode(t, out)
	if m["model"] != "llama-x" || m["max_tokens"].(float64) != 50 || m["temperature"].(float64) != 0.2 {
		t.Fatalf("head = %s", out)
	}
	if m["stream_options"].(map[string]any)["include_usage"] != true {
		t.Fatal("usage not requested")
	}
	var roles []string
	for _, x := range m["messages"].([]any) {
		roles = append(roles, x.(map[string]any)["role"].(string))
	}
	if got := strings.Join(roles, ","); got != "system,user,assistant,tool,user" {
		t.Fatalf("roles = %s", got)
	}
	asst := m["messages"].([]any)[2].(map[string]any)
	call := asst["tool_calls"].([]any)[0].(map[string]any)
	if asst["content"] != "ok" || call["id"] != "t1" || call["function"].(map[string]any)["arguments"] != `{"p":1}` {
		t.Fatalf("assistant = %v", asst)
	}
	tc := m["tool_choice"].(map[string]any)["function"].(map[string]any)["name"]
	if tc != "Read" {
		t.Fatalf("tool_choice = %v", m["tool_choice"])
	}
}

func chatSSE(chunks ...string) string {
	var b strings.Builder
	for _, c := range chunks {
		b.WriteString("data: " + c + "\n\n")
	}
	return b.String()
}

func runChat(t *testing.T, in string) []map[string]any {
	t.Helper()
	var out bytes.Buffer
	if err := ChatStreamToAnthropic(strings.NewReader(in), &out, func() {}, "m"); err != nil {
		t.Fatal(err)
	}
	var evs []map[string]any
	for _, blk := range strings.Split(strings.TrimSpace(out.String()), "\n\n") {
		lines := strings.SplitN(blk, "\n", 2)
		evs = append(evs, decode(t, []byte(strings.TrimPrefix(lines[1], "data: "))))
	}
	return evs
}

func TestChatStreamTextThenParallelToolCalls(t *testing.T) {
	evs := runChat(t, chatSSE(
		`{"id":"c","choices":[{"delta":{"role":"assistant","content":"Hi"}}]}`,
		`{"id":"c","choices":[{"delta":{"content":" there"}}]}`,
		`{"id":"c","choices":[{"delta":{"tool_calls":[{"index":0,"id":"a","type":"function","function":{"name":"Read","arguments":""}}]}}]}`,
		`{"id":"c","choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"p\":"}}]}}]}`,
		`{"id":"c","choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"1}"}}]}}]}`,
		`{"id":"c","choices":[{"delta":{"tool_calls":[{"index":1,"id":"b","type":"function","function":{"name":"Bash","arguments":"{}"}}]}}]}`,
		`{"id":"c","choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		`{"id":"c","choices":[],"usage":{"prompt_tokens":10,"completion_tokens":4,"prompt_tokens_details":{"cached_tokens":6}}}`,
		`[DONE]`,
	))
	want := "message_start,content_block_start,content_block_delta,content_block_delta,content_block_stop," +
		"content_block_start,content_block_delta,content_block_delta,content_block_stop," +
		"content_block_start,content_block_delta,content_block_stop,message_delta,message_stop"
	if types(evs) != want {
		t.Fatalf("events:\n got %s\nwant %s", types(evs), want)
	}
	md := evs[len(evs)-2]
	u := md["usage"].(map[string]any)
	if md["delta"].(map[string]any)["stop_reason"] != "tool_use" || u["input_tokens"].(float64) != 4 || u["cache_read_input_tokens"].(float64) != 6 {
		t.Fatalf("message_delta = %v", md)
	}
}

func TestChatStreamWithoutDoneAndStopWithToolsAndErrors(t *testing.T) {
	// no [DONE], finish_reason "stop" but tools were called -> tool_use
	evs := runChat(t, chatSSE(
		`{"id":"c","choices":[{"delta":{"tool_calls":[{"id":"a","function":{"name":"X","arguments":"{}"}}]},"finish_reason":"stop"}]}`,
	))
	if evs[len(evs)-2]["delta"].(map[string]any)["stop_reason"] != "tool_use" {
		t.Fatalf("got %v", evs)
	}
	// error chunk
	evs = runChat(t, chatSSE(`{"error":{"message":"bad key"}}`))
	if evs[len(evs)-1]["type"] != "error" {
		t.Fatalf("got %v", evs)
	}
	// cut stream
	evs = runChat(t, "")
	if evs[len(evs)-1]["type"] != "error" {
		t.Fatalf("got %v", evs)
	}
}

func TestChatReplyToAnthropic(t *testing.T) {
	in := `{"id":"z","choices":[{"message":{"content":"yo","tool_calls":[{"id":"c","function":{"name":"Read","arguments":"{\"a\":1}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`
	b, err := ChatReplyToAnthropic([]byte(in), "m")
	if err != nil {
		t.Fatal(err)
	}
	m := decode(t, b)
	c := m["content"].([]any)
	if m["stop_reason"] != "tool_use" || len(c) != 2 || c[1].(map[string]any)["input"].(map[string]any)["a"].(float64) != 1 {
		t.Fatalf("got %s", b)
	}
}
