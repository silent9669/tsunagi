package events

import (
	"testing"
	"time"
)

func TestWriterAndTailRoundTrip(t *testing.T) {
	dir := t.TempDir()
	w, err := NewWriter(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	tail := Tail{Path: Latest(dir)}
	if tail.Path == "" {
		t.Fatal("no events file")
	}
	w.Write(Event{Time: time.Now(), Provider: "claude", Status: 200})
	got := tail.Poll()
	if len(got) != 1 || got[0].Provider != "claude" {
		t.Fatalf("got %+v", got)
	}
	w.Write(Event{Provider: "opencode", Status: 401})
	w.Write(Event{Provider: "zai", Status: 200})
	if got := tail.Poll(); len(got) != 2 {
		t.Fatalf("second poll = %+v", got)
	}
	if got := tail.Poll(); len(got) != 0 {
		t.Fatalf("repeat poll = %+v", got)
	}
}
