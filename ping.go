package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/silent9669/tsunagi/internal/config"
	"github.com/silent9669/tsunagi/internal/router"
	"github.com/silent9669/tsunagi/internal/ui"
)

// cmdPing sends one small request through the real router to a model and
// shows what came back. It is the quickest check that a key, an endpoint and
// a translator work. With --tool the request also offers a tool, to check
// tool calls.
func cmdPing(t *ui.Term, args []string) int {
	withTool := false
	var rest []string
	for _, a := range args {
		if a == "--tool" {
			withTool = true
		} else {
			rest = append(rest, a)
		}
	}
	if len(rest) != 1 {
		return fail(t, "usage: tsunagi ping [--tool] <provider/model>")
	}
	model := rest[0]
	cfg, err := config.Load()
	if err != nil {
		return fail(t, "%v", err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fail(t, "%v", err)
	}
	token, err := randomToken()
	if err != nil {
		return fail(t, "%v", err)
	}
	host := ln.Addr().String()
	rt := router.New(host, token, cfg.All(), cfg.Default, keyFor(cfg, newChatGPT()))
	rt.Aliases = cfg.Aliases
	srv := &http.Server{Handler: rt, ReadHeaderTimeout: 10 * time.Second}
	go srv.Serve(ln)
	defer srv.Close()

	req := map[string]any{
		"model": model, "max_tokens": 128, "stream": true,
		"messages": []any{map[string]any{"role": "user", "content": "Reply with the single word: ok"}},
	}
	if withTool {
		req["messages"] = []any{map[string]any{"role": "user", "content": "Call the echo tool with text set to hi."}}
		req["tools"] = []any{map[string]any{
			"name": "echo", "description": "Echo some text",
			"input_schema": map[string]any{"type": "object", "properties": map[string]any{"text": map[string]any{"type": "string"}}, "required": []string{"text"}},
		}}
	}
	body, _ := json.Marshal(req)

	sp := t.Spin(model)
	start := time.Now()
	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Post("http://"+host+"/"+token+"/v1/messages", "application/json", bytes.NewReader(body))
	if err != nil {
		sp.Stop(false, err.Error())
		return 1
	}
	defer resp.Body.Close()
	res := readReply(resp.Body)
	ms := time.Since(start).Milliseconds()
	if resp.StatusCode != 200 || res.err != "" {
		msg := res.err
		if msg == "" {
			msg = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		sp.Stop(false, fmt.Sprintf("%s (%d ms)", msg, ms))
		return 1
	}
	detail := fmt.Sprintf("%d ms, %d in, %d out, %s", ms, res.in, res.out, res.stop)
	sp.Stop(true, detail)
	if res.text != "" {
		t.Printf("  %s %s\n", t.Dim("text"), strings.TrimSpace(res.text))
	}
	for _, c := range res.tools {
		t.Printf("  %s %s\n", t.Dim("tool"), c)
	}
	return 0
}

type reply struct {
	text, stop, err string
	in, out         int
	tools           []string
}

type sseEvent struct {
	Type         string                      `json:"type"`
	ContentBlock struct{ Type, Name string } `json:"content_block"`
	Delta        struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		PartialJSON string `json:"partial_json"`
		StopReason  string `json:"stop_reason"`
	} `json:"delta"`
	Usage struct {
		In    int `json:"input_tokens"`
		Out   int `json:"output_tokens"`
		Cache int `json:"cache_read_input_tokens"`
	} `json:"usage"`
	Error struct{ Message string } `json:"error"`
}

// readReply collects a streamed Anthropic reply, or the error that came back
// instead.
func readReply(r io.Reader) reply {
	var res reply
	var names []string
	var args []*strings.Builder
	var raw bytes.Buffer
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), 16<<20)
	for sc.Scan() {
		line := sc.Text()
		raw.WriteString(line + "\n")
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var ev sseEvent
		if json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &ev) != nil {
			continue
		}
		switch ev.Type {
		case "content_block_start":
			if ev.ContentBlock.Type == "tool_use" {
				names = append(names, ev.ContentBlock.Name)
				args = append(args, &strings.Builder{})
			}
		case "content_block_delta":
			switch {
			case ev.Delta.Type == "text_delta":
				res.text += ev.Delta.Text
			case ev.Delta.Type == "input_json_delta" && len(args) > 0:
				args[len(args)-1].WriteString(ev.Delta.PartialJSON)
			}
		case "message_delta":
			res.stop = ev.Delta.StopReason
			res.in = ev.Usage.In + ev.Usage.Cache
			res.out = ev.Usage.Out
		case "error":
			res.err = ev.Error.Message
		}
	}
	for i, n := range names {
		res.tools = append(res.tools, n+" "+args[i].String())
	}
	if res.stop == "" && res.err == "" {
		var e sseEvent // a plain error body, not a stream
		if json.Unmarshal(raw.Bytes(), &e) == nil && e.Error.Message != "" {
			res.err = e.Error.Message
		} else {
			snippet := strings.TrimSpace(raw.String())
			if len(snippet) > 300 {
				snippet = snippet[:300]
			}
			res.err = "reply ended without a stop reason: " + strings.ReplaceAll(snippet, "\n", " ")
		}
	}
	return res
}
