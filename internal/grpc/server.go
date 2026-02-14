package grpc

import (
	"fmt"
	"log"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthgrpc "google.golang.org/grpc/health/grpc_health_v1"

	rpcv1 "github.com/bimakw/multichain-rpc-proxy/api/gen/rpc/v1"
	"github.com/bimakw/multichain-rpc-proxy/internal/cache"
	"github.com/bimakw/multichain-rpc-proxy/internal/chain"
	"github.com/bimakw/multichain-rpc-proxy/internal/config"
)

type Server struct {
	config   config.GRPCConfig
	manager  *chain.Manager
	cache    *cache.InMemoryCache
	grpcSrv  *grpc.Server
	listener net.Listener
}

func NewServer(cfg config.GRPCConfig, manager *chain.Manager, cache *cache.InMemoryCache) (*Server, error) {
	var opts []grpc.ServerOption

	// TLS configuration
	if cfg.TLS.Enabled {
		creds, err := credentials.NewServerTLSFromFile(cfg.TLS.CertFile, cfg.TLS.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load TLS credentials: %w", err)
		}
		opts = append(opts, grpc.Creds(creds))
	}

	opts = append(opts,
		grpc.UnaryInterceptor(loggingUnaryInterceptor),
		grpc.StreamInterceptor(loggingStreamInterceptor),
	)

	grpcSrv := grpc.NewServer(opts...)

	return &Server{
		config:  cfg,
		manager: manager,
		cache:   cache,
		grpcSrv: grpcSrv,
	}, nil
}

func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.config.Port)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}
	s.listener = listener

	rpcService := NewRPCService(s.manager, s.cache)
	rpcv1.RegisterRPCServiceServer(s.grpcSrv, rpcService)

	healthService := NewHealthService(s.manager)
	rpcv1.RegisterHealthServiceServer(s.grpcSrv, healthService)

	healthSrv := health.NewServer()
	healthgrpc.RegisterHealthServer(s.grpcSrv, healthSrv)
	healthSrv.SetServingStatus("", healthgrpc.HealthCheckResponse_SERVING)

	log.Printf("Starting gRPC server on %s", addr)

	go func() {
		if err := s.grpcSrv.Serve(listener); err != nil {
			log.Printf("gRPC server error: %v", err)
		}
	}()

	return nil
}

func (s *Server) Stop() {
	log.Println("Stopping gRPC server...")
	s.grpcSrv.GracefulStop()
}
