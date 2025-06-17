FROM golang:1.24.3-alpine AS builder

ENV GOOS=linux GOARCH=amd64 CGO_ENABLED=0

WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy only the necessary source files
COPY cmd/ ./cmd/
COPY pkg/ ./pkg/
RUN go build -o couchbase-reschedule-hook cmd/main.go

FROM scratch

LABEL org.opencontainers.image.source="https://github.com/couchbase/couchbase-reschedule-hook" \
      org.opencontainers.image.description="Couchbase Reschedule Hook" \
      org.opencontainers.image.licenses="Apache-2.0"

COPY --from=builder /app/couchbase-reschedule-hook /couchbase-reschedule-hook
USER 8453
CMD ["/couchbase-reschedule-hook"]