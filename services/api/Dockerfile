FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /agent-nexus-api ./cmd/server

FROM alpine:3.20
# nodejs + npm are needed for stdio MCP servers that use npx (e.g. @modelcontextprotocol/* packages)
RUN apk --no-cache add ca-certificates tzdata nodejs npm
WORKDIR /app
COPY --from=builder /agent-nexus-api .
EXPOSE 8080
CMD ["./agent-nexus-api"]
