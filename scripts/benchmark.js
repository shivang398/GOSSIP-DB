import http from 'k6/http';
import { check, sleep } from 'k6';
import { Trend, Counter } from 'k6/metrics';
import encoding from 'k6/encoding';

// Custom metrics
const replicationDelay = new Trend('replication_delay');
const writeErrors = new Counter('write_errors');
const readErrors = new Counter('read_errors');

export const options = {
  scenarios: {
    // High write load
    write_load: {
      executor: 'constant-vus',
      vus: 50,
      duration: '30s',
      exec: 'writeTest',
    },
    // Concurrent reads
    read_load: {
      executor: 'constant-vus',
      vus: 50,
      duration: '30s',
      exec: 'readTest',
    },
    // Replication latency test
    replication_test: {
      executor: 'constant-vus',
      vus: 10,
      duration: '30s',
      exec: 'replicationTest',
    },
  },
  thresholds: {
    http_req_duration: ['p(99)<200'], // 99% of requests must be below 200ms
  },
};

const BASE_URLS = [
  'http://localhost:8081',
  'http://localhost:8082',
  'http://localhost:8083',
];

export function writeTest() {
  // Randomly pick a node
  const nodeUrl = BASE_URLS[Math.floor(Math.random() * BASE_URLS.length)];
  const url = `${nodeUrl}/kv/key-${__VU}-${__ITER}`;
  const payload = JSON.stringify({ value: `data-${__ITER}` });
  const params = { headers: { 'Content-Type': 'application/json' }, tags: { name: 'KV_Put' } };
  
  const res = http.put(url, payload, params);
  if (!check(res, { 'is status 201 or 200': (r) => r.status === 201 || r.status === 200 })) {
    writeErrors.add(1);
  }
}

export function readTest() {
  const nodeUrl = BASE_URLS[Math.floor(Math.random() * BASE_URLS.length)];
  const url = `${nodeUrl}/kv/key-${Math.floor(Math.random() * 1000)}`;
  const res = http.get(url, { tags: { name: 'KV_Get' } });
  if (!check(res, { 'is status 200 or 404': (r) => r.status === 200 || r.status === 404 })) {
    readErrors.add(1);
  }
}

export function replicationTest() {
  const key = `repl-key-${__VU}-${__ITER}`;
  const val = `val-${Date.now()}`;
  
  // 1. Write to Node 1 (using Node 1 as the primary for this test to ensure stability)
  const start = Date.now();
  const writeRes = http.put(`${BASE_URLS[0]}/kv/${key}`, JSON.stringify({ value: val }), {
    headers: { 'Content-Type': 'application/json' },
    tags: { name: 'KV_Replication_Put' },
  });
  
  if (writeRes.status !== 201 && writeRes.status !== 200) {
    writeErrors.add(1);
    return;
  }
  
  // 2. Poll Node 2 until value is replicated
  let replicated = false;
  let attempts = 0;
  while (!replicated && attempts < 20) {
    const res = http.get(`${BASE_URLS[1]}/kv/${key}`, { tags: { name: 'KV_Replication_Get' } });
    if (res.status === 200) {
      let body;
      try {
        body = JSON.parse(res.body);
      } catch (e) {
        // invalid JSON
      }
      
      if (body && body.data) {
        // Data is base64 encoded in JSON
        const decoded = encoding.b64decode(body.data, 'std', 's');
        if (decoded === val) {
          replicated = true;
          replicationDelay.add(Date.now() - start);
          break;
        }
      }
    }
    attempts++;
    sleep(0.1); // Wait 100ms between polls
  }
}

