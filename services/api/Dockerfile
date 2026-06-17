FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /agent-nexus-api ./cmd/server

FROM alpine:3.20
# nodejs + npm are needed for stdio MCP servers that use npx (e.g. @modelcontextprotocol/* packages)
RUN apk --no-cache add ca-certificates tzdata nodejs npm git
WORKDIR /app
COPY --from=builder /agent-nexus-api .
COPY whatsapp-adapter ./whatsapp-adapter
RUN cd whatsapp-adapter && npm install --omit=dev
COPY start-api.sh .
RUN chmod +x ./start-api.sh
EXPOSE 8080
EXPOSE 18901
CMD ["./start-api.sh"]
