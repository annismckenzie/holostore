#!/usr/bin/env bash
# Regenerate Go protobuf + gRPC stubs from holo.proto.
#
# Prerequisites:
#   go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
#   go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
#
# Run from the examples/go-grpc/ directory.
set -euo pipefail

PROTO_DIR="../../crates/holo_store/proto"

protoc \
  --proto_path="$PROTO_DIR" \
  --go_out=. --go_opt=module=holostore-go-example \
  --go-grpc_out=. --go-grpc_opt=module=holostore-go-example \
  "$PROTO_DIR/holo.proto"

echo "Generated holo_store/rpc/holo.pb.go and holo_store/rpc/holo_grpc.pb.go"
