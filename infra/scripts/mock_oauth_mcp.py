#!/usr/bin/env python3
"""Mock OAuth 2.1 authorization server + Bearer-gated MCP server for
integration-testing Agent Nexus's MCP OAuth flow.

Implements: RFC 9728 protected-resource metadata, RFC 8414 AS metadata,
RFC 7591 dynamic client registration, /authorize with auto-consent redirect
(PKCE S256 verified at /token), authorization_code + refresh_token grants,
and a minimal MCP JSON-RPC endpoint (initialize, tools/list, tools/call)
that requires a currently-valid Bearer token.

Access tokens expire fast (TOKEN_TTL, default 5s) so the refresh path gets
exercised without waiting.
"""
import base64
import hashlib
import json
import os
import time
from http.server import BaseHTTPRequestHandler, HTTPServer
from urllib.parse import urlparse, parse_qs, urlencode

PORT = int(os.environ.get("MOCK_OAUTH_PORT", "9098"))
TOKEN_TTL = int(os.environ.get("TOKEN_TTL", "5"))
BASE = f"http://127.0.0.1:{PORT}"

codes = {}        # code -> {challenge, client_id}
tokens = {}       # access_token -> expiry epoch
refreshes = set() # valid refresh tokens
counters = {"access": 0, "refresh": 0, "registered": 0}


def s256(verifier: str) -> str:
    return base64.urlsafe_b64encode(hashlib.sha256(verifier.encode()).digest()).rstrip(b"=").decode()


class Handler(BaseHTTPRequestHandler):
    def log_message(self, fmt, *args):
        print("[mock-oauth]", fmt % args, flush=True)

    def _json(self, obj, code=200, headers=None):
        body = json.dumps(obj).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        for k, v in (headers or {}).items():
            self.send_header(k, v)
        self.end_headers()
        self.wfile.write(body)

    # ── discovery + authorize ────────────────────────────────────────────────
    def do_GET(self):
        u = urlparse(self.path)
        if u.path.startswith("/.well-known/oauth-protected-resource"):
            self._json({"resource": BASE + "/mcp", "authorization_servers": [BASE]})
        elif u.path.startswith("/.well-known/oauth-authorization-server"):
            self._json({
                "issuer": BASE,
                "authorization_endpoint": BASE + "/authorize",
                "token_endpoint": BASE + "/token",
                "registration_endpoint": BASE + "/register",
                "scopes_supported": ["mcp.read", "mcp.write", "offline_access"],
            })
        elif u.path == "/authorize":
            q = parse_qs(u.query)
            code = "code-" + str(len(codes) + 1)
            codes[code] = {"challenge": q["code_challenge"][0], "client_id": q["client_id"][0]}
            target = q["redirect_uri"][0] + "?" + urlencode({"code": code, "state": q["state"][0]})
            self.send_response(302)
            self.send_header("Location", target)
            self.end_headers()
        else:
            self._json({"error": "not found"}, 404)

    # ── registration, token, MCP ─────────────────────────────────────────────
    def do_POST(self):
        length = int(self.headers.get("Content-Length", 0))
        raw = self.rfile.read(length).decode()
        u = urlparse(self.path)

        if u.path == "/register":
            counters["registered"] += 1
            self._json({"client_id": f"mock-client-{counters['registered']}"}, 201)
            return

        if u.path == "/token":
            form = {k: v[0] for k, v in parse_qs(raw).items()}
            grant = form.get("grant_type")
            if grant == "authorization_code":
                c = codes.pop(form.get("code", ""), None)
                if not c or c["client_id"] != form.get("client_id") or s256(form.get("code_verifier", "")) != c["challenge"]:
                    self._json({"error": "invalid_grant"}, 400)
                    return
            elif grant == "refresh_token":
                if form.get("refresh_token") not in refreshes:
                    self._json({"error": "invalid_grant"}, 400)
                    return
                refreshes.discard(form["refresh_token"])
                counters["refresh"] += 1
            else:
                self._json({"error": "unsupported_grant_type"}, 400)
                return
            counters["access"] += 1
            at, rt = f"at-{counters['access']}", f"rt-{counters['access']}"
            tokens[at] = time.time() + TOKEN_TTL
            refreshes.add(rt)
            self._json({"access_token": at, "refresh_token": rt, "expires_in": TOKEN_TTL, "token_type": "Bearer"})
            return

        if u.path == "/mcp":
            auth = self.headers.get("Authorization", "")
            token = auth.removeprefix("Bearer ")
            if token not in tokens or tokens[token] < time.time():
                self._json({"error": "invalid_token"}, 401)
                return
            req = json.loads(raw or "{}")
            rid, method = req.get("id"), req.get("method")
            if method == "initialize":
                self._json({"jsonrpc": "2.0", "id": rid, "result": {
                    "protocolVersion": "2025-03-26",
                    "serverInfo": {"name": "mock-mcp", "version": "1.0"},
                    "capabilities": {"tools": {}},
                }}, headers={"Mcp-Session-Id": "sess-1"})
            elif method == "tools/list":
                self._json({"jsonrpc": "2.0", "id": rid, "result": {"tools": [{
                    "name": "mock_echo",
                    "description": "Echo the input back",
                    "inputSchema": {"type": "object", "properties": {"text": {"type": "string"}}},
                }]}})
            elif method == "tools/call":
                text = req.get("params", {}).get("arguments", {}).get("text", "")
                self._json({"jsonrpc": "2.0", "id": rid, "result": {
                    "content": [{"type": "text", "text": "echo: " + text}], "isError": False,
                }})
            else:
                self._json({"jsonrpc": "2.0", "id": rid, "result": {}})
            return

        self._json({"error": "not found"}, 404)


if __name__ == "__main__":
    print(f"[mock-oauth] listening on :{PORT}, token ttl {TOKEN_TTL}s", flush=True)
    HTTPServer(("127.0.0.1", PORT), Handler).serve_forever()
