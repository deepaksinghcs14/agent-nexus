FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /agent-nexus-api ./cmd/server

# Use debian-slim so Ollama (glibc) works. Alpine uses musl and is incompatible.
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates tzdata curl nodejs npm git zstd \
    && rm -rf /var/lib/apt/lists/*

# Install Ollama
RUN curl -fsSL https://ollama.com/install.sh | sh

WORKDIR /app
COPY --from=builder /agent-nexus-api .
COPY whatsapp-adapter ./whatsapp-adapter
RUN cd whatsapp-adapter && npm install --omit=dev
COPY start-api.sh .
RUN chmod +x ./start-api.sh

EXPOSE 8080
EXPOSE 18901
CMD ["./start-api.sh"]
