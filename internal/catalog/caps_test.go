package catalog

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

const capsSample = `{
 "acme":{"name":"Acme","api":"https://llm.acme.example/v1","npm":"@ai-sdk/openai-compatible","models":{
   "chat":{"tool_call":true,"limit":{"context":128000,"output":8192},"cost":{"input":0.5,"output":1.5}},
   "plain":{"tool_call":false,"limit":{"context":32000}},
   "bare":{}
 }},
 "oa":{"name":"OA","api":"https://oa.example/v1","npm":"@ai-sdk/openai","models":{"r":{"tool_call":true,"limit":{"context":400000}}}}
}`

func TestParseCapabilities(t *testing.T) {
	es, err := Parse([]byte(capsSample))
	if err != nil {
		t.Fatal(err)
	}
	var acme *Entry
	for i := range es {
		if es[i].ID == "acme" {
			acme = &es[i]
		}
	}
	if acme == nil {
		t.Fatal("acme missing")
	}
	mi, ok := acme.Caps["chat"]
	if !ok || !mi.Tools || mi.Context != 128000 || mi.Output != 8192 {
		t.Fatalf("chat caps = %+v ok=%v", mi, ok)
	}
	if mi.In == nil || *mi.In != 0.5 || mi.Out == nil || *mi.Out != 1.5 {
		t.Fatalf("chat cost = %+v", mi)
	}
	if mi, ok := acme.Caps["plain"]; !ok || mi.Tools || mi.Context != 32000 || mi.In != nil {
		t.Fatalf("plain caps = %+v ok=%v", mi, ok)
	}
	if _, ok := acme.Caps["bare"]; ok {
		t.Fatal("a model with no capability data should have no caps entry")
	}
	if len(acme.Models) != 3 {
		t.Fatalf("models = %v", acme.Models)
	}
}

func TestOldCacheWithoutCapsLoads(t *testing.T) {
	p := filepath.Join(t.TempDir(), "models.json")
	if err := os.WriteFile(p, []byte(`{"providers":[{"id":"old","name":"Old","api":"https://old.example/v1","dialect":"openai-chat","models":["m"]}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	es := LoadFrom(p)
	if len(es) != 1 || es[0].Caps != nil {
		t.Fatalf("entries = %+v", es)
	}
}

// TestCapsLookup is the only caller of Caps in this package: the catalogue
// loads once per process, so every assertion shares one cache.
func TestCapsLookup(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(capsSample)) }))
	t.Cleanup(srv.Close)
	if _, err := Sync(srv.Client(), srv.URL); err != nil {
		t.Fatal(err)
	}
	mi, ok := Caps("acme", "chat")
	if !ok || !mi.Tools || mi.Context != 128000 || mi.Output != 8192 {
		t.Fatalf("caps = %+v ok=%v", mi, ok)
	}
	if mi.In == nil || *mi.In != 0.5 {
		t.Fatalf("cost = %+v", mi)
	}
	if _, ok := Caps("acme", "missing"); ok {
		t.Fatal("unknown model reported caps")
	}
	if _, ok := Caps("missing", "chat"); ok {
		t.Fatal("unknown provider reported caps")
	}
}
