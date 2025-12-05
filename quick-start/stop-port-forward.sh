#!/usr/bin/env bash

# Script to stop all port forwarding processes for Agent Management Platform
# Supports both kubectl port-forward and socat methods

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PID_FILE="${SCRIPT_DIR}/.port-forward.pid"

# Ports used by AMP services
PORTS=(8080 21893 9098 3000 8443)

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Stopping Agent Management Platform Port Forwarding"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# Method 1: Kill socat processes (OpenChoreo approach)
echo "Checking for socat processes..."
SOCAT_PIDS=$(pgrep -f "socat.*TCP-LISTEN" 2>/dev/null || true)

if [[ -n "$SOCAT_PIDS" ]]; then
    echo "Found socat port-forward processes:"
    for pid in $SOCAT_PIDS; do
        PROCESS_INFO=$(ps -p "$pid" -o command= 2>/dev/null || echo "Process not found")
        echo "  PID $pid: $PROCESS_INFO"
    done
    echo ""
    echo "Terminating socat processes..."
    pkill -f "socat.*TCP-LISTEN" 2>/dev/null || true
    sleep 1
    echo "✓ socat processes terminated"
else
    echo "No socat processes found"
fi

echo ""

# Method 2: Find and kill kubectl port-forward processes using lsof
echo "Searching for kubectl port-forward processes on ports: ${PORTS[*]}..."

# Build lsof command with all ports
LSOF_CMD="lsof"
for port in "${PORTS[@]}"; do
    LSOF_CMD="$LSOF_CMD -i :$port"
done

# Get kubectl processes using these ports
KUBECTL_PIDS=$(eval "$LSOF_CMD" 2>/dev/null | grep kubectl | awk '{print $2}' | sort -u || true)

if [[ -n "$KUBECTL_PIDS" ]]; then
    echo "Found kubectl port-forward processes:"
    for pid in $KUBECTL_PIDS; do
        # Get process details
        PROCESS_INFO=$(ps -p "$pid" -o command= 2>/dev/null || echo "Process not found")
        echo "  PID $pid: $PROCESS_INFO"
    done
    echo ""
    echo "Terminating processes..."
    for pid in $KUBECTL_PIDS; do
        if kill "$pid" 2>/dev/null; then
            echo "  ✓ Killed process $pid"
        else
            echo "  ✗ Failed to kill process $pid (may require sudo)"
        fi
    done
    echo ""
    echo "✓ kubectl port-forward processes terminated"
else
    echo "No kubectl port-forward processes found on monitored ports"
fi

echo ""

# Method 3: Clean up PID file if it exists
if [[ -f "$PID_FILE" ]]; then
    echo "Cleaning up PID file..."
    rm -f "$PID_FILE"
    echo "✓ PID file removed"
fi

# Method 4: Fallback - try to kill any remaining kubectl port-forward processes
echo "Checking for any remaining kubectl port-forward processes..."
REMAINING_PIDS=$(pgrep -f "kubectl port-forward" 2>/dev/null || true)

if [[ -n "$REMAINING_PIDS" ]]; then
    echo "Found remaining processes: $REMAINING_PIDS"
    echo "Attempting to kill remaining processes..."
    pkill -f "kubectl port-forward" 2>/dev/null || true
    echo "✓ Cleanup complete"
else
    echo "No remaining port-forward processes found"
fi

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✓ Port forwarding cleanup complete"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
