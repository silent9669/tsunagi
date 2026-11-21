package free

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const orFixture = `{"data":[
	{"id":"qwen/qwen3-coder:free","context_length":262144,
	 "pricing":{"prompt":"0","completion":"0"},
	 "supported_parameters":["tools","reasoning","temperature"]},
	{"id":"openai/gpt-4o","context_length":128000,
	 "pricing":{"prompt":"0.0000025","completion":"0.00001"},
	 "supported_parameters":["tools"]},
	{"id":"openrouter/free","context_length":200000,
	 "pricing":{"prompt":"0","completion":"0"},
	 "supported_parameters":["tools"]},
	{"id":"cheap/half-free","context_length":8000,
	 "pricing":{"prompt":"0","completion":"0.000001"},
	 "supported_parameters":["tools"]},
	{"id":"mystery/model","context_length":32000,
	 "pricing":{"prompt":"0","completion":"0"}}
]}`

const zenFixture = `{"data":[
	{"id":"big-pickle","limit":{"context":200000},
	 "capabilities":{"tools":{"supported":true},"reasoning":{"supported":true}},
	 "cost":{"text":{"input":{"price":0},"output":{"price":0}}}},
	{"id":"gpt-5","limit":{"context":400000},
	 "capabilities":{"tools":{"supported":true},"reasoning":{"supported":false}},
	 "cost":{"text":{"input":{"price":0.000005},"output":{"price":0.00003}}}},
	{"id":"nemotron","limit":{"context":128000},
	 "capabilities":{"tools":{"supported":false},"reasoning":{"supported":false}},
	 "cost":{"text":{"input":{"price":0},"output":{"price":0}}}}
]}`

func TestParseOpenRouter(t *testing.T) {
	ms, err := parseOpenRouter([]byte(orFixture))
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) != 3 {
		t.Fatalf("models = %+v", ms)
	}
	byID := map[string]FreeModel{}
	for _, m := range ms {
		byID[m.Provider+"/"+m.ID] = m
	}
	q := byID["openrouter/qwen/qwen3-coder:free"]
	if q.Context != 262144 || q.Tools == nil || !*q.Tools || !q.Reasoning {
		t.Fatalf("qwen = %+v", q)
	}
	if _, ok := byID["openrouter/openai/gpt-4o"]; ok {
		t.Fatal("priced model included")
	}
	if _, ok := byID["openrouter/cheap/half-free"]; ok {
		t.Fatal("half-priced model included")
	}
	if m := byID["openrouter/openrouter/free"]; m.Tools == nil || !*m.Tools {
		t.Fatalf("openrouter/free tools = %+v", m.Tools)
	}
	// no supported_parameters at all means tools is unknown, not "no"
	if m := byID["openrouter/mystery/model"]; m.Tools != nil {
		t.Fatalf("mystery tools = %+v, want nil", *m.Tools)
	}
}

func TestParseZen(t *testing.T) {
	ms, err := parseZen([]byte(zenFixture))
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) != 2 {
		t.Fatalf("models = %+v", ms)
	}
	byID := map[string]FreeModel{}
	for _, m := range ms {
		byID[m.ID] = m
	}
	if m := byID["big-pickle"]; m.Provider != "opencode" || m.Context != 200000 ||
		m.Tools == nil || !*m.Tools || !m.Reasoning {
		t.Fatalf("big-pickle = %+v", m)
	}
	if m := byID["nemotron"]; m.Tools == nil || *m.Tools {
		t.Fatalf("nemotron tools = %+v, want known false", m.Tools)
	}
	if _, ok := byID["gpt-5"]; ok {
		t.Fatal("priced model included")
	}
}

// cacheHome points the user cache dir at a temp HOME for the test.
func cacheHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "xdg"))
	return home
}

func fakeLists(t *testing.T, hits *int) (orURL, zenURL string) {
	t.Helper()
	or := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*hits++
		w.Write([]byte(orFixture))
	}))
	zen := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*hits++
		w.Write([]byte(zenFixture))
	}))
	t.Cleanup(or.Close)
	t.Cleanup(zen.Close)
	return or.URL, zen.URL
}

func useURLs(t *testing.T, orURL, zenURL string) {
	t.Helper()
	oldOR, oldZen := OpenRouterURL, ZenURL
	OpenRouterURL, ZenURL = orURL, zenURL
	t.Cleanup(func() { OpenRouterURL, ZenURL = oldOR, oldZen })
}

func TestRefreshWritesAtomicCache(t *testing.T) {
	cacheHome(t)
	var hits int
	orURL, zenURL := fakeLists(t, &hits)
	useURLs(t, orURL, zenURL)

	if err := Refresh(context.Background(), http.DefaultClient); err != nil {
		t.Fatal(err)
	}
	if hits != 2 {
		t.Fatalf("hits = %d", hits)
	}
	path := Path()
	if st, err := os.Stat(path); err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("cache mode = %v %v", st.Mode().Perm(), err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("temp file left behind; write is not atomic")
	}
	snap, ok := Load()
	if !ok || len(snap.Models) != 5 || snap.FetchedAt.IsZero() {
		t.Fatalf("snapshot = %+v ok=%v", snap, ok)
	}
	// sorted by provider then id: opencode's two models first, openrouter last
	if snap.Models[0].Provider != "opencode" || snap.Models[len(snap.Models)-1].Provider != "openrouter" {
		t.Fatalf("order = %+v", snap.Models)
	}
}

func TestEnsureUsesFreshCache(t *testing.T) {
	cacheHome(t)
	var hits int
	orURL, zenURL := fakeLists(t, &hits)
	useURLs(t, orURL, zenURL)

	// a fresh cache is used without any fetch
	path := Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	fresh := Snapshot{FetchedAt: time.Now().Add(-time.Hour), Models: []FreeModel{{Provider: "openrouter", ID: "x/y:free"}}}
	b, _ := json.Marshal(fresh)
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	snap, offline := Ensure(context.Background(), http.DefaultClient)
	if offline || len(snap.Models) != 1 || snap.Models[0].ID != "x/y:free" || hits != 0 {
		t.Fatalf("snap=%+v offline=%v hits=%d", snap, offline, hits)
	}
}

func TestEnsureRefreshesStaleAndFallsBackOffline(t *testing.T) {
	cacheHome(t)
	var hits int
	orURL, zenURL := fakeLists(t, &hits)
	useURLs(t, orURL, zenURL)

	path := Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	stale := Snapshot{FetchedAt: time.Now().Add(-7 * time.Hour), Models: []FreeModel{{Provider: "openrouter", ID: "old/model:free"}}}
	b, _ := json.Marshal(stale)
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	snap, offline := Ensure(context.Background(), http.DefaultClient)
	if offline || hits != 2 || len(snap.Models) != 5 {
		t.Fatalf("snap=%+v offline=%v hits=%d", snap, offline, hits)
	}

	// with the lists unreachable a stale cache still comes back, marked
	// offline, instead of nothing at all
	stale = Snapshot{FetchedAt: time.Now().Add(-7 * time.Hour), Models: []FreeModel{{Provider: "openrouter", ID: "old/model:free"}}}
	b, _ = json.Marshal(stale)
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	useURLs(t, "http://127.0.0.1:1/or", "http://127.0.0.1:1/zen")
	snap, offline = Ensure(context.Background(), http.DefaultClient)
	if !offline || len(snap.Models) != 1 || snap.Models[0].ID != "old/model:free" {
		t.Fatalf("offline snap=%+v offline=%v", snap, offline)
	}
}

func TestRefreshForcesFetch(t *testing.T) {
	cacheHome(t)
	var hits int
	orURL, zenURL := fakeLists(t, &hits)
	useURLs(t, orURL, zenURL)
	if err := Refresh(context.Background(), http.DefaultClient); err != nil {
		t.Fatal(err)
	}
	if err := Refresh(context.Background(), http.DefaultClient); err != nil {
		t.Fatal(err)
	}
	if hits != 4 {
		t.Fatalf("refresh did not force: hits=%d", hits)
	}
}

func TestProbeCacheRoundTrip(t *testing.T) {
	cacheHome(t)
	if got := LoadProbes(); len(got) != 0 {
		t.Fatalf("empty cache = %v", got)
	}
	results := map[string]Result{
		"opencode/big-pickle": {Target: "opencode/big-pickle", OK: true, Tools: true, Millis: 812, At: time.Now()},
		"openrouter/x/y:free": {Target: "openrouter/x/y:free", OK: false, Err: "HTTP 429", At: time.Now()},
	}
	if err := SaveProbes(results); err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Stat(ProbePath()); st.Mode().Perm() != 0o600 {
		t.Fatalf("probe cache mode = %o", st.Mode().Perm())
	}
	got := LoadProbes()
	if len(got) != 2 || !got["opencode/big-pickle"].OK || got["openrouter/x/y:free"].Err != "HTTP 429" {
		t.Fatalf("probes = %+v", got)
	}
}
