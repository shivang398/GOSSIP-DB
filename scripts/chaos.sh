#!/bin/bash
while true; do
  sleep 10
  echo "Chaos: Killing Node 3..."
  pkill -f "-id node3" || true
  sleep 5
  echo "Chaos: Restarting Node 3..."
  RATE_LIMIT=100000 RATE_BURST=100000 ./bin/gossipdb -id node3 -http 8083 -grpc 9003 -data data/node3 -peers "node1=localhost:9001,node2=localhost:9002" > data/node3.log 2>&1 &
done
