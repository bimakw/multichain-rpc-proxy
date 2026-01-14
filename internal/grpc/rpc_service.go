package grpc

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	rpcv1 "github.com/bimakw/multichain-rpc-proxy/api/gen/rpc/v1"
	"github.com/bimakw/multichain-rpc-proxy/internal/cache"
	"github.com/bimakw/multichain-rpc-proxy/internal/chain"
)

// RPCService implements the gRPC RPC service
type RPCService struct {
	rpcv1.UnimplementedRPCServiceServer
	manager *chain.Manager
	cache   *cache.InMemoryCache
}

// NewRPCService creates a new RPC service
func NewRPCService(manager *chain.Manager, cache *cache.InMemoryCache) *RPCService {
	return &RPCService{
		manager: manager,
		cache:   cache,
	}
}

// Call performs a single JSON-RPC call
func (s *RPCService) Call(ctx context.Context, req *rpcv1.RPCRequest) (*rpcv1.RPCResponse, error) {
	start := time.Now()

	// Get chain
	ch, ok := s.manager.GetChain(req.Chain)
	if !ok {
		return nil, status.Errorf(codes.NotFound, "chain not found: %s", req.Chain)
	}

	// Build JSON-RPC request
	rpcReq := map[string]interface{}{
		"jsonrpc": req.Jsonrpc,
		"method":  req.Method,
		"id":      req.Id,
	}

	if len(req.Params) > 0 {
		var params interface{}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid params: %v", err)
		}
		rpcReq["params"] = params
	}

	// Check cache
	if s.cache.IsCacheable(req.Method) {
		if cached, ok := s.cache.Get(req.Chain, req.Method, req.Params); ok {
			log.Printf("[gRPC][%s] Cache hit for %s", req.Chain, req.Method)
			return s.parseResponse(cached, true)
		}
	}

	// Forward to chain
	body, err := json.Marshal(rpcReq)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to marshal request: %v", err)
	}

	resp, err := ch.ForwardWithRetry(ctx, body, 2)
	if err != nil {
		log.Printf("[gRPC][%s] Forward error: %v (duration=%v)", req.Chain, err, time.Since(start))
		return nil, status.Errorf(codes.Unavailable, "backend error: %v", err)
	}

	// Cache response
	if s.cache.IsCacheable(req.Method) {
		s.cache.Set(req.Chain, req.Method, req.Params, resp)
	}

	log.Printf("[gRPC][%s] %s completed (duration=%v)", req.Chain, req.Method, time.Since(start))

	return s.parseResponse(resp, false)
}

// parseResponse parses a JSON-RPC response into the gRPC response type
func (s *RPCService) parseResponse(data []byte, cached bool) (*rpcv1.RPCResponse, error) {
	var jsonResp struct {
		JSONRPC string          `json:"jsonrpc"`
		Result  json.RawMessage `json:"result,omitempty"`
		Error   *struct {
			Code    int32  `json:"code"`
			Message string `json:"message"`
			Data    string `json:"data,omitempty"`
		} `json:"error,omitempty"`
		ID int64 `json:"id"`
	}

	if err := json.Unmarshal(data, &jsonResp); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to parse response: %v", err)
	}

	resp := &rpcv1.RPCResponse{
		Jsonrpc: jsonResp.JSONRPC,
		Id:      jsonResp.ID,
		Cached:  cached,
	}

	if jsonResp.Error != nil {
		resp.Error = &rpcv1.RPCError{
			Code:    jsonResp.Error.Code,
			Message: jsonResp.Error.Message,
			Data:    []byte(jsonResp.Error.Data),
		}
	} else {
		resp.Result = jsonResp.Result
	}

	return resp, nil
}

// BatchCall performs multiple JSON-RPC calls
func (s *RPCService) BatchCall(ctx context.Context, req *rpcv1.BatchRPCRequest) (*rpcv1.BatchRPCResponse, error) {
	responses := make([]*rpcv1.RPCResponse, len(req.Requests))

	for i, r := range req.Requests {
		resp, err := s.Call(ctx, r)
		if err != nil {
			// Convert error to response
			st, _ := status.FromError(err)
			responses[i] = &rpcv1.RPCResponse{
				Jsonrpc: "2.0",
				Error: &rpcv1.RPCError{
					Code:    -32603,
					Message: st.Message(),
				},
				Id: r.Id,
			}
		} else {
			responses[i] = resp
		}
	}

	return &rpcv1.BatchRPCResponse{Responses: responses}, nil
}

// StreamCalls handles bidirectional streaming for subscriptions
func (s *RPCService) StreamCalls(stream rpcv1.RPCService_StreamCallsServer) error {
	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		resp, err := s.Call(stream.Context(), req)
		if err != nil {
			st, _ := status.FromError(err)
			resp = &rpcv1.RPCResponse{
				Jsonrpc: "2.0",
				Error: &rpcv1.RPCError{
					Code:    -32603,
					Message: st.Message(),
				},
				Id: req.Id,
			}
		}

		if err := stream.Send(resp); err != nil {
			return err
		}
	}
}
