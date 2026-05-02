#!/bin/bash
set -e

echo "Building kvstore..."
go build -o bin/gossipdb cmd/server/main.go

echo "Cleaning up old data..."
rm -rf data/*
mkdir -p data/node1 data/node2 data/node3

echo "Starting cluster..."
./start_nodes.sh
sleep 5

echo "Starting chaos simulation (Node 3 failure/recovery)..."
./chaos.sh &
CHAOS_PID=$!

echo "Running k6 benchmark..."
./k6 run benchmark.js

echo "Cleaning up..."
kill $CHAOS_PID || true
pkill -f "kvstore" || true

echo "Benchmark complete."
