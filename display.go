package main

import (
	"encoding/json"

	"github.com/silent9669/tsunagi/internal/ui"
)

// printJSON writes v as indented JSON on stdout, never colored.
func printJSON(t *ui.Term, v any) int {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fail(t, "%v", err)
	}
	t.Printf("%s\n", b)
	return 0
}
