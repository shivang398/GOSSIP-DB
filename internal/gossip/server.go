package gossip

import (
	"context"
	"net"

	pb "github.com/shivang/gossipdb/api/proto/gossip"
	"github.com/shivang/gossipdb/internal/logger"
	"github.com/shivang/gossipdb/internal/store"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

type Server struct {
	pb.UnimplementedGossipServiceServer
	store store.Store
}

func NewServer(st store.Store) *Server {
	return &Server{store: st}
}

func (s *Server) Sync(ctx context.Context, req *pb.SyncRequest) (*pb.SyncResponse, error) {
	logger.Get().Debug("Received sync request", zap.String("peer_node_id", req.NodeId))

	localMeta, err := s.store.GetAllMetadata(ctx)
	if err != nil {
		return nil, err
	}

	resp := &pb.SyncResponse{
		Updates: make(map[string]*pb.Value),
	}

	// 1. Check what the peer is missing or has an older version of
	for k, localMd := range localMeta {
		peerMd, exists := req.Keys[k]
		if !exists || localMd.Timestamp > peerMd.Timestamp {
			// Peer needs this value
			val, err := s.store.Get(ctx, k)
			if err == nil && val != nil {
				resp.Updates[k] = &pb.Value{
					Data:      val.Data,
					Timestamp: val.Timestamp,
					Deleted:   val.Deleted,
				}
			}
		}
	}

	// Note: A full anti-entropy sync would also push updates from peer to local if peer has newer data.
	// But in this simple pull-based / read-repair logic, the peer pulls what it needs.
	// To make it bidirectional (push-pull), the peer requesting sync also pushes its own values if they are newer.
	// For simplicity in this implementation, we just return what the peer needs. The peer will eventually request again or we will request from it.

	return resp, nil
}

func StartGRPCServer(port string, st store.Store) (*grpc.Server, error) {
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return nil, err
	}

	grpcServer := grpc.NewServer()
	pb.RegisterGossipServiceServer(grpcServer, NewServer(st))

	go func() {
		logger.Get().Info("Starting gRPC Gossip server", zap.String("port", port))
		if err := grpcServer.Serve(lis); err != nil {
			logger.Get().Fatal("Failed to serve gRPC", zap.Error(err))
		}
	}()

	return grpcServer, nil
}
