#!/bin/bash
set -e

echo "Building binary..."
go build -o bin/gossipdb cmd/server/main.go

# Cleanup previous data
rm -rf data/node1 data/node2 data/node3

echo "Starting Node 1 (HTTP 8081, gRPC 9001)..."
./bin/gossipdb -id node1 -http 8081 -grpc 9001 -data data/node1 -peers "node2=localhost:9002,node3=localhost:9003" > data/node1.log 2>&1 &
PID1=$!

echo "Starting Node 2 (HTTP 8082, gRPC 9002)..."
./bin/gossipdb -id node2 -http 8082 -grpc 9002 -data data/node2 -peers "node1=localhost:9001,node3=localhost:9003" > data/node2.log 2>&1 &
PID2=$!

echo "Starting Node 3 (HTTP 8083, gRPC 9003)..."
./bin/gossipdb -id node3 -http 8083 -grpc 9003 -data data/node3 -peers "node1=localhost:9001,node2=localhost:9002" > data/node3.log 2>&1 &
PID3=$!

# Ensure cleanup on exit
trap "kill -9 $PID1 $PID2 $PID3 2>/dev/null; rm -rf data/node*" EXIT

echo "Waiting for cluster to initialize..."
sleep 3

echo "1. Writing data to Node 1..."
curl -s -X PUT http://localhost:8081/kv/gossip-key -d '{"value": "gossip-replicated-data"}' -H "Content-Type: application/json" > /dev/null
echo "✅ Written 'gossip-key' -> 'gossip-replicated-data' to Node 1"

echo "Waiting for gossip replication (5 seconds)..."
sleep 5

echo "2. Reading data from Node 2..."
NODE2_RES=$(curl -s http://localhost:8082/kv/gossip-key)
if echo "$NODE2_RES" | grep -q "Z29zc2lwLXJlcGxpY2F0ZWQtZGF0YQ=="; then
    echo "✅ Node 2 successfully replicated the data!"
else
    echo "❌ Node 2 failed to replicate. Response: $NODE2_RES"
    exit 1
fi

echo "3. Reading data from Node 3..."
NODE3_RES=$(curl -s http://localhost:8083/kv/gossip-key)
if echo "$NODE3_RES" | grep -q "Z29zc2lwLXJlcGxpY2F0ZWQtZGF0YQ=="; then
    echo "✅ Node 3 successfully replicated the data!"
else
    echo "❌ Node 3 failed to replicate. Response: $NODE3_RES"
    exit 1
fi

echo "4. Testing Tombstone Replication (Delete)..."
curl -s -X DELETE http://localhost:8081/kv/gossip-key > /dev/null
echo "✅ Deleted 'gossip-key' from Node 1"

echo "Waiting for gossip replication (5 seconds)..."
sleep 5

echo "5. Verifying deletion on Node 2..."
NODE2_DEL_CODE=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:8082/kv/gossip-key)
if [ "$NODE2_DEL_CODE" -eq 404 ]; then
    echo "✅ Node 2 correctly received the tombstone!"
else
    echo "❌ Node 2 failed to receive tombstone. Code: $NODE2_DEL_CODE"
    exit 1
fi

echo "All cluster replication tests passed successfully!"
exit 0
