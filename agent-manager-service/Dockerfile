FROM --platform=$BUILDPLATFORM public.ecr.aws/docker/library/golang:1.25-alpine AS builder

# Add build args for cross-platform support
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev

WORKDIR /app

COPY go.mod go.sum ./
COPY clients/openchoreosvc/auth ./clients/openchoreosvc/auth 

# Get dependencies - will also be cached if we won't change mod/sum
RUN go mod download

# Copy the source code as the last step
COPY . .

# Build the binary with optimizations using Go's native cross-compilation
# This runs natively on the build platform (amd64) and cross-compiles for target platform
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -a \
    -installsuffix cgo \
    -ldflags="-w -s -X github.com/wso2/ai-agent-management-platform/agent-manager-service/config.Version=${VERSION}" \
    -o /go/bin/agent-manager-service \
    -buildvcs=false

FROM public.ecr.aws/docker/library/alpine:3.21

# Install runtime dependencies for HTTPS, timezone support, bash and openssl for key generation
RUN apk add --no-cache ca-certificates tzdata bash openssl

# Create a non-root user/group (UID:GID = 1000:1000)
RUN addgroup -g 1000 app && adduser -D -u 1000 -G app app

# Create directory for schema files and keys
RUN mkdir -p /app/clients/openchoreosvc/client /app/keys && chown -R 1000:1000 /app

# Copy binary with ownership set
COPY --from=builder --chown=1000:1000 /go/bin/agent-manager-service /go/bin/agent-manager-service

# Copy the default OpenAPI schema file
COPY --from=builder --chown=1000:1000 /app/clients/openchoreosvc/client/default-openapi-schema.yaml /app/clients/openchoreosvc/client/default-openapi-schema.yaml

# Copy scripts for runtime key generation
COPY --from=builder --chown=1000:1000 /app/scripts/gen_keys.sh /app/scripts/gen_keys.sh

# Copy entrypoint script
COPY --from=builder --chown=1000:1000 /app/entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh /app/scripts/gen_keys.sh

# Drop root privileges
USER 1000:1000

# Set working directory
WORKDIR /app

ENV GODEBUG=x509negativeserial=1

# Expose application port
EXPOSE 8080

ENTRYPOINT ["/app/entrypoint.sh"]
