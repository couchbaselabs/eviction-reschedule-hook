FROM golang:1.24.3-alpine AS builder

ENV GOOS=linux GOARCH=amd64 CGO_ENABLED=0

WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy only the necessary source files
COPY cmd/ ./cmd/
COPY pkg/ ./pkg/
RUN go build -o eviction-reschedule-hook cmd/main.go

FROM scratch

LABEL org.opencontainers.image.source="https://github.com/couchbaselabs/eviction-reschedule-hook" \
      org.opencontainers.image.description="Eviction Reschedule Hook" \
      org.opencontainers.image.licenses="Apache-2.0"

COPY --from=builder /app/eviction-reschedule-hook /eviction-reschedule-hook
USER 8453
CMD ["/eviction-reschedule-hook"]