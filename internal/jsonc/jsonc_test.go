package jsonc

import (
	"encoding/json"
	"testing"
)

func TestCleanKeepsStringsAndDropsCommentsAndTrailingCommas(t *testing.T) {
	in := `{
	  // line comment
	  "url": "https://a.example/x//y", /* block */
	  "esc": "q\"//not a comment",
	  "list": [1, 2, ],
	  "obj": {"a": 1,},
	}`
	var m map[string]any
	if err := json.Unmarshal(Clean([]byte(in)), &m); err != nil {
		t.Fatal(err)
	}
	if m["url"] != "https://a.example/x//y" || m["esc"] != `q"//not a comment` {
		t.Fatalf("strings damaged: %v", m)
	}
	if len(m["list"].([]any)) != 2 {
		t.Fatalf("list = %v", m["list"])
	}
}
