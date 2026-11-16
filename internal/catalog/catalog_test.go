package catalog

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

const sample = `{
 "acme":{"name":"Acme","api":"https://llm.acme.example/v1","npm":"@ai-sdk/openai-compatible","env":["ACME_KEY"],"models":{"b":{},"a":{}}},
 "anthropic":{"name":"Anthropic","npm":"@ai-sdk/anthropic","models":{}},
 "kimi":{"name":"Kimi","api":"https://k.example/anthropic","npm":"@ai-sdk/anthropic","models":{}},
 "oa":{"name":"OA","api":"https://oa.example/v1","npm":"@ai-sdk/openai","models":{}},
 "cerebras":{"name":"Cerebras","npm":"@ai-sdk/cerebras","models":{}},
 "azure":{"name":"Azure","npm":"@ai-sdk/azure","api":"https://x","models":{}},
 "noapi":{"name":"No API","npm":"@ai-sdk/openai-compatible","models":{}}
}`

func TestParse(t *testing.T) {
	es, err := Parse([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]Entry{}
	for _, e := range es {
		got[e.ID] = e
	}
	if len(got) != 4 {
		t.Fatalf("want acme, kimi, oa, cerebras; got %v", got)
	}
	if got["acme"].Dialect != "openai-chat" || got["acme"].Models[0] != "a" {
		t.Fatalf("acme = %+v", got["acme"])
	}
	if got["kimi"].Dialect != "anthropic" || got["oa"].Dialect != "openai-responses" {
		t.Fatalf("dialects: %+v %+v", got["kimi"], got["oa"])
	}
	if got["cerebras"].API != "https://api.cerebras.ai/v1" {
		t.Fatalf("default api: %+v", got["cerebras"])
	}
}

func TestSyncWritesCacheThatLoads(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // UserCacheDir derives from HOME on macOS
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(sample)) }))
	defer srv.Close()
	n, err := Sync(srv.Client(), srv.URL)
	if err != nil || n != 4 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if len(Load()) != 4 {
		t.Fatalf("load = %v", Load())
	}
	if len(LoadFrom(filepath.Join(t.TempDir(), "none.json"))) != 0 {
		t.Fatal("missing cache must load empty")
	}
}
