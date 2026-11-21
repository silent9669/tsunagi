package router

import (
	"bytes"
	"encoding/json"
	"math"
	"math/rand/v2"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/silent9669/tsunagi/internal/provider"
)

// Combo is a named set of provider/model targets a model name can resolve to.
type Combo struct {
	Targets []string
	// Strategy orders the eligible targets: priority (default, as written),
	// fastest (lowest measured latency), round_robin, cheapest (lowest
	// catalogue price, unknown last), context (largest context window first),
	// least_used (fewest attempts so far), weighted (random draw favouring
	// higher weights) or headroom (most quota left first).
	Strategy string
	// Weights biases the weighted strategy; keyed by the exact target string,
	// a missing weight counts as 1.
	Weights map[string]int
}

// Caps is what the catalogue knows about one model, reduced to what combo
// routing needs.
type Caps struct {
	Tools   bool
	Context int
}

// Need describes what a request asks of its target.
type Need struct {
	Tools  bool
	Tokens int
	// Think means the request enables extended thinking, Web that it carries
	// a web_search tool, Input is its approximate prompt size in tokens.
	Think bool
	Web   bool
	Input int
}

// Step is one target's place in a Plan.
type Step struct {
	Target string `json:"target"`
	Status string `json:"status"` // ready, skip or circuit-open
	Reason string `json:"reason,omitempty"`
}

// Plan is what `tsunagi route` shows: how a model name resolves and which
// targets would be tried, in order.
type Plan struct {
	Resolved string `json:"resolved"`
	Why      string `json:"why,omitempty"`
	Combo    string `json:"combo,omitempty"`
	Steps    []Step `json:"steps"`
}

const (
	circuitFails = 3                // consecutive failures that open a circuit
	circuitBase  = 30 * time.Second // first cooldown; doubles on each re-open
	circuitMax   = 10 * time.Minute
)

// targetHealth tracks one combo target's recent upstream behavior.
type targetHealth struct {
	fails   int           // consecutive failures
	ewma    float64       // ms to response headers
	sampled bool          // ewma holds a real sample
	open    bool          // circuit is open
	since   time.Time     // when the current open period started
	backoff time.Duration // current cooldown
	probe   bool          // a half-open probe is in flight
}

// comboState is the router's combo runtime state: per-target health and
// per-combo round-robin cursors.
type comboState struct {
	mu     sync.Mutex
	health map[string]*targetHealth
	rr     map[string]int
	used   map[string]int // attempts per target, for least_used
}

func (rt *Router) clock() time.Time {
	if rt.now != nil {
		return rt.now()
	}
	return time.Now()
}

// nextRand draws the weighted strategy's uniform sample; tests set rand.
func (rt *Router) nextRand() float64 {
	if rt.rand != nil {
		return rt.rand()
	}
	return rand.Float64()
}

// resolve applies aliases and then tiers to a requested model name. It
// returns the resolved name and a short Why for the event, "" when the name
// is unchanged.
func (rt *Router) resolve(model string) (string, string) {
	if a, ok := rt.Aliases[model]; ok {
		return a, "alias " + model + " -> " + a
	}
	if strings.HasPrefix(model, "claude-") {
		for _, tier := range []string{"opus", "sonnet", "haiku"} {
			if strings.Contains(model, "-"+tier) {
				if v, ok := rt.Tiers[tier]; ok {
					return v, "tier " + tier + " -> " + v
				}
			}
		}
	}
	return model, ""
}

// resolveFor applies scenarios before the usual alias and tier resolution.
// A scenario only reroutes a requested claude-* model that no alias claims:
// long_context first, then web_search, then think, and a matching scenario
// with no configured target falls through to the next. A scenario always
// beats a tier mapping.
func (rt *Router) resolveFor(model string, need Need) (string, string) {
	if _, aliased := rt.Aliases[model]; !aliased && strings.HasPrefix(model, "claude-") {
		threshold := rt.ScenarioTokens
		if threshold <= 0 {
			threshold = 60000
		}
		checks := []struct {
			name string
			hit  bool
		}{
			{"long_context", need.Input > threshold},
			{"web_search", need.Web},
			{"think", need.Think},
		}
		for _, c := range checks {
			if c.hit && rt.Scenarios[c.name] != "" {
				return rt.Scenarios[c.name], "scenario " + c.name + " -> " + rt.Scenarios[c.name]
			}
		}
	}
	return rt.resolve(model)
}

// peekNeed estimates what a request body asks of its target: whether it
// carries tools, enables thinking or web search, and roughly how many
// tokens it will use.
func peekNeed(body []byte) Need {
	body = bytes.TrimSpace(body)
	if len(body) == 0 || body[0] != '{' {
		return Need{}
	}
	var m struct {
		Tools     []json.RawMessage `json:"tools"`
		Thinking  json.RawMessage   `json:"thinking"`
		MaxTokens json.Number       `json:"max_tokens"`
	}
	if json.Unmarshal(body, &m) != nil {
		return Need{}
	}
	n := Need{Tools: len(m.Tools) > 0, Tokens: len(body) / 3, Input: len(body) / 3}
	if f, err := m.MaxTokens.Float64(); err == nil {
		n.Tokens += int(f)
	}
	if len(m.Thinking) > 0 && string(m.Thinking) != "null" {
		var th struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(m.Thinking, &th) == nil && th.Type != "disabled" {
			n.Think = true
		}
	}
	for _, raw := range m.Tools {
		var tool struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(raw, &tool) == nil && strings.HasPrefix(tool.Type, "web_search") {
			n.Web = true
			break
		}
	}
	return n
}

// parseComboTarget splits "provider[@label]/model" into its parts.
func parseComboTarget(t string) (name, label, model string, ok bool) {
	i := strings.Index(t, "/")
	if i <= 0 || i == len(t)-1 {
		return "", "", "", false
	}
	name, label, _ = strings.Cut(t[:i], "@")
	model = t[i+1:]
	return name, label, model, name != "" && model != ""
}

// targetPlan is one combo target with its eligibility already decided.
type targetPlan struct {
	target string // as written in the combo
	name   string // provider
	label  string // pinned account label, if any
	model  string // model sent upstream
	hk     string // health key "provider/model", without @label
	skip   string // why it cannot serve, "" when eligible
}

// planTargets checks every target of a combo against the request. Plan uses
// the same function, so preview and routing cannot disagree.
func (rt *Router) planTargets(combo Combo, need Need) []targetPlan {
	var out []targetPlan
	for _, ts := range combo.Targets {
		tp := targetPlan{target: ts}
		name, label, m, ok := parseComboTarget(ts)
		if !ok {
			tp.skip = "invalid target"
			out = append(out, tp)
			continue
		}
		tp.name, tp.label, tp.model = name, label, m
		tp.hk = name + "/" + m
		p, exists := rt.Providers[name]
		switch {
		case !exists:
			tp.skip = "unknown provider"
		case !p.Ready():
			tp.skip = "provider not ready"
		case !rt.hasKey(p, label):
			tp.skip = "no key"
		default:
			if rt.Caps != nil {
				if c, ok := rt.Caps(name, m); ok {
					switch {
					case need.Tools && !c.Tools:
						tp.skip = "no tool calling"
					case c.Context > 0 && need.Tokens > c.Context:
						tp.skip = "context too large"
					}
				}
			}
		}
		out = append(out, tp)
	}
	return out
}

// hasKey reports whether any candidate account of p has a usable key or
// sign-in. It checks the same account set the request path uses, without
// spending the round-robin or headroom ordering on a lookup.
func (rt *Router) hasKey(p provider.Provider, label string) bool {
	if p.Local {
		return true
	}
	if rt.Key == nil {
		return false
	}
	for _, acct := range rt.accountNames(p, label) {
		if k, err := rt.Key(p, acct); err == nil && k != "" {
			return true
		}
	}
	return false
}

// accountNames lists a provider's accounts in preference order without the
// pool's rotation or blocking, which only matter when a request is sent.
func (rt *Router) accountNames(p provider.Provider, label string) []string {
	if p.Local {
		return []string{p.Name}
	}
	if label != "" {
		return []string{Account(p.Name, label)}
	}
	all := []string{p.Name}
	if rt.Accounts != nil {
		if a := rt.Accounts(p.Name); len(a) > 0 {
			all = a
		}
	}
	return all
}

// orderTargets sorts eligible targets by the combo's strategy and moves
// circuit-open ones to the end, keeping each group's order stable. commit=true
// spends the round-robin turn; Plan passes false to stay side-effect free.
func (rt *Router) orderTargets(comboName string, combo Combo, plans []targetPlan, commit bool) []targetPlan {
	out := append([]targetPlan(nil), plans...)
	switch combo.Strategy {
	case "fastest":
		lat := map[string]float64{}
		for _, tp := range out {
			lat[tp.hk] = rt.latency(tp.hk)
		}
		// Unmeasured targets count as 0, so they sort first and get measured.
		sort.SliceStable(out, func(i, j int) bool { return lat[out[i].hk] < lat[out[j].hk] })
	case "cheapest":
		price := func(tp targetPlan) float64 {
			if rt.Price != nil {
				if p, ok := rt.Price(tp.name, tp.model); ok {
					return p
				}
			}
			return math.MaxFloat64
		}
		sort.SliceStable(out, func(i, j int) bool { return price(out[i]) < price(out[j]) })
	case "context":
		ctx := func(tp targetPlan) int {
			if rt.Caps != nil {
				if c, ok := rt.Caps(tp.name, tp.model); ok {
					return c.Context
				}
			}
			return 0
		}
		sort.SliceStable(out, func(i, j int) bool { return ctx(out[i]) > ctx(out[j]) })
	case "least_used":
		rt.cstate.mu.Lock()
		used := map[string]int{}
		for _, tp := range out {
			used[tp.hk] = rt.cstate.used[tp.hk]
		}
		rt.cstate.mu.Unlock()
		sort.SliceStable(out, func(i, j int) bool { return used[out[i].hk] < used[out[j].hk] })
	case "weighted":
		w := func(tp targetPlan) int {
			if n, ok := combo.Weights[tp.target]; ok && n > 0 {
				return n
			}
			return 1
		}
		if commit {
			// Efraimidis-Spirakis draw without replacement: key = u^(1/w).
			keys := make(map[string]float64, len(out))
			for _, tp := range out {
				keys[tp.target] = math.Pow(rt.nextRand(), 1.0/float64(w(tp)))
			}
			sort.SliceStable(out, func(i, j int) bool { return keys[out[i].target] > keys[out[j].target] })
		} else {
			sort.SliceStable(out, func(i, j int) bool { return w(out[i]) > w(out[j]) })
		}
	case "headroom":
		best := func(tp targetPlan) float64 {
			if rt.Headroom == nil {
				return -1
			}
			p := rt.Providers[tp.name]
			top := -1.0
			for _, a := range rt.accountNames(p, tp.label) {
				if f, ok := rt.Headroom(p, a); ok && f > top {
					top = f
				}
			}
			return top
		}
		sort.SliceStable(out, func(i, j int) bool { return best(out[i]) > best(out[j]) })
	case "round_robin":
		if n := len(out); n > 1 {
			k := rt.rrAt(comboName) % n
			if commit {
				rt.rrBump(comboName)
			}
			out = append(append([]targetPlan{}, out[k:]...), out[:k]...)
		}
	}
	open := map[string]bool{}
	for _, tp := range out {
		open[tp.hk] = rt.circuitOpen(tp.hk)
	}
	var first, last []targetPlan
	for _, tp := range out {
		if open[tp.hk] {
			last = append(last, tp)
		} else {
			first = append(first, tp)
		}
	}
	return append(first, last...)
}

// healthNote records the outcome of one attempt against a combo target.
// status is the upstream HTTP status, -1 for a transport error and -2 when no
// request was sent at all; millis is the time to response headers.
func (rt *Router) healthNote(hk string, status int, millis float64) {
	if hk == "" {
		return
	}
	rt.cstate.mu.Lock()
	defer rt.cstate.mu.Unlock()
	if rt.cstate.health == nil {
		rt.cstate.health = map[string]*targetHealth{}
	}
	s := rt.cstate.health[hk]
	if s == nil {
		s = &targetHealth{}
		rt.cstate.health[hk] = s
	}
	now := rt.clock()
	if status != -2 {
		if rt.cstate.used == nil {
			rt.cstate.used = map[string]int{}
		}
		rt.cstate.used[hk]++
	}
	if status >= 0 {
		if !s.sampled {
			s.ewma, s.sampled = millis, true
		} else {
			s.ewma = 0.7*s.ewma + 0.3*millis
		}
	}
	fail := status == -1 || status == http.StatusRequestTimeout ||
		status == http.StatusTooManyRequests || status >= 500
	switch {
	case status == -2:
		// Never sent (no usable key): release a probe, change nothing else.
		s.probe = false
	case fail:
		s.fails++
		if s.probe {
			// The probe failed: re-open with doubled backoff.
			s.probe = false
			s.open = true
			s.since = now
			s.backoff = min(s.backoff*2, circuitMax)
		} else if !s.open && s.fails >= circuitFails {
			s.open = true
			s.since = now
			s.backoff = circuitBase
		}
	case status >= 0 && status < 400:
		// Success closes the circuit and clears the failure count.
		s.fails = 0
		if s.probe {
			s.probe = false
			s.open = false
			s.backoff = circuitBase
		}
	case s.probe:
		// A neutral answer still proves the target is alive.
		s.probe = false
		s.open = false
		s.backoff = circuitBase
	}
}

// circuitOpen reports whether the target's circuit is currently open.
func (rt *Router) circuitOpen(hk string) bool {
	rt.cstate.mu.Lock()
	defer rt.cstate.mu.Unlock()
	s := rt.cstate.health[hk]
	return s != nil && s.open
}

// circuitAllows reports whether an attempt could reach the target now: the
// circuit is closed, or open but past its cooldown (a probe may run).
func (rt *Router) circuitAllows(hk string) bool {
	rt.cstate.mu.Lock()
	defer rt.cstate.mu.Unlock()
	s := rt.cstate.health[hk]
	if s == nil || !s.open {
		return true
	}
	return !rt.clock().Before(s.since.Add(s.backoff))
}

// circuitGate is the attempt-time check: it lets a closed target through, and
// for an open one past its cooldown lets exactly one probe through.
func (rt *Router) circuitGate(hk string) (allowed, probe bool) {
	rt.cstate.mu.Lock()
	defer rt.cstate.mu.Unlock()
	s := rt.cstate.health[hk]
	if s == nil || !s.open {
		return true, false
	}
	if rt.clock().Before(s.since.Add(s.backoff)) || s.probe {
		return false, false
	}
	s.probe = true
	return true, true
}

// circuitPassable reports whether the gate would let an attempt through now,
// without claiming a probe.
func (rt *Router) circuitPassable(hk string) bool {
	rt.cstate.mu.Lock()
	defer rt.cstate.mu.Unlock()
	s := rt.cstate.health[hk]
	if s == nil || !s.open {
		return true
	}
	return !rt.clock().Before(s.since.Add(s.backoff)) && !s.probe
}

// circuitRetryAt is when an open target's cooldown ends; the zero time when
// the circuit is closed.
func (rt *Router) circuitRetryAt(hk string) time.Time {
	rt.cstate.mu.Lock()
	defer rt.cstate.mu.Unlock()
	s := rt.cstate.health[hk]
	if s == nil || !s.open {
		return time.Time{}
	}
	return s.since.Add(s.backoff)
}

// forceProbe marks an open target's next attempt as its half-open probe,
// ignoring the cooldown. serveCombo uses it when every target is cooling: a
// transient failure streak must not black out the combo.
func (rt *Router) forceProbe(hk string) {
	rt.cstate.mu.Lock()
	defer rt.cstate.mu.Unlock()
	if s := rt.cstate.health[hk]; s != nil && s.open {
		s.probe = true
	}
}

// probeSettle releases a probe marker when an attempt ended without a health
// note (for example an early local error), so the next request can probe.
func (rt *Router) probeSettle(hk string) {
	if hk == "" {
		return
	}
	rt.cstate.mu.Lock()
	defer rt.cstate.mu.Unlock()
	if s := rt.cstate.health[hk]; s != nil {
		s.probe = false
	}
}

// latency is the target's EWMA time to response headers; 0 when unmeasured,
// which sorts first under the fastest strategy so the target gets measured.
func (rt *Router) latency(hk string) float64 {
	rt.cstate.mu.Lock()
	defer rt.cstate.mu.Unlock()
	if s := rt.cstate.health[hk]; s != nil && s.sampled {
		return s.ewma
	}
	return 0
}

func (rt *Router) rrAt(name string) int {
	rt.cstate.mu.Lock()
	defer rt.cstate.mu.Unlock()
	return rt.cstate.rr[name]
}

func (rt *Router) rrBump(name string) {
	rt.cstate.mu.Lock()
	defer rt.cstate.mu.Unlock()
	if rt.cstate.rr == nil {
		rt.cstate.rr = map[string]int{}
	}
	rt.cstate.rr[name]++
}

// serveCombo routes a request whose resolved model names a combo. Every
// target but the last deliverable one is tried soft: a retryable failure
// falls through to the next target before anything reaches the client. It
// returns the provider that answered (the combo name when none did) and the
// Why text for the event.
func (rt *Router) serveCombo(rec *recorder, r *http.Request, body []byte, model, comboName string, combo Combo, why string) (string, string) {
	need := peekNeed(body)
	plans := rt.planTargets(combo, need)
	var eligible []targetPlan
	reasons := map[string]string{} // target -> reason it cannot serve
	for _, tp := range plans {
		if tp.skip != "" {
			reasons[tp.target] = tp.skip
		} else {
			eligible = append(eligible, tp)
		}
	}
	ordered := rt.orderTargets(comboName, combo, eligible, true)

	// The last target that can actually be attempted delivers as-is; the
	// rest are soft so a retryable failure can fall through.
	lastIdx := -1
	for i := len(ordered) - 1; i >= 0; i-- {
		if rt.circuitAllows(ordered[i].hk) {
			lastIdx = i
			break
		}
	}
	// Open-and-cooling targets sit after lastIdx and are always skipped; note
	// them up front so Why explains them even when an earlier target delivers.
	for _, tp := range ordered {
		if !rt.circuitAllows(tp.hk) {
			reasons[tp.target] = "circuit open"
		}
	}

	// When every eligible target is cooling, don't black out the combo: force
	// one attempt on the target whose cooldown ends soonest, delivered as-is.
	forced := -1
	passable := false
	for _, tp := range ordered {
		if rt.circuitPassable(tp.hk) {
			passable = true
			break
		}
	}
	if !passable {
		var soonest time.Time
		for i, tp := range ordered {
			at := rt.circuitRetryAt(tp.hk)
			if forced == -1 || at.Before(soonest) {
				forced, soonest = i, at
			}
		}
	}

	var tried []string
	servedBy := ""
	for i, tp := range ordered {
		probe := false
		soft := i != lastIdx
		if i == forced {
			rt.forceProbe(tp.hk)
			probe, soft = true, false
		} else {
			var allowed bool
			allowed, probe = rt.circuitGate(tp.hk)
			if !allowed {
				reasons[tp.target] = "circuit open"
				continue
			}
		}
		d := decision{provider: tp.name, label: tp.label, model: tp.model}
		delivered, status := rt.forwardProvider(rec, r, body, model, d, soft, tp.hk)
		if probe {
			rt.probeSettle(tp.hk)
		}
		tried = append(tried, tp.target+" "+outcomeText(status))
		if delivered {
			servedBy = tp.name
			break
		}
	}

	var chain strings.Builder
	if why != "" {
		chain.WriteString(why)
	}
	if len(tried) > 0 {
		if chain.Len() > 0 {
			chain.WriteString("; ")
		}
		chain.WriteString(strings.Join(tried, " -> "))
	}
	var skips []string
	for _, ts := range combo.Targets {
		if reason, ok := reasons[ts]; ok {
			skips = append(skips, "skipped "+ts+": "+reason)
		}
	}
	if len(skips) > 0 {
		if chain.Len() > 0 {
			chain.WriteString("; ")
		}
		chain.WriteString(strings.Join(skips, "; "))
	}
	evWhy := chain.String()

	if servedBy == "" {
		var parts []string
		for _, ts := range combo.Targets {
			if reason, ok := reasons[ts]; ok {
				parts = append(parts, ts+": "+reason)
			}
		}
		msg := "tsunagi: combo " + comboName + " has no usable target"
		if len(parts) > 0 {
			msg += " (" + strings.Join(parts, "; ") + ")"
		}
		writeErr(rec, http.StatusBadRequest, "invalid_request_error", msg)
		servedBy = comboName
	}
	return servedBy, evWhy
}

// outcomeText is the short result word a combo target gets in Why.
func outcomeText(status int) string {
	switch {
	case status >= 400:
		return strconv.Itoa(status)
	case status == -1:
		return "unreachable"
	case status == -2:
		return "no key"
	}
	return "ok"
}

// Plan resolves a model name the way ServeHTTP would and lists the targets
// that would be tried, in order. It never sends a request.
func (rt *Router) Plan(model string, need Need) Plan {
	resolved, why := rt.resolveFor(model, need)
	plan := Plan{Resolved: resolved, Why: why}
	if combo, ok := rt.Combos[resolved]; ok {
		plan.Combo = resolved
		plans := rt.planTargets(combo, need)
		var eligible, skipped []targetPlan
		for _, tp := range plans {
			if tp.skip == "" {
				eligible = append(eligible, tp)
			} else {
				skipped = append(skipped, tp)
			}
		}
		for _, tp := range rt.orderTargets(resolved, combo, eligible, false) {
			st := Step{Target: tp.target, Status: "ready"}
			if rt.circuitOpen(tp.hk) {
				st.Status, st.Reason = "circuit-open", "circuit open"
			} else if combo.Strategy == "weighted" {
				w := combo.Weights[tp.target]
				if w < 1 {
					w = 1
				}
				st.Reason = "weight " + strconv.Itoa(w)
			}
			plan.Steps = append(plan.Steps, st)
		}
		for _, tp := range skipped {
			plan.Steps = append(plan.Steps, Step{Target: tp.target, Status: "skip", Reason: tp.skip})
		}
		return plan
	}
	d := rt.route(resolved)
	if d.provider == "" {
		plan.Steps = []Step{{Target: "claude", Status: "ready"}}
		if rt.ClaudeFallback != "" {
			plan.Steps = append(plan.Steps, Step{Target: rt.ClaudeFallback, Status: "ready", Reason: "only if Claude answers 429 or 529"})
		}
		return plan
	}
	target := d.provider + "/" + d.model
	if d.label != "" {
		target = d.provider + "@" + d.label + "/" + d.model
	}
	st := Step{Target: target, Status: "ready"}
	p, ok := rt.Providers[d.provider]
	switch {
	case !ok:
		st.Status, st.Reason = "skip", "unknown provider"
	case !p.Ready():
		st.Status, st.Reason = "skip", "provider not ready"
	case !rt.hasKey(p, d.label):
		st.Status, st.Reason = "skip", "no key"
	}
	plan.Steps = []Step{st}
	return plan
}
