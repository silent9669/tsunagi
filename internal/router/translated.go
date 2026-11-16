package router

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/silent9669/tsunagi/internal/provider"
	"github.com/silent9669/tsunagi/internal/translate"
)

// forwardTranslated serves a Claude Code request from a provider that does
// not speak the Anthropic format: translate the request, call the provider,
// translate the reply back.
func (rt *Router) forwardTranslated(w http.ResponseWriter, r *http.Request, body []byte, p provider.Provider, model string) {
	switch r.URL.Path {
	case "/v1/messages/count_tokens":
		// The translated APIs have no counting endpoint; a rough size is enough for Claude Code.
		json.NewEncoder(w).Encode(map[string]int{"input_tokens": len(body)/4 + 1})
		return
	case "/v1/messages":
	default:
		writeErr(w, http.StatusNotFound, "not_found_error", "tsunagi: "+p.Name+" does not support "+r.URL.Path)
		return
	}
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "invalid_request_error", "tsunagi: POST only")
		return
	}
	key, err := rt.Key(p.Name)
	if err != nil || key == "" {
		writeErr(w, http.StatusUnauthorized, "authentication_error", keyMessage(p.Name, err))
		return
	}
	endpoint, toReq, toStream, toReply := "/responses", translate.ToResponses, translate.StreamToAnthropic, translate.ReplyToAnthropic
	if p.Dialect == provider.OpenAIChat {
		endpoint, toReq, toStream, toReply = "/chat/completions", translate.ToChat, translate.ChatStreamToAnthropic, translate.ChatReplyToAnthropic
	}
	out, stream, err := toReq(body, model)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request_error", "tsunagi: "+err.Error())
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost,
		strings.TrimRight(p.BaseURL, "/")+endpoint, bytes.NewReader(out))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "api_error", "tsunagi: bad base_url for "+p.Name)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	}
	resp, err := rt.transport.RoundTrip(req)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "api_error", "tsunagi: upstream "+req.URL.Host+" unreachable: "+err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		w.Write(translate.ErrorToAnthropic(resp.StatusCode, b))
		return
	}
	if stream && strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		rc := http.NewResponseController(w)
		toStream(resp.Body, w, func() { rc.Flush() }, model)
		return
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err == nil {
		b, err = toReply(b, model)
	}
	if err != nil {
		writeErr(w, http.StatusBadGateway, "api_error", "tsunagi: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(b)
}
