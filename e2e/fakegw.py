import http.server, json, sys
LOG=open('/tmp/fakegw.log','a')
class H(http.server.BaseHTTPRequestHandler):
    def log_message(self,*a): pass
    def do_GET(self):
        self.send_response(200); self.send_header("Content-Type","application/json"); self.end_headers()
        self.wfile.write(json.dumps({"data":[{"id":"tiny-model"}]}).encode())
    def do_POST(self):
        n=int(self.headers.get("Content-Length",0)); body=json.loads(self.rfile.read(n))
        LOG.write(json.dumps({"path":self.path,"keys":list(body.keys()),"model":body.get("model"),"ntools":len(body.get("tools",[])),"roles":[m["role"] for m in body["messages"]]})+"\n"); LOG.flush()
        self.send_response(200); self.send_header("Content-Type","text/event-stream"); self.end_headers()
        def ev(o): self.wfile.write(("data: "+json.dumps(o)+"\n\n").encode()); self.wfile.flush()
        last=body["messages"][-1]
        if last["role"]=="tool":
            ev({"id":"1","choices":[{"delta":{"content":"Tool said: "+str(last["content"]).strip()[:60]},"finish_reason":None}]})
            ev({"id":"1","choices":[{"delta":{},"finish_reason":"stop"}]})
        elif body.get("tools"):
            ev({"id":"1","choices":[{"delta":{"content":"Running it. "}}]})
            ev({"id":"1","choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_abc","type":"function","function":{"name":"Bash","arguments":"{\"command\":\"echo tsunagi-e2e\","}}]}}]})
            ev({"id":"1","choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"description\":\"echo\"}"}}]},"finish_reason":"tool_calls"}]})
        else:
            ev({"id":"1","choices":[{"delta":{"content":"title"},"finish_reason":None}]})
            ev({"id":"1","choices":[{"delta":{},"finish_reason":"stop"}]})
        ev({"id":"1","choices":[],"usage":{"prompt_tokens":12,"completion_tokens":6}})
        self.wfile.write(b"data: [DONE]\n\n")
http.server.ThreadingHTTPServer(("127.0.0.1",59997),H).serve_forever()
