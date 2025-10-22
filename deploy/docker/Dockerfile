# Build stage
FROM golang:1.22-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /src

# Copy go mod files for better caching
COPY core/go.mod core/go.sum ./
RUN go mod download

# Copy source code
COPY core/ ./
COPY config/ ../config/

# Build arguments for version info
ARG VERSION=dev
ARG BUILD_TIME
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -tags netgo \
    -ldflags="-s -w -X main.version=${VERSION} -X main.buildTime=${BUILD_TIME}" \
    -o /out/karoo ./cmd/karoo

# Runtime stage
FROM alpine:3.20

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata && \
    adduser -D -s /bin/sh karoo karoo

# Set working directory
WORKDIR /app

# Copy binary and set permissions
COPY --from=builder /out/karoo /usr/local/bin/karoo
RUN chmod +x /usr/local/bin/karoo

# Copy config template
COPY --from=builder /src/../config/config.example.json /app/config.json

# Create non-root user setup
RUN chown -R karoo:karoo /app
USER karoo

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/healthz || exit 1

# Expose ports
EXPOSE 3334 8080

# Set entrypoint
ENTRYPOINT ["/usr/local/bin/karoo", "-config", "/app/config.json"]
