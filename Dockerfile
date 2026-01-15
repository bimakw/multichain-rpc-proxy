# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install dependencies
RUN apk add --no-cache git ca-certificates

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /bin/proxy ./cmd/proxy

# Final stage
FROM alpine:3.19

WORKDIR /app

# Install ca-certificates for HTTPS
RUN apk --no-cache add ca-certificates tzdata

# Copy binary and config
COPY --from=builder /bin/proxy /app/proxy
COPY configs/config.yaml /app/configs/config.yaml

# Create non-root user
RUN adduser -D -g '' appuser
USER appuser

EXPOSE 8080 9090

ENTRYPOINT ["/app/proxy"]
CMD ["-config", "/app/configs/config.yaml"]
