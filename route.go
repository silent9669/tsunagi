package main

import (
	"strconv"

	"github.com/silent9669/tsunagi/internal/config"
	"github.com/silent9669/tsunagi/internal/router"
	"github.com/silent9669/tsunagi/internal/ui"
)

// cmdRoute shows how a model name would be routed: the resolved name, any
// combo, and each target's status. It never sends a request and never prints
// keys.
func cmdRoute(t *ui.Term, args []string) int {
	var model string
	need := router.Need{}
	asJSON := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			asJSON = true
		case "--tools":
			need.Tools = true
		case "--tokens":
			if i+1 >= len(args) {
				return fail(t, "--tokens needs a number")
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil || n < 0 {
				return fail(t, "--tokens needs a non-negative number")
			}
			need.Tokens = n
		default:
			if model != "" || len(args[i]) == 0 || args[i][0] == '-' {
				return fail(t, "usage: tsunagi route [--tools] [--tokens N] [--json] <model>")
			}
			model = args[i]
		}
	}
	if model == "" {
		return fail(t, "usage: tsunagi route [--tools] [--tokens N] [--json] <model>")
	}
	cfg, err := config.Load()
	if err != nil {
		return fail(t, "%v", err)
	}
	plan := newRouter(cfg, "", "").Plan(model, need)
	if asJSON {
		return printJSON(t, plan)
	}
	t.Printf("  %s %s\n", t.Dim("model"), model)
	if plan.Resolved != model {
		t.Printf("  %s %s -> %s\n", t.Dim("as"), model, plan.Resolved)
	}
	if plan.Why != "" {
		t.Printf("  %s %s\n", t.Dim("why "), plan.Why)
	}
	if plan.Combo != "" {
		t.Printf("  %s %s\n", t.Dim("combo"), plan.Combo)
	}
	var rows [][]string
	for _, s := range plan.Steps {
		rows = append(rows, []string{s.Target, routeStatus(t, s.Status), s.Reason})
	}
	t.Table([]string{"target", "status", "reason"}, rows)
	return 0
}

func routeStatus(t *ui.Term, s string) string {
	switch s {
	case "ready":
		return t.Paint(ui.Mint, s)
	case "circuit-open":
		return t.Paint(ui.Rose, s)
	}
	return t.Paint(ui.Amber, s)
}
