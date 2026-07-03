FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
# Tolerate an offline module download: when the build context carries a
# vendor/ directory (run `go mod vendor` on the host, e.g. behind a corporate
# proxy that intercepts TLS), the build below uses it and needs no network.
RUN go mod download || echo "go mod download failed — expecting vendor/ in the build context"
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /agent-nexus-api ./cmd/server

FROM alpine:3.20
# nodejs + npm are needed for stdio MCP servers that use npx (e.g. @modelcontextprotocol/* packages).
# Fall back to HTTP mirrors when TLS to the CDN is intercepted (corporate
# proxies) — apk verifies package signatures, so transport TLS is not
# integrity-critical.
RUN apk --no-cache add ca-certificates tzdata nodejs npm git python3 \
 || (sed -i 's|https://|http://|g' /etc/apk/repositories \
     && apk --no-cache add ca-certificates tzdata nodejs npm git python3)
# uv/uvx for Python-based stdio MCP servers (e.g. mcp-atlassian); uses the
# system python3 above, packages cached under UV_CACHE_DIR.
RUN wget -qO- https://astral.sh/uv/install.sh | env UV_INSTALL_DIR=/usr/local/bin sh
ENV UV_CACHE_DIR=/app/data/uv-cache UV_PYTHON_INSTALL_DIR=/app/data/uv-python
WORKDIR /app
COPY --from=builder /agent-nexus-api .
COPY whatsapp-adapter ./whatsapp-adapter
RUN cd whatsapp-adapter && npm install --omit=dev
COPY start-api.sh .
RUN chmod +x ./start-api.sh
EXPOSE 8080
EXPOSE 18901
CMD ["./start-api.sh"]
