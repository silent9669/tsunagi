package router

import (
	"bytes"
	"encoding/json"
)

// tap watches reply bytes for token counts, without changing them. It reads
// server-sent events line by line, and keeps a small prefix of a plain JSON
// reply.
type tap struct {
	line   []byte
	plain  []byte
	sse    bool
	In     int
	Out    int
	sawAny bool
}

const maxPlain = 256 << 10

func (t *tap) Write(p []byte) {
	if !t.sawAny {
		t.sawAny = true
		t.sse = bytes.HasPrefix(bytes.TrimSpace(p), []byte("event:")) || bytes.HasPrefix(bytes.TrimSpace(p), []byte("data:"))
	}
	if !t.sse {
		if len(t.plain) < maxPlain {
			room := maxPlain - len(t.plain)
			if len(p) > room {
				p = p[:room]
			}
			t.plain = append(t.plain, p...)
		}
		return
	}
	for len(p) > 0 {
		i := bytes.IndexByte(p, '\n')
		if i < 0 {
			if len(t.line) < maxPlain {
				t.line = append(t.line, p...)
			}
			return
		}
		t.line = append(t.line, p[:i]...)
		t.take(t.line)
		t.line = t.line[:0]
		p = p[i+1:]
	}
}

// Done reads a plain reply once it is complete.
func (t *tap) Done() {
	if !t.sse && len(t.plain) > 0 {
		t.take(t.plain)
	}
}

type usageDoc struct {
	Message struct {
		Usage usageFields `json:"usage"`
	} `json:"message"`
	Usage usageFields `json:"usage"`
}

type usageFields struct {
	In       int `json:"input_tokens"`
	Out      int `json:"output_tokens"`
	CacheR   int `json:"cache_read_input_tokens"`
	CacheNew int `json:"cache_creation_input_tokens"`
}

func (t *tap) take(line []byte) {
	line = bytes.TrimSpace(bytes.TrimPrefix(bytes.TrimSpace(line), []byte("data:")))
	if len(line) == 0 || line[0] != '{' || !bytes.Contains(line, []byte(`"usage"`)) {
		return
	}
	var d usageDoc
	if json.Unmarshal(line, &d) != nil {
		return
	}
	for _, u := range []usageFields{d.Message.Usage, d.Usage} {
		if in := u.In + u.CacheR + u.CacheNew; in > t.In {
			t.In = in
		}
		if u.Out > 0 {
			t.Out = u.Out
		}
	}
}
