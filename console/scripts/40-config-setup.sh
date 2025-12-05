#!/bin/sh
set -e

echo "Starting runtime configuration..."

# Check if the config template exists
if [ -f "/usr/share/nginx/html/config.template.js" ]; then
    echo "Processing config.template.js with environment variables..."
    envsubst < /usr/share/nginx/html/config.template.js > /usr/share/nginx/html/config.js
    echo "Configuration file generated: config.js"
else
    echo "Warning: config.template.js not found, skipping runtime configuration"
fi

echo "Runtime configuration completed"
