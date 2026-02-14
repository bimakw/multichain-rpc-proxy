package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/bimakw/multichain-rpc-proxy/internal/cache"
	"github.com/bimakw/multichain-rpc-proxy/internal/chain"
	"github.com/bimakw/multichain-rpc-proxy/internal/config"
	grpcserver "github.com/bimakw/multichain-rpc-proxy/internal/grpc"
	"github.com/bimakw/multichain-rpc-proxy/internal/proxy"
	tlsutil "github.com/bimakw/multichain-rpc-proxy/internal/tls"
)

func main() {
	configPath := flag.String("config", "configs/config.yaml", "Path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	log.Printf("Starting multichain-rpc-proxy")
	log.Printf("Loaded %d chains", len(cfg.Chains))

	var clientTLSConfig *tls.Config
	if cfg.Server.TLS.ClientTLS.Enabled {
		var err error
		clientTLSConfig, err = tlsutil.NewClientTLSConfig(cfg.Server.TLS.ClientTLS)
		if err != nil {
			log.Fatalf("Failed to create client TLS config: %v", err)
		}
		log.Printf("Client TLS enabled for backend connections")
	}

	// Initialize chain manager with TLS
	manager := chain.NewManagerWithOptions(chain.ManagerOptions{
		Chains:    cfg.Chains,
		TLSConfig: clientTLSConfig,
	})
	manager.Start()

	memCache := cache.NewInMemory(cfg.Cache)

	rateLimiter := proxy.NewRateLimiter(cfg.RateLimit)

	handler := proxy.NewHandler(manager, memCache)

	app := fiber.New(fiber.Config{
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		BodyLimit:    10 * 1024 * 1024, // 10MB
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			return c.Status(500).JSON(fiber.Map{
				"jsonrpc": "2.0",
				"error": fiber.Map{
					"code":    -32603,
					"message": "Internal error",
				},
				"id": nil,
			})
		},
	})

	app.Use(proxy.RecoveryMiddleware())
	app.Use(proxy.CORSMiddleware())
	app.Use(compress.New())

	app.Get("/", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"name":    "multichain-rpc-proxy",
			"version": "1.0.0",
			"docs":    "https://github.com/bimakw/multichain-rpc-proxy",
		})
	})

	app.Get("/health", handler.HandleHealth)
	app.Get("/health/:chain", handler.HandleChainHealth)
	app.Get("/chains", handler.HandleChains)

	// RPC endpoints with rate limiting
	rpc := app.Group("/:chain", rateLimiter.Middleware())
	rpc.Post("/", handler.HandleRPC)

	if cfg.Metrics.Enabled {
		go func() {
			metricsAddr := fmt.Sprintf(":%d", cfg.Metrics.Port)
			log.Printf("Starting metrics server on %s", metricsAddr)

			mux := http.NewServeMux()
			mux.Handle("/metrics", promhttp.Handler())

			if err := http.ListenAndServe(metricsAddr, mux); err != nil {
				log.Printf("Metrics server error: %v", err)
			}
		}()
	}

	var grpcSrv *grpcserver.Server
	if cfg.GRPC.Enabled {
		var err error
		grpcSrv, err = grpcserver.NewServer(cfg.GRPC, manager, memCache)
		if err != nil {
			log.Fatalf("Failed to create gRPC server: %v", err)
		}

		if err := grpcSrv.Start(); err != nil {
			log.Fatalf("Failed to start gRPC server: %v", err)
		}
	}

	go func() {
		addr := fmt.Sprintf(":%d", cfg.Server.Port)

		var err error
		if cfg.Server.TLS.Enabled {
			log.Printf("Starting HTTPS proxy server on %s", addr)
			err = app.ListenTLS(addr, cfg.Server.TLS.CertFile, cfg.Server.TLS.KeyFile)
		} else {
			log.Printf("Starting HTTP proxy server on %s", addr)
			err = app.Listen(addr)
		}

		if err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := app.ShutdownWithContext(ctx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	if grpcSrv != nil {
		grpcSrv.Stop()
	}

	manager.Stop()

	log.Println("Server stopped")
}
