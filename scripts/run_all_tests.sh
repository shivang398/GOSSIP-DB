#!/bin/bash
set -e

echo "1. Running Go Unit Tests..."
make test
echo "✅ Unit Tests Passed!"

echo ""
echo "2. Building Docker Image..."
make docker-build > /dev/null
echo "✅ Docker Image Built Successfully!"

echo ""
echo "3. Starting Server inside Docker container..."
mkdir -p test-data
docker run -d -p 8080:8080 --name test-kv-store -v "$(pwd)/test-data:/root/data/badger" kvstore:latest
sleep 3 # Wait for server to start

echo "✅ Server Started!"

echo ""
echo "4. Testing REST API endpoints..."

echo "- Testing PUT /kv/user1"
curl -s -X PUT http://localhost:8080/kv/user1 -d '{"value": "Alice"}' -H "Content-Type: application/json" | grep "success" > /dev/null
echo "✅ PUT Successful!"

echo "- Testing GET /kv/user1"
GET_RES=$(curl -s http://localhost:8080/kv/user1)
echo $GET_RES | grep "QWxpY2U=" > /dev/null # QWxpY2U= is base64 for "Alice"
echo "✅ GET Successful! Retrieved value: $GET_RES"

echo "- Testing SSE Watch endpoint..."
# Start watching in the background
curl -s -N http://localhost:8080/watch/user1 > watch_out.txt &
WATCH_PID=$!
sleep 2

# Trigger an update
curl -s -X PUT http://localhost:8080/kv/user1 -d '{"value": "Alice-Updated"}' -H "Content-Type: application/json" > /dev/null
sleep 1

# Kill watcher
kill $WATCH_PID 2>/dev/null || true

# Verify output contains the updated value (QWxpY2UtVXBkYXRlZA== is base64 for "Alice-Updated")
grep "QWxpY2UtVXBkYXRlZA==" watch_out.txt > /dev/null
echo "✅ Watch/SSE Successful! Event received."

echo "- Testing DELETE /kv/user1"
DELETE_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE http://localhost:8080/kv/user1)
if [ "$DELETE_CODE" -ne 204 ]; then
  echo "❌ DELETE Failed. Expected 204, got $DELETE_CODE"
  exit 1
fi
echo "✅ DELETE Successful!"

echo "- Verifying Soft-Delete (Tombstone)..."
GET_DELETED_CODE=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/kv/user1)
if [ "$GET_DELETED_CODE" -ne 404 ]; then
  echo "❌ Tombstone verification failed. Expected 404, got $GET_DELETED_CODE"
  exit 1
fi
echo "✅ Soft-Delete (Tombstone) Successful! Key is hidden."

echo ""
echo "5. Testing Persistence Across Restarts..."
echo "- Restarting Docker container..."
docker restart test-kv-store
sleep 3

# user1 was deleted, let's create a new one to test persistence across restarts
curl -s -X PUT http://localhost:8080/kv/user2 -d '{"value": "Bob"}' -H "Content-Type: application/json" > /dev/null
docker restart test-kv-store
sleep 3

GET_BOB=$(curl -s http://localhost:8080/kv/user2)
echo $GET_BOB | grep "Qm9i" > /dev/null # Qm9i is base64 for "Bob"
echo "✅ Persistence Successful! Data survived restart."

echo ""
echo "6. Cleanup..."
docker rm -f test-kv-store > /dev/null
# Cannot easily rm -rf test-data without sudo due to docker root ownership, so we leave it or ask user.
echo "✅ Tests Complete!"
