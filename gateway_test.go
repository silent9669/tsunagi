package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/silent9669/tsunagi/internal/provider"
)

func TestParseAdd(t *testing.T) {
	p, err := parseAdd([]string{"gw", "http://127.0.0.1:4000/v1/", "--header", "X-Team: core", "--strategy", "round_robin"})
	if err != nil || p.Name != "gw" || p.BaseURL != "http://127.0.0.1:4000/v1" || p.Dialect != provider.OpenAIChat || !p.Local {
		t.Fatalf("local gateway: %+v %v", p, err)
	}
	if p.Headers["X-Team"] != "core" || p.Strategy != "round_robin" {
		t.Fatalf("options lost: %+v", p)
	}
	p, err = parseAdd([]string{"corp", "https://llm.corp.example/anthropic"})
	if err != nil || p.Dialect != provider.Anthropic || p.Local {
		t.Fatalf("remote anthropic gateway: %+v %v", p, err)
	}
	p, _ = parseAdd([]string{"lan", "http://10.0.0.5:8000/v1", "--local"})
	if !p.Local {
		t.Fatal("--local must stick for a LAN host")
	}
	for _, bad := range [][]string{
		{"gw"}, {"Bad Name", "http://x"}, {"gw", "ftp://x"}, {"gw", "http://u:p@x.example"}, {"gw", "http://x", "--dialect", "grpc"},
		{"gw", "http://x", "--header", "nocolon"}, {"gw", "http://x", "--bogus"}, {"gw", "http://x", "--strategy", "random"},
	} {
		if _, err := parseAdd(bad); err == nil {
			t.Errorf("%v accepted", bad)
		}
	}
}

func TestLiveModelsReadsOpenAIAndOllamaShapes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Team") != "core" || r.Header.Get("Authorization") != "Bearer k" {
			w.WriteHeader(401)
			return
		}
		switch r.URL.Path {
		case "/v1/models":
			w.Write([]byte(`{"data":[{"id":"a"},{"id":"b"}]}`))
		case "/native/models":
			w.Write([]byte(`{"models":[{"name":"llama3"}]}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	hdr := map[string]string{"X-Team": "core"}
	ms, err := liveModels(context.Background(), provider.Provider{BaseURL: srv.URL + "/v1", Dialect: provider.OpenAIChat, Headers: hdr}, "k")
	if err != nil || len(ms) != 2 || ms[0] != "a" {
		t.Fatalf("chat: %v %v", ms, err)
	}
	ms, err = liveModels(context.Background(), provider.Provider{BaseURL: srv.URL, Dialect: provider.Anthropic, Headers: hdr}, "k")
	if err != nil || len(ms) != 2 {
		t.Fatalf("anthropic path is /v1/models: %v %v", ms, err)
	}
	ms, _ = liveModels(context.Background(), provider.Provider{BaseURL: srv.URL + "/native", Dialect: provider.OpenAIChat, Headers: hdr}, "k")
	if len(ms) != 1 || ms[0] != "llama3" {
		t.Fatalf("native shape: %v", ms)
	}
	if _, err := liveModels(context.Background(), provider.Provider{BaseURL: srv.URL + "/v1", Dialect: provider.OpenAIChat}, "k"); err == nil {
		t.Fatal("a 401 must be an error")
	}
}
