# Go gRPC Client Example

A self-contained Go program that demonstrates how to interact with a HoloStore
cluster using the gRPC API.

## What it covers

1. **Connect** to a node via gRPC
2. **Seed data** via the Redis protocol (there is no `KvSet` gRPC RPC — writes
   go through the Redis interface)
3. **KvGet** — read a single key, inspect its value and version
4. **KvBatchGet** — read multiple keys in one RPC call
5. **ClusterState** — fetch and pretty-print the cluster state JSON
6. **RangeStats** — query per-shard statistics (record counts, ops, leaseholder)
7. **Multi-node reads** — connect to all three nodes and read the same key to
   verify strong consistency

## Prerequisites

- **Go 1.22+**
- A running HoloStore 3-node cluster (see below)

## Running

Start the cluster from the repository root:

```bash
./scripts/cleanup_cluster.sh
./scripts/start_cluster.sh
```

Then run the example:

```bash
cd examples/go-grpc
go run .
```

## Expected output

```
=== Connecting to node 1 via gRPC ===
Connected to 127.0.0.1:15051

=== Seeding data via Redis SET ===
  SET greeting = "hello world"
  SET language = "Go"
  SET project = "HoloStore"

=== KvGet: read a single key ===
  greeting = "hello world"  (version: seq=..., txn=1:...)

=== KvBatchGet: read multiple keys ===
  greeting = "hello world"
  language = "Go"
  project = "HoloStore"
  nonexistent: (not found)

=== ClusterState: cluster health ===
  { ... }

=== RangeStats: per-shard statistics ===
  Node 1 — N shard(s):
    shard 1 (index 0): records=3, writes=3, reads=..., leaseholder=true

=== Multi-node reads: consistency check ===
  Node 1 (127.0.0.1:15051): greeting = "hello world"
  Node 2 (127.0.0.1:15052): greeting = "hello world"
  Node 3 (127.0.0.1:15053): greeting = "hello world"

Done.
```

## Regenerating protobuf stubs

The generated Go files in `holo_store/rpc/` are committed for convenience. To
regenerate them (e.g., after proto changes):

```bash
# Install the Go protoc plugins (once):
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Regenerate:
./generate.sh
```

This requires `protoc` (the protobuf compiler) on your `PATH`.

## Why writes use Redis

HoloStore's gRPC `HoloRpc` service exposes `KvGet` and `KvBatchGet` for reads,
but there is no `KvSet` RPC. Key/value writes are submitted through the Redis
protocol interface (`SET` command on port 16379). The example includes a minimal
RESP2 helper that writes via raw TCP — no external Redis library needed.
