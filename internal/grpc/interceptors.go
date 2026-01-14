package grpc

import (
	"context"
	"log"
	"time"

	"google.golang.org/grpc"
)

// loggingUnaryInterceptor logs unary RPC calls
func loggingUnaryInterceptor(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (interface{}, error) {
	start := time.Now()

	resp, err := handler(ctx, req)

	duration := time.Since(start)
	if err != nil {
		log.Printf("[gRPC] %s error=%v duration=%v", info.FullMethod, err, duration)
	} else {
		log.Printf("[gRPC] %s duration=%v", info.FullMethod, duration)
	}

	return resp, err
}

// loggingStreamInterceptor logs streaming RPC calls
func loggingStreamInterceptor(
	srv interface{},
	ss grpc.ServerStream,
	info *grpc.StreamServerInfo,
	handler grpc.StreamHandler,
) error {
	start := time.Now()

	err := handler(srv, ss)

	duration := time.Since(start)
	if err != nil {
		log.Printf("[gRPC] %s (stream) error=%v duration=%v", info.FullMethod, err, duration)
	} else {
		log.Printf("[gRPC] %s (stream) duration=%v", info.FullMethod, duration)
	}

	return err
}
