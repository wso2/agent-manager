# CONSOLE=embed builds the console into the binary; CONSOLE=none skips the
# Node stage entirely (backend-only images, `amp init` in the dev stack).
ARG CONSOLE=embed

# The console's source: the repository's console/ directory, passed as a named
# build context (`--build-context console-src=../console`, or Compose's
# additional_contexts). Without it this stage is empty and the binary is built
# without the console; it then serves a page saying so.
FROM scratch AS console-src

# Builds the console that the binary embeds. Runs natively on the build
# platform: the output is plain static files.
FROM --platform=$BUILDPLATFORM public.ecr.aws/docker/library/node:26.12.1-slim AS console-embed
# Node 25+ no longer bundles corepack; it activates the pnpm version pinned in
# package.json's packageManager field.
RUN npm install -g corepack@0.36.0 && corepack enable
WORKDIR /console
COPY --from=console-src / ./
# The console build peaks above Node's default heap on large chunks but must
# stay well inside small Docker VMs (8 GiB, shared with the Go build).
ENV NODE_OPTIONS=--max-old-space-size=3072
RUN mkdir -p /out && if [ -f package.json ]; then \
      pnpm install --frozen-lockfile && pnpm build && cp -r apps/web-ui/dist/. /out/ ; \
    else echo "no console source: building the binary without the console"; fi

FROM --platform=$BUILDPLATFORM public.ecr.aws/docker/library/alpine:3.24 AS console-none
RUN mkdir /out

FROM console-${CONSOLE} AS console

FROM --platform=$BUILDPLATFORM public.ecr.aws/docker/library/golang:1.26.10-alpine AS builder

# Add build args for cross-platform support
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
# The distribution to build: amp (the core and every module) or amp-core.
ARG DISTRIBUTION=amp

WORKDIR /app

COPY go.mod go.sum ./

# Get dependencies - will also be cached if we won't change mod/sum
RUN go mod download

# Copy the source code as the last step
COPY . .
COPY --from=console /out/ ./webconsole/dist/

# Build the binary with optimizations using Go's native cross-compilation
# This runs natively on the build platform (amd64) and cross-compiles for target platform
RUN if [ -f webconsole/dist/index.html ]; then tags=console; else tags=; fi; \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -tags "${tags}" \
    -a \
    -installsuffix cgo \
    -ldflags="-w -s -X github.com/wso2/agent-manager/agent-manager-service/config.Version=${VERSION}" \
    -o /go/bin/agent-manager-service \
    -buildvcs=false \
    ./cmd/${DISTRIBUTION}

FROM public.ecr.aws/docker/library/alpine:3.24

# Install runtime dependencies for HTTPS, timezone support, bash and openssl for key generation
RUN apk add --no-cache 'ca-certificates>=20261020' 'ca-certificates-bundle>=20261020' tzdata bash openssl

# Create a non-root user/group (UID:GID = 1000:1000)
RUN addgroup -g 1000 app && adduser -D -u 1000 -G app app

# Create directories for keys, certificates and the SQLite database
RUN mkdir -p /app/keys /app/data/certs /var/lib/agent-manager && chown -R 1000:1000 /app /var/lib/agent-manager

# Copy binary with ownership set
COPY --from=builder --chown=1000:1000 /go/bin/agent-manager-service /go/bin/agent-manager-service
# The same binary under its command name: `amp serve | migrate | init | module`.
RUN ln -s /go/bin/agent-manager-service /go/bin/amp

# Copy entrypoint script
COPY --from=builder --chown=1000:1000 /app/entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh

# Drop root privileges
USER 1000:1000

# Set working directory
WORKDIR /app

ENV GODEBUG=x509negativeserial=1
# State: the SQLite database, certificates, and the secrets amp serve and
# amp init generate (encryption key, JWT signing keys, client secrets).
ENV AMP_DATA_DIR=/var/lib/agent-manager

# Expose application port
EXPOSE 8080

ENTRYPOINT ["/app/entrypoint.sh"]
