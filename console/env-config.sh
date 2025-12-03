#!/bin/sh
set -e

# Generate config.js from template using envsubst
envsubst < /app/dist/config.template.js > /app/dist/config.js

# Start the server
serve -s /app/dist -l 3000
