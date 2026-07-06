# ── Stage 1: Build ───────────────────────────────────────────────────────
FROM golang:1.26-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .
RUN CGO_ENABLED=0 go build -ldflags "-s -w" -o /server ./cmd/server

# ── Stage 2: Runtime - distroless for security (minimal attack surface) ──────
FROM gcr.io/distroless/base-debian12:latest

COPY --from=builder /server /usr/local/bin/torrsyncplayer

EXPOSE 8889
VOLUME ["/data"]

ENTRYPOINT ["torrsyncplayer"]
CMD ["--port", "8889", "--data-dir", "/data"]