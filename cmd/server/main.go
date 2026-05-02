// GossipDB - A Highly Available, Causally-Consistent Distributed Key-Value Store.
// Built with Go, BadgerDB, and gRPC.
package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/shivang/gossipdb/internal/config"
	"github.com/shivang/gossipdb/internal/gossip"
	"github.com/shivang/gossipdb/internal/logger"
	"github.com/shivang/gossipdb/internal/server"
	"github.com/shivang/gossipdb/internal/store"
	"go.uber.org/zap"
)

func main() {
	// Parse flags for cluster setup
	nodeID := flag.String("id", "node1", "Node ID")
	httpPort := flag.Int("http", 8080, "HTTP Port")
	grpcPort := flag.String("grpc", "9000", "gRPC Port")
	peers := flag.String("peers", "", "Comma-separated list of peer_id=host:port (e.g. node2=localhost:9001)")
	dataDir := flag.String("data", "./data/badger", "Data directory for BadgerDB")
	flag.Parse()

	// 1. Load Config (Fallback to env vars if needed)
	cfg, err := config.LoadConfig()
	if err != nil {
		panic(err)
	}

	// 2. Initialize Logger
	logger.Init(cfg.LogLevel)
	defer logger.Sync()

	log := logger.Get()
	log.Info("Starting KV Store Node",
		zap.String("node_id", *nodeID),
		zap.Int("http_port", *httpPort),
		zap.String("grpc_port", *grpcPort),
		zap.String("data_dir", *dataDir),
	)

	// 3. Initialize Store
	kvStore, err := store.NewBadgerStore(*dataDir, cfg.MaxWatchers)
	if err != nil {
		log.Fatal("Failed to initialize BadgerStore", zap.Error(err))
	}
	defer kvStore.Close()

	// 4. Initialize and Start gRPC Server for Gossip
	grpcServer, err := gossip.StartGRPCServer(*grpcPort, kvStore)
	if err != nil {
		log.Fatal("Failed to start gRPC server", zap.Error(err))
	}
	defer grpcServer.Stop()

	// 5. Initialize Gossip Node
	gossipNode := gossip.NewGossip(*nodeID, kvStore, 2*time.Second, cfg) // 2 second gossip interval for quick testing
	if *peers != "" {
		for _, peer := range strings.Split(*peers, ",") {
			parts := strings.Split(peer, "=")
			if len(parts) == 2 {
				gossipNode.AddPeer(parts[0], parts[1])
				log.Info("Added peer", zap.String("peer_id", parts[0]), zap.String("addr", parts[1]))
			}
		}
	}
	gossipNode.Start()
	defer gossipNode.Stop()

	// 6. Initialize and Start HTTP Server
	srv := server.NewServer(kvStore, *httpPort, cfg)

	go func() {
		if err := srv.Start(); err != nil {
			log.Fatal("Server failed to start", zap.Error(err))
		}
	}()

	// 7. Wait for termination
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	<-stop
	log.Info("Shutting down gracefully...")

	// Create a context with a timeout for the shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Stop(ctx); err != nil {
		log.Error("Server forced to shutdown", zap.Error(err))
	} else {
		log.Info("Server stopped cleanly")
	}
}
