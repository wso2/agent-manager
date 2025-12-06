# Production Dockerfile for Agent Management Platform Console
# Multi-stage build for optimized image size

# Stage 1: Build stage
# Use native platform for build to avoid QEMU emulation slowdown
FROM --platform=$BUILDPLATFORM node:20.19-slim AS builder

# Install necessary build tools
RUN apt-get update && \
    apt-get install -y --no-install-recommends git gettext && \
    rm -rf /var/lib/apt/lists/*

# Install pnpm and Rush globally
RUN npm install -g pnpm@9.12.3 @microsoft/rush@5.157.0

WORKDIR /app

# Copy Rush configuration and monorepo structure
COPY rush.json ./
COPY common/ common/

# Copy all project sources
COPY apps/ apps/
COPY workspaces/ workspaces/

# Install dependencies using Rush
# This will install all dependencies for all projects in the monorepo
RUN rush update --full

# Build all projects
# This compiles TypeScript and generates production-ready builds
RUN rush build --verbose

# Build the webapp for production
WORKDIR /app/apps/webapp
RUN pnpm run build

# Stage 2: Production stage with nginx
FROM nginx:1.27-alpine AS production

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Create a non-root user for running nginx
RUN addgroup -g 1001 -S nginx-app && \
    adduser -S -D -H -u 1001 -h /var/cache/nginx -s /sbin/nologin -G nginx-app -g nginx-app nginx-app

# Copy custom nginx configuration
COPY --chown=nginx-app:nginx-app nginx.conf /etc/nginx/nginx.conf

# Copy built application from builder stage
COPY --from=builder --chown=nginx-app:nginx-app /app/apps/webapp/dist /usr/share/nginx/html

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
