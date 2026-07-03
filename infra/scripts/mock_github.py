#!/usr/bin/env python3
"""Mock GitHub REST API for integration-testing the pipeline's PR tools.

Implements POST /repos/{owner}/{repo}/pulls (create PR) and
GET /repos/{owner}/{repo}/compare/{base}...{head} (branch diff).
"""
import json
import os
import re
from http.server import BaseHTTPRequestHandler, HTTPServer

PORT = int(os.environ.get("MOCK_GITHUB_PORT", "9097"))
prs = []


class Handler(BaseHTTPRequestHandler):
    def log_message(self, fmt, *args):
        print("[mock-github]", fmt % args, flush=True)

    def _json(self, obj, code=200):
        body = json.dumps(obj).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        m = re.match(r"^/repos/([^/]+)/([^/]+)/compare/(.+)$", self.path)
        if m:
            self._json({"total_commits": 2, "files": [{
                "filename": "internal/service/handler.go", "status": "modified",
                "additions": 24, "deletions": 3,
                "patch": "@@ -10,3 +10,24 @@\n+func NewEndpoint() {}\n",
            }]})
            return
        self._json({"error": "not found"}, 404)

    def do_POST(self):
        length = int(self.headers.get("Content-Length", 0))
        body = json.loads(self.rfile.read(length) or b"{}")
        m = re.match(r"^/repos/([^/]+)/([^/]+)/pulls$", self.path)
        if m:
            number = len(prs) + 1
            pr = {
                "number": number,
                "html_url": f"https://github.com/{m.group(1)}/{m.group(2)}/pull/{number}",
                "state": "open",
                "title": body.get("title"), "head": body.get("head"), "base": body.get("base"),
            }
            prs.append(pr)
            self._json(pr, 201)
            return
        self._json({"error": "not found"}, 404)


if __name__ == "__main__":
    print(f"[mock-github] listening on :{PORT}", flush=True)
    HTTPServer(("127.0.0.1", PORT), Handler).serve_forever()
