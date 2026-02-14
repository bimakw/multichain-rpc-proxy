package grpc

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	rpcv1 "github.com/bimakw/multichain-rpc-proxy/api/gen/rpc/v1"
	"github.com/bimakw/multichain-rpc-proxy/internal/chain"
)

type HealthService struct {
	rpcv1.UnimplementedHealthServiceServer
	manager *chain.Manager
}

func NewHealthService(manager *chain.Manager) *HealthService {
	return &HealthService{manager: manager}
}

func (s *HealthService) Check(ctx context.Context, req *rpcv1.HealthCheckRequest) (*rpcv1.HealthCheckResponse, error) {
	stats := s.manager.AllStats()

	overallHealthy := true
	chains := make(map[string]*rpcv1.ChainStatus)

	for name, st := range stats {
		healthy := st.HealthyEndpoints > 0
		if !healthy {
			overallHealthy = false
		}

		chains[name] = &rpcv1.ChainStatus{
			Healthy:          healthy,
			HealthyEndpoints: int32(st.HealthyEndpoints),
			TotalEndpoints:   int32(st.TotalEndpoints),
			BlockHeight:      st.HighestBlock,
		}
	}

	var overallStatus rpcv1.HealthCheckResponse_Status
	if len(stats) == 0 {
		overallStatus = rpcv1.HealthCheckResponse_UNHEALTHY
	} else if overallHealthy {
		overallStatus = rpcv1.HealthCheckResponse_HEALTHY
	} else {
		overallStatus = rpcv1.HealthCheckResponse_DEGRADED
	}

	return &rpcv1.HealthCheckResponse{
		Status: overallStatus,
		Chains: chains,
	}, nil
}

func (s *HealthService) CheckChain(ctx context.Context, req *rpcv1.ChainHealthRequest) (*rpcv1.ChainHealthResponse, error) {
	ch, ok := s.manager.GetChain(req.Chain)
	if !ok {
		return nil, status.Errorf(codes.NotFound, "chain not found: %s", req.Chain)
	}

	stats := ch.Stats()

	endpoints := make([]*rpcv1.EndpointStatus, len(stats.Endpoints))
	for i, ep := range stats.Endpoints {
		endpoints[i] = &rpcv1.EndpointStatus{
			Url:            ep.URL,
			Healthy:        ep.Healthy,
			BlockHeight:    ep.BlockHeight,
			LatencyMs:      ep.LatencyMs,
			TotalRequests:  ep.TotalReqs,
			FailedRequests: ep.FailedReqs,
		}
	}

	return &rpcv1.ChainHealthResponse{
		Chain:        req.Chain,
		ChainId:      int32(stats.ChainID),
		Healthy:      stats.HealthyEndpoints > 0,
		HighestBlock: stats.HighestBlock,
		Endpoints:    endpoints,
	}, nil
}

func (s *HealthService) ListChains(ctx context.Context, req *rpcv1.ListChainsRequest) (*rpcv1.ListChainsResponse, error) {
	stats := s.manager.AllStats()

	chains := make([]*rpcv1.ChainInfo, 0, len(stats))
	for name, st := range stats {
		chains = append(chains, &rpcv1.ChainInfo{
			Name:             name,
			ChainId:          int32(st.ChainID),
			Healthy:          st.HealthyEndpoints > 0,
			HealthyEndpoints: int32(st.HealthyEndpoints),
			TotalEndpoints:   int32(st.TotalEndpoints),
			BlockHeight:      st.HighestBlock,
		})
	}

	return &rpcv1.ListChainsResponse{Chains: chains}, nil
}
