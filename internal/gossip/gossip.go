// Package gossip implements the anti-entropy cluster synchronization protocol.
// Uses gRPC for peer-to-peer data reconciliation.
package gossip

import (
	"context"
	"math/rand"
	"sync"
	"time"

	pb "github.com/shivang/gossipdb/api/proto/gossip"
	"github.com/shivang/gossipdb/internal/config"
	"github.com/shivang/gossipdb/internal/logger"
	"github.com/shivang/gossipdb/internal/metrics"
	"github.com/shivang/gossipdb/internal/store"
	"go.uber.org/zap"
)

type Node struct {
	ID    string
	Addr  string
}

type Gossip struct {
	nodeID   string
	store    store.Store
	peers    map[string]Node // peer_id -> Node
	peersMu  sync.RWMutex
	interval time.Duration
	stopCh   chan struct{}
	cfg      *config.Config
}

func NewGossip(nodeID string, st store.Store, interval time.Duration, cfg *config.Config) *Gossip {
	return &Gossip{
		nodeID:   nodeID,
		store:    st,
		peers:    make(map[string]Node),
		interval: interval,
		stopCh:   make(chan struct{}),
		cfg:      cfg,
	}
}

func (g *Gossip) AddPeer(id, addr string) {
	g.peersMu.Lock()
	defer g.peersMu.Unlock()
	g.peers[id] = Node{ID: id, Addr: addr}
}

func (g *Gossip) Start() {
	ticker := time.NewTicker(g.interval)
	go func() {
		for {
			select {
			case <-ticker.C:
				g.gossipRound()
			case <-g.stopCh:
				ticker.Stop()
				return
			}
		}
	}()
}

func (g *Gossip) Stop() {
	close(g.stopCh)
}

func (g *Gossip) gossipRound() {
	g.peersMu.RLock()
	if len(g.peers) == 0 {
		g.peersMu.RUnlock()
		return
	}

	// Select a random peer
	var peers []Node
	for _, p := range g.peers {
		peers = append(peers, p)
	}
	g.peersMu.RUnlock()

	targetPeer := peers[rand.Intn(len(peers))]

	logger.Get().Debug("Initiating gossip round", zap.String("target_peer", targetPeer.ID))
	metrics.GossipRoundsTotal.Inc()

	// Get local metadata
	metadata, err := g.store.GetAllMetadata(context.Background())
	if err != nil {
		metrics.GossipErrorsTotal.Inc()
		logger.Get().Error("Failed to get local metadata for gossip", zap.Error(err))
		return
	}

	// Prepare request
	req := &pb.SyncRequest{
		NodeId: g.nodeID,
		Keys:   make(map[string]*pb.Metadata),
	}
	for k, md := range metadata {
		req.Keys[k] = &pb.Metadata{
			Timestamp: md.Timestamp,
			Deleted:   md.Deleted,
		}
	}

	// Send to peer (Client implementation)
	resp, err := SendSyncRequest(targetPeer.Addr, req, g.cfg.DialTimeout, g.cfg.SyncTimeout)
	if err != nil {
		metrics.GossipErrorsTotal.Inc()
		logger.Get().Error("Failed to sync with peer", zap.String("peer", targetPeer.ID), zap.Error(err))
		return
	}

	// Apply updates from peer via PutReplicated.
	// PutReplicated is version-gated: it only writes (and notifies watchers)
	// if the incoming value is actually newer than what we already have.
	// This prevents duplicate SSE events for values we already know about.
	for k, v := range resp.Updates {
		incoming := &store.Value{
			Data:      v.Data,
			Timestamp: v.Timestamp,
			Deleted:   v.Deleted,
			Version:   make(map[string]int),
		}

		if err := g.store.PutReplicated(context.Background(), k, incoming); err != nil {
			logger.Get().Error("Failed to apply replicated update",
				zap.String("key", k),
				zap.Error(err),
			)
		} else {
			metrics.GossipKeysReplicated.Inc()
			logger.Get().Debug("Replicated update applied",
				zap.String("key", k),
				zap.Bool("deleted", incoming.Deleted),
			)
		}
	}
}

