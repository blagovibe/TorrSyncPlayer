# ============================================================
# Multi-stage build для TorrSyncPlayer Backend
# ============================================================

# ── Stage 1: Build ──────────────────────────────────────────
FROM golang:1.26-alpine AS builder

# Устанавливаем зависимости для сборки
RUN apk add --no-cache git make

# Устанавливаем рабочую директорию
WORKDIR /app

# Копируем файлы зависимостей
COPY backend/go.mod backend/go.sum ./
RUN go mod download

# Копируем исходный код
COPY backend/ .

# Собираем бинарный файл
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/bin/server ./cmd/server/

# ── Stage 2: Runtime ────────────────────────────────────────
FROM alpine:3.19

# Устанавливаем необходимые пакеты
RUN apk add --no-cache ca-certificates tzdata

# Создаём непривилегированного пользователя
RUN addgroup -g 1000 -S appgroup && \
    adduser -u 1000 -S appuser -G appgroup

# Устанавливаем рабочую директорию
WORKDIR /app

# Копируем бинарный файл из builder
COPY --from=builder /app/bin/server /app/bin/server

# Создаём директорию для данных
RUN mkdir -p /app/data && chown -R appuser:appgroup /app

# Переключаемся на непривилегированного пользователя
USER appuser

# Порт сервера
EXPOSE 8889

# Директория для данных
VOLUME ["/app/data"]

# Переменные окружения по умолчанию
ENV PORT=8889
ENV DATA_DIR=/app/data
ENV LOG_LEVEL=info
ENV LOG_FORMAT=json

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -q -O /dev/null http://localhost:8889/health || exit 1

# Запуск сервера
ENTRYPOINT ["/app/bin/server"]
CMD ["--port", "8889", "--data-dir", "/app/data"]
