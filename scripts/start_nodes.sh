#!/bin/bash
mkdir -p data/node1 data/node2 data/node3

echo "Starting Node 1 (HTTP 8081, gRPC 9001)..."
RATE_LIMIT=100000 RATE_BURST=100000 ./bin/gossipdb -id node1 -http 8081 -grpc 9001 -data data/node1 -peers "node2=localhost:9002,node3=localhost:9003" > data/node1.log 2>&1 &

echo "Starting Node 2 (HTTP 8082, gRPC 9002)..."
RATE_LIMIT=100000 RATE_BURST=100000 ./bin/gossipdb -id node2 -http 8082 -grpc 9002 -data data/node2 -peers "node1=localhost:9001,node3=localhost:9003" > data/node2.log 2>&1 &

echo "Starting Node 3 (HTTP 8083, gRPC 9003)..."
RATE_LIMIT=100000 RATE_BURST=100000 ./bin/gossipdb -id node3 -http 8083 -grpc 9003 -data data/node3 -peers "node1=localhost:9001,node2=localhost:9002" > data/node3.log 2>&1 &

echo "Nodes started in background."
