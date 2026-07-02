#!/usr/bin/env python3
"""Mock OpenAI-compatible chat-completions server for integration testing.

Behavior: if the request contains a tool-result message, stream a final text
answer; otherwise stream a single tool call to the tool named in MOCK_TOOL.
This drives an Agent Nexus run into the approval gate on the first model call
and to completion on the call after the tool executes.
"""
import json
import os
from http.server import BaseHTTPRequestHandler, HTTPServer

MOCK_TOOL = os.environ.get("MOCK_TOOL", "native_list_agents")
MOCK_TOOL_ARGS = os.environ.get("MOCK_TOOL_ARGS", "{}")
PORT = int(os.environ.get("MOCK_PORT", "9099"))


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

        if has_tool_result:
            sse({"choices": [{"delta": {"content": "All done after approval."}, "finish_reason": None}]})
            sse({"choices": [{"delta": {}, "finish_reason": "stop"}]})
        else:
            sse({"choices": [{"delta": {"tool_calls": [{
                "index": 0, "id": "call_mock_1", "type": "function",
                "function": {"name": MOCK_TOOL, "arguments": MOCK_TOOL_ARGS},
            }]}, "finish_reason": None}]})
            sse({"choices": [{"delta": {}, "finish_reason": "tool_calls"}]})
        sse({"usage": {"prompt_tokens": 10, "completion_tokens": 5}})
        self.wfile.write(b"data: [DONE]\n\n")


if __name__ == "__main__":
    print(f"[mock-llm] listening on :{PORT}, tool={MOCK_TOOL}", flush=True)
    HTTPServer(("127.0.0.1", PORT), Handler).serve_forever()
