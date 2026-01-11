package main

import (
	"context"
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
	"github.com/bimakw/multichain-rpc-proxy/internal/proxy"
)

func main() {
	configPath := flag.String("config", "configs/config.yaml", "Path to config file")
	flag.Parse()

	// Load config
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	log.Printf("Starting multichain-rpc-proxy")
	log.Printf("Loaded %d chains", len(cfg.Chains))

	// Initialize chain manager
	manager := chain.NewManager(cfg.Chains)
	manager.Start()

	// Initialize cache
	memCache := cache.NewInMemory(cfg.Cache)

	// Initialize rate limiter
	rateLimiter := proxy.NewRateLimiter(cfg.RateLimit)

	// Initialize proxy handler
	handler := proxy.NewHandler(manager, memCache)

	// Create Fiber app
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

	// Middleware
	app.Use(proxy.RecoveryMiddleware())
	app.Use(proxy.CORSMiddleware())
	app.Use(compress.New())

	// Routes
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

	// Start metrics server if enabled
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

	// Start server
	go func() {
		addr := fmt.Sprintf(":%d", cfg.Server.Port)
		log.Printf("Starting proxy server on %s", addr)

		if err := app.Listen(addr); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down...")

	// Stop accepting new requests
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := app.ShutdownWithContext(ctx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	// Stop chain manager
	manager.Stop()

	log.Println("Server stopped")
}
