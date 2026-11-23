#!/bin/sh
set -e

echo "Starting runtime configuration..."

# Check if the config template exists
if [ -f "/usr/share/nginx/html/config.template.js" ]; then
    echo "Processing config.template.js with environment variables..."
    # Free-text values land inside single-quoted JS strings in the template; escape
    # backslashes and quotes, and flatten line breaks, so they can't break config.js.
    # Keep this list in step with the dev server's (apps/web-ui/vite.config.ts).
    for var in PRODUCT_BRAND PRODUCT_NAME; do
        eval "value=\${$var:-}"
        escaped=$(printf '%s' "$value" | sed -e 's/\\/\\\\/g' -e "s/'/\\\\'/g" | tr '\r\n' '  ')
        export "$var=$escaped"
    done
    envsubst < /usr/share/nginx/html/config.template.js > /usr/share/nginx/html/config.js
    echo "Configuration file generated: config.js"
else
    echo "Warning: config.template.js not found, skipping runtime configuration"
fi

echo "Runtime configuration completed"
