# GossipDB - Architectural Walkthrough

Welcome to the architectural walkthrough for **GossipDB**. This guide breaks down the core components, their responsibilities, and how they interact to provide a highly available, causally-consistent distributed storage system.

## 1. System Overview

At its core, **GossipDB** is a multi-node distributed database. Each node in the cluster acts as an independent worker that handles reads, writes, and real-time streams while asynchronously syncing data with its peers using a custom Gossip protocol over gRPC.

The system is split into two major parts:
- **Go Backend**: The storage engine, HTTP API, and cluster management.
- **React Dashboard**: A real-time observability UI for monitoring node health.

## 2. Component Walkthrough

### 2.1 Storage Engine (`internal/store`)
The lowest level of the application is the storage engine, powered by [BadgerDB](https://github.com/dgraph-io/badger). BadgerDB is an embeddable, extremely fast key-value store optimized for SSDs.
- **Abstractions**: We wrap BadgerDB inside a `Store` interface, enabling easy testing and future storage engine replacements.
- **Soft Deletes (Tombstones)**: When a key is deleted (`DELETE /kv/{key}`), it is not immediately wiped from disk. Instead, a "tombstone" (a value with a `Deleted: true` flag) is written. This ensures that the deletion event can be replicated to other nodes via the Gossip protocol.
- **Watchers**: The store implements a pub-sub mechanism. Clients can subscribe to key changes via Server-Sent Events (SSE). When a `Put` operation occurs, the store broadcasts the new value to all active watchers over Go channels.

### 2.2 HTTP Server (`internal/server`)
The RESTful HTTP server, built with `gorilla/mux`, acts as the client-facing layer.
- **Endpoints**: Standard CRUD operations (`GET`, `PUT`, `DELETE`).
- **SSE Stream**: The `GET /watch/{key}` endpoint upgrades the connection to an SSE stream, tying directly into the store's watcher channels.
- **Middleware**: Includes IP-based Rate Limiting (using `golang.org/x/time/rate`), API Key Authentication, CORS handling, and structured request logging.
- **Metrics**: A `/metrics` endpoint exposes Prometheus counters and gauges (e.g., active watchers, request latency).

### 2.3 Distributed Replication (`internal/gossip` & `api/proto`)
To ensure high availability, data must be replicated. We use an "anti-entropy" Gossip protocol implemented over gRPC.
- **Protocol Buffer**: Defined in `api/proto/gossip.proto`. It defines a `Sync` method.
- **Sync Process**: 
  1. Periodically, a node randomly selects a peer.
  2. It sends a summary of its data (keys + metadata/timestamps).
  3. The peer compares this with its own data. If the peer is missing keys or has older versions, it replies with the full data payloads.
  4. The initiating node updates its BadgerDB with the received data.
- **Causal Consistency**: Uses timestamps (and vector clocks in broader architectures) to resolve conflicts and ensure the latest write wins.

### 2.4 Application Lifecycle (`cmd/server/main.go`)
The `main.go` file orchestrates everything:
1. Parses CLI flags (ports, node IDs, peer lists).
2. Loads configuration (`viper`) and initializes structured logging (`zap`).
3. Bootstraps the BadgerDB instance.
4. Starts the gRPC server for inter-node Gossip.
5. Initializes the HTTP server for client traffic.
6. Implements **Graceful Shutdown**: Listens for `SIGTERM`/`SIGINT` signals, safely draining HTTP connections and closing BadgerDB to prevent data corruption.

### 2.5 Observability Dashboard (`dashboard/`)
A Vite + React application styled with TailwindCSS. It polls the backend APIs and Prometheus metrics to visualize:
- Overall cluster status and peer connectivity.
- Write throughput, read latency, and replication lag.
- Live data streams via the SSE `watch` endpoint.

## 3. Data Flow Example: Writing a Value
1. A client sends `PUT /kv/user123` with payload `{"value": "John Doe"}` to Node A.
2. Node A's HTTP Server receives the request, passes rate-limiting and auth.
3. The Store wraps the payload in a `Value` struct, assigns a Unix Nano timestamp, and writes it to BadgerDB.
4. The Store broadcasts the update to any SSE watchers subscribed to `user123`.
5. The HTTP server returns `201 Created`.
6. **Later**: Node B initiates a Gossip `Sync` with Node A. Node A sends the metadata for `user123`. Node B sees its timestamp is older (or missing) and requests the full value. Node B then writes "John Doe" to its own BadgerDB instance.

## 4. Operational Considerations
- **Resource Constraints**: The server limits memory footprint via max watcher configuration limits.
- **Resilience**: The gossip protocol's exponential backoff prevents network floods when peers are down.
- **Testing**: Validated by K6 load testing scripts (`k6/`) and shell scripts orchestrating local clusters (`chaos.sh`, `test_cluster.sh`).

---
This architecture ensures that **GossipDB** is linearly scalable, highly available during network partitions (AP in CAP theorem), and easy to operate.
