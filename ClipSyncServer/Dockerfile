# ============================================================================
# Stage 1: Build
# ============================================================================
FROM golang:1.23-alpine AS builder

RUN apk add --no-cache gcc musl-dev

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=1 GOOS=linux go build \
    -ldflags="-s -w" \
    -o clipsyncd .

# ============================================================================
# Stage 2: Run
# ============================================================================
FROM alpine:latest

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /build/clipsyncd .

RUN mkdir -p /app/data

EXPOSE 8080

ENV LISTEN_ADDR=:8080 \
    DB_PATH=/app/data/clip.db \
    JWT_SECRET=change-me-in-production

VOLUME ["/app/data"]

ENTRYPOINT ["./clipsyncd"]
