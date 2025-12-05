#!/bin/bash
set -e

echo "=== Setting up Agent Manager Core Platform ==="

# Check if Docker is available
if ! docker info &> /dev/null; then
    echo "❌ Docker is not running. Please start Colima first:"
    echo "   ./setup-colima.sh"
    exit 1
fi

# Check if docker-compose file exists
if [ ! -f "../docker-compose.yml" ]; then
    echo "❌ docker-compose.yml not found"
    exit 1
fi

echo "🚀 Starting Agent Manager platform services..."
cd ..
docker compose up -d

echo ""
echo "⏳ Waiting for services to be healthy..."
sleep 5

echo ""
echo "📊 Service Status:"
docker compose ps

echo ""
echo "✅ Agent Manager platform is running!"
echo ""
echo "🌐 Access points:"
echo "   Console:   http://localhost:3000"
echo "   API:       http://localhost:8080"
echo "   Database:  postgresql://agentmanager:agentmanager@localhost:5432/agentmanager"
echo ""
echo "📋 View logs:"
echo "   docker compose logs -f"
echo ""
echo "🛑 Stop services:"
echo "   docker compose down"
