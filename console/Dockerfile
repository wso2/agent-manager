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
COPY package.json pnpm-workspace.yaml pnpm-lock.yaml .npmrc ./

# Copy all project sources
COPY apps/ apps/
COPY workspaces/ workspaces/

# --frozen-lockfile resolves from the committed lockfile so image builds are
# reproducible; a bare `pnpm install` would float semver ranges to the newest
# published version at build time.
RUN pnpm install --frozen-lockfile

# Build all projects (pnpm walks the workspace graph topologically)
RUN pnpm build

# Stage 2: Production stage with nginx
FROM public.ecr.aws/docker/library/nginx:1.30.1-alpine  AS production

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Create a non-root user for running nginx
RUN addgroup -g 1001 -S nginx-app && \
    adduser -S -D -H -u 1001 -h /var/cache/nginx -s /sbin/nologin -G nginx-app -g nginx-app nginx-app

# Copy custom nginx configuration
COPY --chown=nginx-app:nginx-app nginx.conf /etc/nginx/nginx.conf

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
