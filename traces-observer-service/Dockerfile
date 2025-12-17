# Build stage
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder

# Add build args for cross-platform support
ARG TARGETOS
ARG TARGETARCH

WORKDIR /app

COPY go.mod go.sum ./

# Get dependencies - will also be cached if we won't change mod/sum
RUN go mod download

# Copy the source code as the last step
COPY . .

# Build the binary with optimizations using Go's native cross-compilation
# This runs natively on the build platform (amd64) and cross-compiles for target platform
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -a \
    -installsuffix cgo \
    -ldflags="-w -s" \
    -o /go/bin/traces-observer \
    -buildvcs=false

FROM alpine:3.21

# Install runtime dependencies for HTTPS and timezone support
RUN apk add --no-cache ca-certificates tzdata

# Create a non-root user/group (UID:GID = 1000:1000)
RUN addgroup -g 1000 app && adduser -D -u 1000 -G app app

# Copy binary with ownership set
COPY --from=builder --chown=1000:1000 /go/bin/traces-observer /go/bin/traces-observer

# Drop root privileges
USER 1000:1000

# Set working directory
WORKDIR /app

ENV GODEBUG=x509negativeserial=1

# Expose application port
EXPOSE 9098

ENTRYPOINT ["/go/bin/traces-observer"]
