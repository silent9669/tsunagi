package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/silent9669/tsunagi/internal/events"
	"github.com/silent9669/tsunagi/internal/ui"
	"github.com/silent9669/tsunagi/internal/usage"
)

// cmdStatusline prints one line for Claude Code's status line: which provider
// the current model goes to and what it carried today. Claude Code passes its
// state as JSON on stdin.
func cmdStatusline(t *ui.Term, _ []string) int {
	var in struct {
		Model struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
		} `json:"model"`
	}
	b, _ := io.ReadAll(io.LimitReader(os.Stdin, 1<<20))
	json.Unmarshal(b, &in)
	fmt.Println(statusLine(in.Model.ID, in.Model.DisplayName))
	return 0
}

func statusLine(modelID, display string) string {
	name := modelID
	if name == "" {
		name = display
	}
	provider := "claude"
	if i := strings.Index(modelID, "/"); i > 0 {
		provider, _, _ = strings.Cut(modelID[:i], "@")
	}
	line := "tsunagi | " + clean(name)
	dir, err := events.Dir()
	if err != nil {
		return line
	}
	for _, r := range usage.Aggregate(events.ReadAll(dir, usage.Since(1))) {
		if r.Provider == provider {
			line += fmt.Sprintf(" | %s %d req %s/%s tok", provider, r.Requests, ui.Count(r.In), ui.Count(r.Out))
			if r.Skipped > 0 {
				line += fmt.Sprintf(" | %d failover", r.Skipped)
			}
			return line
		}
	}
	return line + " | " + provider
}
