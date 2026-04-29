#!/bin/bash
# Stops all AMP port-forward processes tracked in the PID file.

PID_FILE="/tmp/amp-port-forward.pids"

if [ ! -f "$PID_FILE" ] || [ ! -s "$PID_FILE" ]; then
    echo "ℹ️  No active AMP port-forwards found"
    exit 0
fi

KILLED=0
while IFS= read -r PID; do
    if [[ "$PID" =~ ^[0-9]+$ ]] && kill -0 "$PID" 2>/dev/null; then
        CMD="$(ps -p "$PID" -o args= 2>/dev/null || true)"
        if [[ "$CMD" == kubectl\ port-forward* ]]; then
            kill "$PID" 2>/dev/null && KILLED=$((KILLED + 1))
        fi
    fi
done < "$PID_FILE"

rm -f "$PID_FILE"

if [ "$KILLED" -gt 0 ]; then
    echo "✅ Stopped $KILLED port-forward process(es)"
else
    echo "ℹ️  No active AMP port-forwards found"
fi
