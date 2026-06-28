# Stage 1: Build
FROM golang:1.25-bookworm AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux \
    go build -ldflags="-s -w" -o alcatraz ./cmd/alcatraz && \
    CGO_ENABLED=0 GOOS=linux \
    go build -ldflags="-s -w" -o healthcheck ./cmd/healthcheck

# Stage 2: Runtime
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /app/alcatraz /alcatraz
COPY --from=builder /app/healthcheck /healthcheck

EXPOSE 8080

ENTRYPOINT ["/alcatraz"]
