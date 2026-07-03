#!/usr/bin/env python3
"""Mock OpenAI-compatible chat-completions server for integration testing.

Two modes:

Default: if the request contains a tool-result message, stream a final text
answer; otherwise stream a single tool call to MOCK_TOOL (args MOCK_TOOL_ARGS).

Scripted (MOCK_SCRIPT set): MOCK_SCRIPT is a JSON array of turns served in
order, one per request — {"tool": "name", "args": {...}} for a tool call or
{"text": "..."} for a final answer. Repeats the last turn if requests exceed
the script. Drives multi-step orchestrator runs deterministically.
"""
import itertools
import json
import os
from http.server import BaseHTTPRequestHandler, HTTPServer

MOCK_TOOL = os.environ.get("MOCK_TOOL", "native_list_agents")
MOCK_TOOL_ARGS = os.environ.get("MOCK_TOOL_ARGS", "{}")
MOCK_SCRIPT = json.loads(os.environ.get("MOCK_SCRIPT", "null"))
PORT = int(os.environ.get("MOCK_PORT", "9099"))
_counter = itertools.count()


class Handler(BaseHTTPRequestHandler):
    def log_message(self, fmt, *args):
        print("[mock-llm]", fmt % args, flush=True)

    def do_POST(self):
        if not self.path.endswith("/chat/completions"):
            self.send_response(404)
            self.end_headers()
            return
        length = int(self.headers.get("Content-Length", 0))
        body = json.loads(self.rfile.read(length) or b"{}")
        has_tool_result = any(m.get("role") == "tool" for m in body.get("messages", []))

        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.end_headers()

        def sse(obj):
            self.wfile.write(b"data: " + json.dumps(obj).encode() + b"\n\n")

        def emit_text(text):
            sse({"choices": [{"delta": {"content": text}, "finish_reason": None}]})
            sse({"choices": [{"delta": {}, "finish_reason": "stop"}]})

        def emit_tool(name, args, call_no):
            sse({"choices": [{"delta": {"tool_calls": [{
                "index": 0, "id": f"call_mock_{call_no}", "type": "function",
                "function": {"name": name, "arguments": json.dumps(args) if isinstance(args, dict) else args},
            }]}, "finish_reason": None}]})
            sse({"choices": [{"delta": {}, "finish_reason": "tool_calls"}]})

        n = next(_counter)
        if MOCK_SCRIPT is not None:
            turn = MOCK_SCRIPT[min(n, len(MOCK_SCRIPT) - 1)]
            print(f"[mock-llm] scripted turn {n}: {json.dumps(turn)[:120]}", flush=True)
            if "tool" in turn:
                emit_tool(turn["tool"], turn.get("args", {}), n)
            else:
                emit_text(turn.get("text", "Done."))
        elif has_tool_result:
            emit_text("All done after approval.")
        else:
            emit_tool(MOCK_TOOL, MOCK_TOOL_ARGS, 1)
        sse({"usage": {"prompt_tokens": 10, "completion_tokens": 5}})
        self.wfile.write(b"data: [DONE]\n\n")


if __name__ == "__main__":
    print(f"[mock-llm] listening on :{PORT}, tool={MOCK_TOOL}", flush=True)
    HTTPServer(("127.0.0.1", PORT), Handler).serve_forever()
