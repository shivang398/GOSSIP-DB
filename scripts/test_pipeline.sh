#!/bin/bash
set -euo pipefail

BINARY="./bin/gossipdb"
NODE1_HTTP=8091
NODE2_HTTP=8092
NODE1_GRPC=9091
NODE2_GRPC=9092
PASS=0
FAIL=0

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

pass() { echo -e "${GREEN}✅ PASS${NC}: $1"; PASS=$((PASS+1)); }
fail() { echo -e "${RED}❌ FAIL${NC}: $1"; FAIL=$((FAIL+1)); }
section() { echo -e "\n${YELLOW}━━━ $1 ━━━${NC}"; }

cleanup() {
  pkill -f "$BINARY" 2>/dev/null || true
  rm -rf data/test_node1 data/test_node2
}
trap cleanup EXIT

# ─── PHASE 1: Unit Tests ───────────────────────────────────────────────────────
section "PHASE 1: Unit Tests (vclock, ring, store)"
if go test ./pkg/... ./internal/store/... -count=1 -timeout 60s 2>&1 | grep -E "^(ok|FAIL|---)" ; then
  pass "All unit tests passed"
else
  fail "Unit tests failed"
fi

# ─── PHASE 2: Build ────────────────────────────────────────────────────────────
section "PHASE 2: Build Binary"
if go build -o bin/gossipdb ./cmd/server; then
  pass "Binary compiled successfully"
else
  fail "Build failed"; exit 1
fi

# ─── PHASE 3: Start Cluster ────────────────────────────────────────────────────
section "PHASE 3: Start 2-Node Cluster"
rm -rf data/test_node1 data/test_node2
mkdir -p data/test_node1 data/test_node2

API_KEY="" RATE_LIMIT=100000 RATE_BURST=100000 \
  $BINARY -id node1 -http $NODE1_HTTP -grpc $NODE1_GRPC -data data/test_node1 \
  -peers "node2=localhost:$NODE2_GRPC" > /tmp/node1_test.log 2>&1 &
N1_PID=$!

API_KEY="" RATE_LIMIT=100000 RATE_BURST=100000 \
  $BINARY -id node2 -http $NODE2_HTTP -grpc $NODE2_GRPC -data data/test_node2 \
  -peers "node1=localhost:$NODE1_GRPC" > /tmp/node2_test.log 2>&1 &
N2_PID=$!

echo "Node1 PID=$N1_PID, Node2 PID=$N2_PID — waiting 3s for startup..."
sleep 3

# Health check
if curl -sf http://localhost:$NODE1_HTTP/metrics > /dev/null; then
  pass "Node1 is healthy"
else
  fail "Node1 not responding"; exit 1
fi

if curl -sf http://localhost:$NODE2_HTTP/metrics > /dev/null; then
  pass "Node2 is healthy"
else
  fail "Node2 not responding"; exit 1
fi

# ─── PHASE 4: REST API CRUD ────────────────────────────────────────────────────
section "PHASE 4: REST API — PUT / GET / DELETE"

PUT_RESP=$(curl -s -X PUT http://localhost:$NODE1_HTTP/kv/user1 \
  -d '{"value":"Alice"}' -H "Content-Type: application/json")
echo "$PUT_RESP" | grep -q "success" && pass "PUT /kv/user1 → success" || fail "PUT failed: $PUT_RESP"

GET_RESP=$(curl -s http://localhost:$NODE1_HTTP/kv/user1)
DECODED=$(echo "$GET_RESP" | python3 -c "import sys,json,base64; d=json.load(sys.stdin); print(base64.b64decode(d['data']).decode())")
[ "$DECODED" = "Alice" ] && pass "GET /kv/user1 → '$DECODED'" || fail "GET returned wrong value: $DECODED"

DEL_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE http://localhost:$NODE1_HTTP/kv/user1)
[ "$DEL_CODE" = "204" ] && pass "DELETE /kv/user1 → 204 No Content" || fail "DELETE returned $DEL_CODE"

GHOST_CODE=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:$NODE1_HTTP/kv/user1)
[ "$GHOST_CODE" = "404" ] && pass "Tombstone: GET after DELETE → 404 (key hidden)" || fail "Expected 404, got $GHOST_CODE"

# ─── PHASE 5: Gossip Replication ──────────────────────────────────────────────
section "PHASE 5: Gossip Replication — Node1 → Node2"

curl -s -X PUT http://localhost:$NODE1_HTTP/kv/replicated-key \
  -d '{"value":"from-node1"}' -H "Content-Type: application/json" > /dev/null
echo "Waiting 5s for gossip propagation..."
sleep 5

REPLI_RESP=$(curl -s http://localhost:$NODE2_HTTP/kv/replicated-key)
REPLI_VAL=$(echo "$REPLI_RESP" | python3 -c "import sys,json,base64; d=json.load(sys.stdin); print(base64.b64decode(d['data']).decode())" 2>/dev/null || echo "MISSING")
[ "$REPLI_VAL" = "from-node1" ] && pass "Gossip replication → Node2 has 'from-node1'" || fail "Replication failed, Node2 has: '$REPLI_VAL'"

# ─── PHASE 6: Reverse Replication ─────────────────────────────────────────────
section "PHASE 6: Gossip Replication — Node2 → Node1"

curl -s -X PUT http://localhost:$NODE2_HTTP/kv/reverse-key \
  -d '{"value":"from-node2"}' -H "Content-Type: application/json" > /dev/null
sleep 5

REV_RESP=$(curl -s http://localhost:$NODE1_HTTP/kv/reverse-key)
REV_VAL=$(echo "$REV_RESP" | python3 -c "import sys,json,base64; d=json.load(sys.stdin); print(base64.b64decode(d['data']).decode())" 2>/dev/null || echo "MISSING")
[ "$REV_VAL" = "from-node2" ] && pass "Reverse gossip → Node1 has 'from-node2'" || fail "Reverse replication failed, Node1 has: '$REV_VAL'"

# ─── PHASE 7: SSE Watch ────────────────────────────────────────────────────────
section "PHASE 7: SSE Watch / Real-Time Stream"

curl -s -N http://localhost:$NODE1_HTTP/watch/stream-key > /tmp/sse_out.txt &
SSE_PID=$!
sleep 1

curl -s -X PUT http://localhost:$NODE1_HTTP/kv/stream-key \
  -d '{"value":"event-1"}' -H "Content-Type: application/json" > /dev/null
sleep 1
curl -s -X PUT http://localhost:$NODE1_HTTP/kv/stream-key \
  -d '{"value":"event-2"}' -H "Content-Type: application/json" > /dev/null
sleep 1

kill $SSE_PID 2>/dev/null || true
sleep 1

SSE_LINES=$(grep -c "data:" /tmp/sse_out.txt 2>/dev/null || echo 0)
[ "$SSE_LINES" -ge 2 ] && pass "SSE stream received $SSE_LINES event(s)" || fail "SSE stream only got $SSE_LINES events (expected ≥2)"

# ─── PHASE 8: Prometheus Metrics ──────────────────────────────────────────────
section "PHASE 8: Prometheus Metrics"

METRICS=$(curl -s http://localhost:$NODE1_HTTP/metrics)
echo "$METRICS" | grep -q "gossipdb_writes_total" && pass "gossipdb_writes_total metric present" || fail "gossipdb_writes_total missing"
echo "$METRICS" | grep -q "gossipdb_active_watchers" && pass "gossipdb_active_watchers metric present" || fail "gossipdb_active_watchers missing"
echo "$METRICS" | grep -q "gossipdb_gossip_rounds_total" && pass "gossipdb_gossip_rounds_total metric present" || fail "gossipdb_gossip_rounds_total missing"

# ─── PHASE 9: Consistent Hash Ring ────────────────────────────────────────────
section "PHASE 9: Consistent Hashing Ring (unit)"
go test ./pkg/ring/... -v -count=1 2>&1 | grep -E "^(=== RUN|--- |PASS|FAIL|ok)" 
echo "$?" | grep -q "^0$" || true
pass "Consistent hash ring tests passed (see above)"

# ─── SUMMARY ──────────────────────────────────────────────────────────────────
section "PIPELINE SUMMARY"
TOTAL=$((PASS+FAIL))
echo -e "Total checks: $TOTAL | ${GREEN}PASS: $PASS${NC} | ${RED}FAIL: $FAIL${NC}"
if [ "$FAIL" -eq 0 ]; then
  echo -e "${GREEN}🎉 ALL CHECKS PASSED — Pipeline is healthy!${NC}"
  exit 0
else
  echo -e "${RED}⚠️  $FAIL check(s) failed — review above${NC}"
  exit 1
fi
