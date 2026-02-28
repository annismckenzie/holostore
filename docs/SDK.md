# HoloStore SDK & Client Guide

HoloStore exposes multiple interfaces for reading and writing data. Choose the
one that best fits your use case:

| Interface | Protocol | Best for |
|-----------|----------|----------|
| [Redis protocol](#redis-protocol) | RESP2 over TCP | Quick prototyping, benchmarks, any language with a Redis client |
| [Embedded Rust API](#embedded-rust-api) | In-process | Rust applications that want to embed a node directly |
| [gRPC API](#grpc-api) | HTTP/2 + Protobuf | Cross-language clients, custom tooling, admin operations |
| [SQL via HoloFusion](#sql-via-holofusion) | PostgreSQL wire protocol | Relational workloads, SQL familiarity, secondary indexes |
| [Admin CLI (holoctl)](#admin-cli-holoctl) | gRPC (wrapped) | Cluster operations, topology inspection, range management |

---

## Redis Protocol

The simplest way to interact with HoloStore. Any Redis client library works.

### Connecting

```bash
redis-cli -p 16379
```

### Supported Commands

| Command | Syntax | Description |
|---------|--------|-------------|
| `PING` | `PING` | Health check — returns `PONG` |
| `GET` | `GET <key>` | Read a key's value |
| `SET` | `SET <key> <value>` | Write a key/value pair |
| `HOLOSTATS` | `HOLOSTATS` | Runtime debug statistics (JSON) |
| `HOLOMETRICS` | `HOLOMETRICS` | Prometheus-format metrics export |

GET and SET operations are automatically batched for throughput when multiple
operations arrive on the same connection (pipeline-friendly).

### Example: Quick Smoke Test

```bash
redis-cli -p 16379 SET greeting "hello world"
# OK
redis-cli -p 16379 GET greeting
# "hello world"
```

### Example: Benchmarking

```bash
redis-benchmark -h localhost -p 16379 -c 50 -n 1000000 -r 100000 -P 100 -t get
```

### Multi-Node Reads

Writes go through Accord consensus across all replicas, so you can read from
any node in the cluster:

```bash
redis-cli -p 16379 SET mykey myvalue   # write to node 1
redis-cli -p 16380 GET mykey           # read from node 2 → "myvalue"
```

---

## Embedded Rust API

For Rust applications that want to run a HoloStore node in-process. This gives
you direct access to the storage engine without network hops.

### Add the Dependency

```toml
[dependencies]
holo_store = { path = "crates/holo_store" }
tokio = { version = "1", features = ["full"] }
```

### Start a Single-Node Instance

```rust
use std::net::SocketAddr;
use std::path::PathBuf;
use holo_store::{EmbeddedNodeConfig, start_embedded_node, HoloStoreClient};

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    let redis_addr: SocketAddr = "127.0.0.1:16379".parse()?;
    let grpc_addr: SocketAddr = "127.0.0.1:15051".parse()?;
    let data_dir = PathBuf::from("/tmp/holostore-data");

    // Start the embedded node (bootstrap = true for single-node).
    let node = start_embedded_node(
        EmbeddedNodeConfig::single_node(1, redis_addr, grpc_addr, data_dir),
    ).await?;

    // Create a client — automatically uses a direct in-process path
    // when the target address matches a local embedded node.
    let client = HoloStoreClient::new(grpc_addr);

    // Use the client API...
    let state = client.cluster_state_json().await?;
    println!("Cluster state: {state}");

    // Graceful shutdown.
    node.shutdown().await?;
    Ok(())
}
```

### Key Types

- **`EmbeddedNodeConfig`** — configuration for an embedded node (node ID,
  listen addresses, data directory, shard count, routing mode).
  - `EmbeddedNodeConfig::single_node(id, redis_addr, grpc_addr, data_dir)` —
    convenience constructor for a standalone single-node setup.
- **`EmbeddedNodeHandle`** — handle to a running node.
  - `.shutdown()` — graceful async shutdown.
  - `.abort()` — immediate cancellation.
- **`HoloStoreClient`** — client for issuing operations against a node.
  - `::new(addr)` — connect with default 10-second timeout.
  - `::with_timeout(addr, duration)` — connect with custom timeout.

### Client Methods

| Method | Description |
|--------|-------------|
| `cluster_state_json()` | Full cluster state as a JSON string |
| `range_stats()` | Per-shard statistics (record counts, ops, latencies) |
| `range_snapshot_latest(shard, start, end, cursor, limit, reverse)` | Paginated scan of latest key/value entries in a shard |
| `range_apply_latest(shard, start, end, entries)` | Bulk-apply snapshot entries to a shard |
| `range_apply_latest_conditional(shard, start, end, entries)` | Conditional bulk-apply with version checks |
| `range_write_latest(shard, start, end, entries)` | Replicated write of entries through consensus |
| `range_write_latest_conditional(shard, start, end, entries)` | Conditional replicated write with version checks |
| `range_split(split_key)` | Split a shard at the given key boundary |
| `range_rebalance(shard_id, replicas, leaseholder)` | Move shard replicas or transfer leaseholder |

### Local Backend Optimization

When `HoloStoreClient::new(addr)` detects that `addr` matches an embedded node
running in the same process, it automatically uses a direct in-process path
instead of going through gRPC. This eliminates serialization overhead for
co-located clients.

---

## gRPC API

The full RPC surface is defined in
[`crates/holo_store/proto/holo.proto`](../crates/holo_store/proto/holo.proto).

### Service: `HoloRpc`

**Consensus RPCs** (internal, used between nodes):
- `PreAccept`, `PreAcceptBatch`
- `Accept`, `AcceptBatch`
- `Commit`, `CommitBatch`
- `Recover`, `RecoverBatch`
- `FetchCommand`
- `ReportExecuted`, `LastCommitted`, `LastExecutedPrefix`, `SeedExecutedPrefix`
- `Executed`, `MarkVisible`

**KV RPCs** (client-facing):
- `KvGet` — read a single key
- `KvBatchGet` — read multiple keys in one call
- `KvSet` — write a single key (goes through Accord consensus)
- `KvBatchSet` — write multiple keys in one call

**Range Management RPCs**:
- `RangeStats` — per-shard statistics
- `RangeSnapshotLatest` — paginated scan
- `RangeWriteLatest` / `RangeWriteLatestConditional` — replicated writes
- `RangeApplyLatest` / `RangeApplyLatestConditional` — direct applies
- `RangeSplit` — split a shard
- `RangeRebalance` — move replicas
- `RangeMerge` — merge adjacent shards

**Cluster Admin RPCs**:
- `ClusterState` — full cluster state
- `ClusterAddNode` / `ClusterRemoveNode` — membership changes
- `ClusterSplitMetaRange` / `ClusterMetaRebalance` — meta-range operations
- `ClusterFreeze` — freeze/unfreeze client traffic
- `ClusterCheckpointControl` — durability checkpoint management
- `Join` — node join handshake

### Generating Client Stubs

The proto file uses standard `proto3` syntax. Generate stubs for your language
with `protoc`:

```bash
protoc --proto_path=crates/holo_store/proto \
       --go_out=./gen/go \
       --go-grpc_out=./gen/go \
       crates/holo_store/proto/holo.proto
```

Replace `--go_out` / `--go-grpc_out` with your language's plugin (e.g.,
`--python_out`, `--java_out`, `--ts_proto_out`).

For a complete working example in Go, see [`examples/go-grpc/`](../examples/go-grpc/).

---

## SQL via HoloFusion

HoloFusion adds a full SQL layer on top of HoloStore, exposing a PostgreSQL
wire protocol. Any PostgreSQL client driver works.

### Connecting

```bash
psql "host=127.0.0.1 port=55432 user=datafusion dbname=datafusion sslmode=disable"
```

### Supported SQL

HoloFusion supports a practical subset of SQL (see
[HOLO_FUSION_SQL_SCOPE.md](../crates/holo_fusion/docs/HOLO_FUSION_SQL_SCOPE.md)
for the full matrix):

| Category | Examples |
|----------|---------|
| Queries | `SELECT`, `WHERE`, `ORDER BY`, `LIMIT`, `JOIN`, aggregates (`COUNT`, `SUM`, `AVG`, `MIN`, `MAX`) |
| DML | `INSERT INTO ... VALUES (...)`, `INSERT INTO ... SELECT ...`, `UPDATE ... WHERE ...`, `DELETE FROM ... WHERE ...` |
| DDL | `CREATE TABLE` (with `PRIMARY KEY`, `DEFAULT`, `CHECK` constraints) |
| Transactions | `BEGIN`, `COMMIT`, `ROLLBACK` |

### Example Session

```sql
CREATE TABLE users (
    id BIGINT PRIMARY KEY,
    name TEXT NOT NULL,
    email TEXT
);

INSERT INTO users VALUES (1, 'Alice', 'alice@example.com');
INSERT INTO users VALUES (2, 'Bob', 'bob@example.com');

SELECT * FROM users WHERE id = 1;
--  id | name  |       email
-- ----+-------+------------------
--   1 | Alice | alice@example.com

BEGIN;
INSERT INTO users VALUES (3, 'Carol', 'carol@example.com');
COMMIT;
```

### Running HoloFusion

Single process:

```bash
cargo run -p holo_fusion --bin holo-fusion
```

3-node cluster:

```bash
./crates/holo_fusion/scripts/start_cluster.sh
./crates/holo_fusion/scripts/stop_cluster.sh  # stop
```

See [crates/holo_fusion/README.md](../crates/holo_fusion/README.md) for
environment variables and configuration.

---

## Admin CLI (holoctl)

`holoctl` is a command-line admin client for cluster management.

### Building

```bash
cargo build -p holo_store --release
# Binary: ./target/release/holoctl
```

### Commands

| Command | Description |
|---------|-------------|
| `state` | Full cluster state JSON |
| `topology` | Per-node range responsibilities and record counts |
| `meta-status` | Meta-range load, lag, proposal stats, and in-flight moves |
| `controller-status` | Controller lease holders for each domain |
| `add-node` | Add a node to cluster membership |
| `remove-node` | Decommission a node (drain + remove) |
| `split` | Split a data range at a key boundary |
| `merge` | Merge a range with its right-hand neighbor |
| `rebalance` | Move shard replicas or transfer leaseholder |
| `split-meta` | Split a metadata range at a hash boundary |
| `meta-rebalance` | Rebalance metadata range replicas |
| `freeze` | Freeze/unfreeze client traffic |
| `checkpoint` | Control durability checkpoints (status/pause/resume/trigger) |
| `merge-status` | Show in-flight merge progress |

### Examples

```bash
# Cluster health overview
./target/release/holoctl --target 127.0.0.1:15051 meta-status
./target/release/holoctl --target 127.0.0.1:15051 controller-status

# View topology
./target/release/holoctl --target 127.0.0.1:15051 topology

# Split a range at key "m"
./target/release/holoctl --target 127.0.0.1:15051 split --split-key m

# Rebalance shard 1 to nodes 1 and 2
./target/release/holoctl --target 127.0.0.1:15051 rebalance \
    --shard-id 1 --replica 1 --replica 2
```
