// Go gRPC client example for HoloStore.
//
// Demonstrates the main client-facing RPCs against a local 3-node cluster:
//   - Seed data via Redis protocol (SET — no gRPC KvSet exists)
//   - Read keys via gRPC (KvGet, KvBatchGet)
//   - Inspect cluster state (ClusterState)
//   - Query per-shard statistics (RangeStats)
//   - Read the same key from all three nodes (consistency check)
//
// Prerequisites: start the cluster with ../../scripts/start_cluster.sh
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"time"

	pb "holostore-go-example/holo_store/rpc"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Cluster addresses matching the default 3-node cluster (scripts/start_cluster.sh).
var (
	grpcAddrs  = []string{"127.0.0.1:15051", "127.0.0.1:15052", "127.0.0.1:15053"}
	redisAddrs = []string{"127.0.0.1:16379", "127.0.0.1:16380", "127.0.0.1:16381"}
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// --- 1. Connect to node 1 via gRPC ---
	fmt.Println("=== Connecting to node 1 via gRPC ===")
	conn, err := grpc.NewClient(grpcAddrs[0], grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("grpc.NewClient: %v", err)
	}
	defer conn.Close()
	client := pb.NewHoloRpcClient(conn)
	fmt.Printf("Connected to %s\n\n", grpcAddrs[0])

	// --- 2. Seed data via Redis protocol ---
	fmt.Println("=== Seeding data via Redis SET ===")
	keys := map[string]string{
		"greeting": "hello world",
		"language": "Go",
		"project":  "HoloStore",
	}
	for k, v := range keys {
		if err := redisSet(redisAddrs[0], k, v); err != nil {
			log.Fatalf("redisSet(%s): %v", k, err)
		}
		fmt.Printf("  SET %s = %q\n", k, v)
	}
	fmt.Println()

	// --- 3. KvGet — read a single key ---
	fmt.Println("=== KvGet: read a single key ===")
	demoKvGet(ctx, client, "greeting")
	fmt.Println()

	// --- 4. KvBatchGet — read multiple keys at once ---
	fmt.Println("=== KvBatchGet: read multiple keys ===")
	demoKvBatchGet(ctx, client, []string{"greeting", "language", "project", "nonexistent"})
	fmt.Println()

	// --- 5. ClusterState — inspect the cluster ---
	fmt.Println("=== ClusterState: cluster health ===")
	demoClusterState(ctx, client)
	fmt.Println()

	// --- 6. RangeStats — per-shard statistics ---
	fmt.Println("=== RangeStats: per-shard statistics ===")
	demoRangeStats(ctx, client)
	fmt.Println()

	// --- 7. Multi-node reads — verify consistency ---
	fmt.Println("=== Multi-node reads: consistency check ===")
	demoMultiNodeRead(ctx, "greeting")

	fmt.Println("\nDone.")
}

// demoKvGet reads a single key and prints the value and version.
func demoKvGet(ctx context.Context, client pb.HoloRpcClient, key string) {
	resp, err := client.KvGet(ctx, &pb.KvGetRequest{Key: []byte(key)})
	if err != nil {
		log.Fatalf("KvGet(%s): %v", key, err)
	}
	if !resp.HasValue {
		fmt.Printf("  %s: (not found)\n", key)
		return
	}
	fmt.Printf("  %s = %q", key, string(resp.Value))
	if v := resp.Version; v != nil {
		fmt.Printf("  (version: seq=%d", v.Seq)
		if v.TxnId != nil {
			fmt.Printf(", txn=%d:%d", v.TxnId.NodeId, v.TxnId.Counter)
		}
		fmt.Print(")")
	}
	fmt.Println()
}

// demoKvBatchGet reads multiple keys in a single RPC.
func demoKvBatchGet(ctx context.Context, client pb.HoloRpcClient, keys []string) {
	reqKeys := make([][]byte, len(keys))
	for i, k := range keys {
		reqKeys[i] = []byte(k)
	}
	resp, err := client.KvBatchGet(ctx, &pb.KvBatchGetRequest{Keys: reqKeys})
	if err != nil {
		log.Fatalf("KvBatchGet: %v", err)
	}
	for i, r := range resp.Responses {
		key := keys[i]
		if !r.HasValue {
			fmt.Printf("  %s: (not found)\n", key)
			continue
		}
		fmt.Printf("  %s = %q\n", key, string(r.Value))
	}
}

// demoClusterState fetches and pretty-prints a summary of the cluster state.
func demoClusterState(ctx context.Context, client pb.HoloRpcClient) {
	resp, err := client.ClusterState(ctx, &pb.ClusterStateRequest{})
	if err != nil {
		log.Fatalf("ClusterState: %v", err)
	}

	// Pretty-print the JSON (truncated for readability).
	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(resp.Json), "  ", "  "); err != nil {
		// If indenting fails, just print raw.
		fmt.Printf("  %s\n", resp.Json)
		return
	}
	output := buf.String()
	const maxLen = 1000
	if len(output) > maxLen {
		output = output[:maxLen] + "\n  ... (truncated)"
	}
	fmt.Printf("  %s\n", output)
}

// demoRangeStats fetches per-shard statistics from the node.
func demoRangeStats(ctx context.Context, client pb.HoloRpcClient) {
	resp, err := client.RangeStats(ctx, &pb.RangeStatsRequest{})
	if err != nil {
		log.Fatalf("RangeStats: %v", err)
	}
	fmt.Printf("  Node %d — %d shard(s):\n", resp.NodeId, len(resp.Ranges))
	for _, s := range resp.Ranges {
		fmt.Printf("    shard %d (index %d): records=%d, writes=%d, reads=%d, leaseholder=%v\n",
			s.ShardId, s.ShardIndex, s.RecordCount, s.WriteOpsTotal, s.ReadOpsTotal, s.IsLeaseholder)
	}
}

// demoMultiNodeRead connects to all three nodes and reads the same key to show consistency.
func demoMultiNodeRead(ctx context.Context, key string) {
	for i, addr := range grpcAddrs {
		conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Fatalf("grpc.NewClient(%s): %v", addr, err)
		}
		client := pb.NewHoloRpcClient(conn)
		resp, err := client.KvGet(ctx, &pb.KvGetRequest{Key: []byte(key)})
		conn.Close()
		if err != nil {
			log.Fatalf("KvGet(%s) on node %d: %v", key, i+1, err)
		}
		if resp.HasValue {
			fmt.Printf("  Node %d (%s): %s = %q\n", i+1, addr, key, string(resp.Value))
		} else {
			fmt.Printf("  Node %d (%s): %s = (not found)\n", i+1, addr, key)
		}
	}
}

// --- Redis RESP2 helper (no external dependencies) ---

// redisSet sends a SET command using the Redis RESP2 protocol over a raw TCP connection.
// HoloStore's gRPC API has KvGet but no KvSet — writes go through the Redis interface.
func redisSet(addr, key, value string) error {
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return fmt.Errorf("dial %s: %w", addr, err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))

	// RESP2: *3\r\n$3\r\nSET\r\n$<keylen>\r\n<key>\r\n$<vallen>\r\n<val>\r\n
	cmd := fmt.Sprintf("*3\r\n$3\r\nSET\r\n$%d\r\n%s\r\n$%d\r\n%s\r\n",
		len(key), key, len(value), value)
	if _, err := conn.Write([]byte(cmd)); err != nil {
		return fmt.Errorf("write: %w", err)
	}

	buf := make([]byte, 256)
	n, err := conn.Read(buf)
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}
	resp := string(buf[:n])
	if resp != "+OK\r\n" {
		return fmt.Errorf("unexpected response: %q", resp)
	}
	return nil
}
