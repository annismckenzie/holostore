# Contributing to HoloStore

Thank you for your interest in contributing to HoloStore! This guide covers
everything you need to get started: building, testing, understanding the
architecture, and submitting changes.

## Prerequisites

- **Rust** (stable toolchain) — install via [rustup](https://rustup.rs/)
- **redis-cli** — for smoke-testing the Redis interface (most Linux package
  managers: `redis-tools`; macOS: `brew install redis`)
- **protobuf compiler** (`protoc`) — required for gRPC codegen during build

Optional:
- **psql** — for interacting with HoloFusion's PostgreSQL wire protocol
- **Go 1.21+** — only needed if running the Porcupine linearizability checker

## Building

Build the core `holo_store` binary (debug):

```bash
make build
```

Optimized release build with `mimalloc`:

```bash
make build-release
```

Individual crate builds:

```bash
cargo build -p holo_accord
cargo build -p holo_store
cargo build -p holo_fusion
cargo build -p holo_workload
```

## Running a Local Cluster

Start a 3-node cluster (builds automatically if needed):

```bash
./scripts/cleanup_cluster.sh   # remove stale data
./scripts/start_cluster.sh     # start nodes 1-3
```

Default ports:

| Node | Redis port | gRPC port |
|------|-----------|-----------|
| 1    | 16379     | 15051     |
| 2    | 16380     | 15052     |
| 3    | 16381     | 15053     |

Verify the cluster is healthy:

```bash
redis-cli -p 16379 PING        # → PONG
redis-cli -p 16379 SET foo bar  # → OK
redis-cli -p 16380 GET foo      # → "bar"
```

Stop and clean up:

```bash
./scripts/cleanup_cluster.sh
```

## Running Tests

**Unit tests** (all crates):

```bash
cargo test
```

**Single crate**:

```bash
cargo test -p holo_accord
cargo test -p holo_store
cargo test -p holo_fusion
```

**Linearizability check** (requires a running cluster and Go):

```bash
make check-linearizability
```

Customize the linearizability workload:

```bash
NODES="127.0.0.1:16379,127.0.0.1:16380,127.0.0.1:16381" \
CLIENTS=3 KEYS=5 SET_PCT=50 DURATION=10s \
./scripts/check_linearizability.sh
```

**Stress test**:

```bash
make check-linearizability-stress
```

## Project Architecture

HoloStore is a Cargo workspace with four crates:

| Crate | Description | Key docs |
|-------|------------|----------|
| **`holo_accord`** | Leaderless Accord consensus protocol engine — pre-accept, accept, commit phases, dependency tracking, execution scheduling | — |
| **`holo_store`** | Core distributed KV store — Redis protocol surface, gRPC service, WAL, storage engine, cluster membership, embedded library API | [DESIGN.md](crates/holo_store/docs/DESIGN.md), [STORAGE.md](crates/holo_store/docs/STORAGE.md), [READ_MODES.md](crates/holo_store/docs/READ_MODES.md), [CLUSTER_MEMBERSHIP.md](crates/holo_store/docs/CLUSTER_MEMBERSHIP.md) |
| **`holo_fusion`** | SQL layer — PostgreSQL wire protocol backed by Apache DataFusion, with embedded HoloStore node | [README](crates/holo_fusion/README.md), [SQL_SCOPE.md](crates/holo_fusion/docs/HOLO_FUSION_SQL_SCOPE.md), [STORAGE_MODEL.md](crates/holo_fusion/docs/HOLO_FUSION_STORAGE_MODEL.md) |
| **`holo_workload`** | Benchmarking and linearizability testing workload generator | [LINEARIZABILITY.md](crates/holo_store/docs/LINEARIZABILITY.md) |

### How a write flows through the system

```
Client (redis-cli SET k v)
  → Redis protocol server (holo_store)
    → Accord consensus (holo_accord): pre-accept → accept → commit
      → WAL: durable commit-log append
        → Executor: apply to storage engine (fjall)
```

Reads can bypass full consensus depending on the configured read mode — see
[READ_MODES.md](crates/holo_store/docs/READ_MODES.md).

## Where to Find Work

Good starting points for new contributors:

- **Testing roadmap**: [crates/holo_store/docs/TESTING_TODO.md](crates/holo_store/docs/TESTING_TODO.md) —
  lists unit tests, WAL tests, and integration tests that need writing,
  prioritized by importance.
- **HoloFusion TODO**: [crates/holo_fusion/docs/HOLO_FUSION_TODO.md](crates/holo_fusion/docs/HOLO_FUSION_TODO.md) —
  SQL layer work items.
- **Client/SDK documentation**: [docs/SDK.md](docs/SDK.md) — describes all
  interaction modes; improving or extending these docs is always welcome.

## Submitting Changes

1. **Fork** the repository and create a feature branch from `main`.
2. **Write clear commit messages** — the project uses imperative-mood
   summaries (e.g., `add bounded proposal pipelining`, `harden distributed
   insert correctness`). Keep the first line under ~72 characters.
3. **Include tests** for behavioral changes where feasible.
4. **Run `cargo test`** before submitting to catch regressions.
5. **Open a pull request** with a concise description of what changed and why.

## License

By contributing, you agree that your contributions will be licensed under the
same license as the project (see [LICENSE](LICENSE)).
