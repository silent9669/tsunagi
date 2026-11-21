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

// cacheHome points the user cache dir at a temp HOME for the test.
func cacheHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "xdg"))
	return home
}

func fakeList(t *testing.T, hits *int) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*hits++
		w.Write([]byte(orFixture))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func useURL(t *testing.T, orURL string) {
	t.Helper()
	old := OpenRouterURL
	OpenRouterURL = orURL
	t.Cleanup(func() { OpenRouterURL = old })
}

func TestRefreshWritesAtomicCache(t *testing.T) {
	cacheHome(t)
	var hits int
	useURL(t, fakeList(t, &hits))

	if err := Refresh(context.Background(), http.DefaultClient); err != nil {
		t.Fatal(err)
	}
	if hits != 1 {
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
	if !ok || len(snap.Models) != 3 || snap.FetchedAt.IsZero() {
		t.Fatalf("snapshot = %+v ok=%v", snap, ok)
	}
	for _, m := range snap.Models {
		if m.Provider != "openrouter" {
			t.Fatalf("provider = %+v", m)
		}
	}
}

func TestEnsureUsesFreshCache(t *testing.T) {
	cacheHome(t)
	var hits int
	useURL(t, fakeList(t, &hits))

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
	useURL(t, fakeList(t, &hits))

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
	if offline || hits != 1 || len(snap.Models) != 3 {
		t.Fatalf("snap=%+v offline=%v hits=%d", snap, offline, hits)
	}

	// with the lists unreachable a stale cache still comes back, marked
	// offline, instead of nothing at all
	stale = Snapshot{FetchedAt: time.Now().Add(-7 * time.Hour), Models: []FreeModel{{Provider: "openrouter", ID: "old/model:free"}}}
	b, _ = json.Marshal(stale)
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	useURL(t, "http://127.0.0.1:1/or")
	snap, offline = Ensure(context.Background(), http.DefaultClient)
	if !offline || len(snap.Models) != 1 || snap.Models[0].ID != "old/model:free" {
		t.Fatalf("offline snap=%+v offline=%v", snap, offline)
	}
}

func TestRefreshForcesFetch(t *testing.T) {
	cacheHome(t)
	var hits int
	useURL(t, fakeList(t, &hits))
	if err := Refresh(context.Background(), http.DefaultClient); err != nil {
		t.Fatal(err)
	}
	if err := Refresh(context.Background(), http.DefaultClient); err != nil {
		t.Fatal(err)
	}
	if hits != 2 {
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
