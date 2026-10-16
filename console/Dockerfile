# Production Dockerfile for Agent Management Platform Console
# Multi-stage build for optimized image size

# Stage 1: Build stage
# Use native platform for build to avoid QEMU emulation slowdown
FROM --platform=$BUILDPLATFORM public.ecr.aws/docker/library/node:20.19-slim AS builder

# Install necessary build tools
RUN apt-get update && \
    apt-get install -y --no-install-recommends git gettext && \
    rm -rf /var/lib/apt/lists/*

# corepack activates the pnpm version pinned in package.json's packageManager field
RUN corepack enable

WORKDIR /app

# Copy workspace configuration
COPY package.json pnpm-workspace.yaml pnpm-lock.yaml .npmrc workspace-aliases.mjs workspace-aliases.d.mts ./

# Copy all project sources
COPY apps/ apps/
COPY workspaces/ workspaces/

# --frozen-lockfile resolves from the committed lockfile so image builds are
# reproducible; a bare `pnpm install` would float semver ranges to the newest
# published version at build time.
RUN pnpm install --frozen-lockfile

# URL path prefix the console is served under ("/" = origin root, e.g. "/console" behind
# a shared host). Baked into the Vite asset base here and persisted as ENV below so
# the runtime config (config.js basePath) and nginx rewrite agree with it.
ARG BASE_PATH=/
ENV BASE_PATH=${BASE_PATH}

# Build all projects (pnpm walks the workspace graph topologically)
RUN pnpm build

# Stage 2: Production stage with nginx
FROM public.ecr.aws/docker/library/nginx:1.30.1-alpine  AS production

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Create a non-root user for running nginx
RUN addgroup -g 1001 -S nginx-app && \
    adduser -S -D -H -u 1001 -h /var/cache/nginx -s /sbin/nologin -G nginx-app -g nginx-app nginx-app

ARG BASE_PATH=/
ENV BASE_PATH=${BASE_PATH}

# Copy custom nginx configuration, expanding the base-path rewrite marker when
# BASE_PATH is not the root (see nginx.conf).
COPY --chown=nginx-app:nginx-app nginx.conf /etc/nginx/nginx.conf
RUN set -eu; \
    bp="$(printf '%s' "${BASE_PATH}" | sed -e 's#^/*#/#' -e 's#/*$##')"; \
    if [ "${bp}" != "/" ] && [ -n "${bp}" ]; then \
      sed -i "s|# __BASE_PATH_REWRITE__|rewrite ^${bp}\$ ${bp}/ permanent;\n        rewrite ^${bp}/(.*)\$ /\$1 last;|" /etc/nginx/nginx.conf; \
    fi

# Copy built application from builder stage
COPY --from=builder --chown=nginx-app:nginx-app /app/apps/web-ui/dist /usr/share/nginx/html

# Fix permissions for nginx directories and create necessary directories
RUN mkdir -p /var/cache/nginx /var/log/nginx /tmp && \
    chown -R nginx-app:nginx-app /etc/nginx /var/cache/nginx /var/log/nginx /tmp /usr/share/nginx && \
    chmod -R 755 /var/cache/nginx /var/log/nginx /tmp

# Copy runtime configuration script for environment variables
COPY scripts/40-config-setup.sh /docker-entrypoint.d/40-config-setup.sh
RUN chmod +x /docker-entrypoint.d/40-config-setup.sh

# Expose port 3000
EXPOSE 3000

# Switch to non-root user
USER nginx-app

# Start nginx in foreground
CMD ["nginx", "-g", "daemon off;"]
