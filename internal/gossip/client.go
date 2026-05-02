package gossip

import (
	"context"
	"sync"
	"time"

	pb "github.com/shivang/gossipdb/api/proto/gossip"
	"github.com/shivang/gossipdb/internal/logger"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	connPool sync.Map // map[string]*grpc.ClientConn
)

// getConn retrieves a cached connection or creates a new one
func getConn(addr string, dialTimeout time.Duration) (*grpc.ClientConn, error) {
	if val, ok := connPool.Load(addr); ok {
		return val.(*grpc.ClientConn), nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, addr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		return nil, err
	}

	// Try to store, if another goroutine stored it first, close our new connection and use the existing one
	if val, loaded := connPool.LoadOrStore(addr, conn); loaded {
		conn.Close()
		return val.(*grpc.ClientConn), nil
	}
	return conn, nil
}

func SendSyncRequest(addr string, req *pb.SyncRequest, dialTimeout, syncTimeout time.Duration) (*pb.SyncResponse, error) {
	conn, err := getConn(addr, dialTimeout)
	if err != nil {
		return nil, err
	}

	client := pb.NewGossipServiceClient(conn)
	
	var resp *pb.SyncResponse
	var syncErr error
	
	// Retry logic (up to 3 times) for failed replication
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), syncTimeout)
		resp, syncErr = client.Sync(ctx, req)
		cancel()
		if syncErr == nil {
			return resp, nil
		}
		
		logger.Get().Warn("Sync request failed, retrying...", zap.String("addr", addr), zap.Int("attempt", i+1), zap.Error(syncErr))
		time.Sleep(time.Millisecond * 100 * time.Duration(1<<i)) // Exponential backoff (100ms, 200ms, 400ms)
	}
	
	// If it failed after retries, it might be a bad connection, remove from pool
	connPool.Delete(addr)
	conn.Close()
	return nil, syncErr
}

