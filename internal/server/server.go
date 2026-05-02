// Package server implements the RESTful API and SSE streaming layer for GossipDB.
// Includes authentication, rate limiting, and Prometheus metrics.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/shivang/gossipdb/internal/logger"
	"github.com/shivang/gossipdb/internal/metrics"
	"github.com/shivang/gossipdb/internal/store"
	"github.com/shivang/gossipdb/internal/config"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

type Server struct {
	store   store.Store
	port    int
	srv     *http.Server
	cfg     *config.Config
	limiter *IPRateLimiter
}

func NewServer(store store.Store, port int, cfg *config.Config) *Server {
	return &Server{
		store:   store,
		port:    port,
		cfg:     cfg,
		limiter: NewIPRateLimiter(rate.Limit(cfg.RateLimit), cfg.RateBurst),
	}
}

func (s *Server) setupRouter() *mux.Router {
	r := mux.NewRouter()

	// CORS Middleware
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}
			
			next.ServeHTTP(w, r)
		})
	})

	// Apply logging middleware
	r.Use(LoggingMiddleware)

	// API Endpoints
	api := r.PathPrefix("/kv").Subrouter()
	api.Use(RateLimitMiddleware(s.limiter))
	api.Use(AuthMiddleware(s.cfg.ApiKey))
	api.HandleFunc("/{key}", s.handleGet).Methods(http.MethodGet)
	api.HandleFunc("/{key}", s.handlePut).Methods(http.MethodPut)
	api.HandleFunc("/{key}", s.handleDelete).Methods(http.MethodDelete)

	watch := r.PathPrefix("/watch").Subrouter()
	watch.Use(RateLimitMiddleware(s.limiter))
	watch.Use(AuthMiddleware(s.cfg.ApiKey))
	watch.HandleFunc("/{key}", s.handleWatch).Methods(http.MethodGet)

	// Prometheus Metrics
	r.Handle("/metrics", promhttp.Handler())

	return r
}

func (s *Server) Start() error {
	router := s.setupRouter()
	addr := fmt.Sprintf(":%d", s.port)
	
	s.srv = &http.Server{
		Addr:    addr,
		Handler: router,
	}

	logger.Get().Info("Starting server", zap.String("addr", addr))
	return s.srv.ListenAndServe()
}

func (s *Server) Stop(ctx context.Context) error {
	if s.srv != nil {
		return s.srv.Shutdown(ctx)
	}
	return nil
}

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	key := vars["key"]

	val, err := s.store.Get(r.Context(), key)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "failed to read value")
		return
	}
	
	if val == nil {
		respondWithError(w, http.StatusNotFound, "key not found")
		return
	}

	// If it's a tombstone, act like it's not found
	if val.Deleted {
		respondWithError(w, http.StatusNotFound, "key not found")
		return
	}

	respondWithJSON(w, http.StatusOK, val)
}

func (s *Server) handlePut(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	key := vars["key"]

	var req struct {
		Value string `json:"value"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	val := &store.Value{
		Data:      []byte(req.Value),
		Timestamp: time.Now().UnixNano(),
		Version:   make(map[string]int),
		Deleted:   false,
	}

	if err := s.store.Put(r.Context(), key, val); err != nil {
		respondWithError(w, http.StatusInternalServerError, "failed to save value")
		return
	}

	respondWithJSON(w, http.StatusCreated, map[string]string{"message": "success"})
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	key := vars["key"]

	// In a distributed system, we usually soft-delete by writing a tombstone.
	// We'll write a new Value with Deleted=true.
	
	tombstone := &store.Value{
		Data:      nil,
		Timestamp: time.Now().UnixNano(),
		Version:   make(map[string]int),
		Deleted:   true,
	}

	if err := s.store.Put(r.Context(), key, tombstone); err != nil {
		respondWithError(w, http.StatusInternalServerError, "failed to delete value")
		return
	}

	// Idempotent operation: returning 200 OK whether it existed or not, or 204 No Content
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleWatch(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	key := vars["key"]

	// Set headers for SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	ch, unsubscribe, err := s.store.Watch(r.Context(), key)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer unsubscribe()

	// Track active watchers
	metrics.ActiveWatchers.Inc()
	defer metrics.ActiveWatchers.Dec()

	// Initial flush to establish connection
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			// Client disconnected
			return
		case val := <-ch:
			data, err := json.Marshal(val)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

// Helpers for JSON responses
func respondWithError(w http.ResponseWriter, code int, message string) {
	respondWithJSON(w, code, map[string]string{"error": message})
}

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	response, err := json.Marshal(payload)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal server error"))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(response)
}
