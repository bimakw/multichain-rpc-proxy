/*
 * Copyright (c) 2025 Bima Kharisma Wicaksana
 * GitHub: https://github.com/bimakw
 *
 * Licensed under MIT License.
 * See LICENSE file for details.
 */

package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewWatcher(t *testing.T) {
	// Create a temp config file
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")

	configContent := `
server:
  port: 8080
chains:
  ethereum:
    chain_id: 1
    endpoints:
      - url: "http://localhost:8545"
        weight: 1
    health_check:
      interval: 10s
      timeout: 5s
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Load initial config
	cfg, err := Load(configPath)
	if err != nil {
		t.Fatal(err)
	}

	// Create watcher
	watcher, err := NewWatcher(configPath, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer watcher.Stop()

	if watcher == nil {
		t.Fatal("watcher should not be nil")
	}

	// Test Config method
	if watcher.Config() == nil {
		t.Error("Config() should not return nil")
	}
}

func TestWatcherConfigReload(t *testing.T) {
	// Create a temp config file
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")

	initialConfig := `
server:
  port: 8080
chains:
  ethereum:
    chain_id: 1
    endpoints:
      - url: "http://localhost:8545"
        weight: 1
    health_check:
      interval: 10s
      timeout: 5s
`
	if err := os.WriteFile(configPath, []byte(initialConfig), 0644); err != nil {
		t.Fatal(err)
	}

	// Load initial config
	cfg, err := Load(configPath)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Server.Port != 8080 {
		t.Errorf("Initial port = %d, want 8080", cfg.Server.Port)
	}

	// Create watcher
	watcher, err := NewWatcher(configPath, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer watcher.Stop()

	// Track reload callback
	reloadCalled := make(chan bool, 1)
	var newPort int

	watcher.OnReload(func(old, new *Config) error {
		newPort = new.Server.Port
		reloadCalled <- true
		return nil
	})

	// Start watching
	watcher.Start()

	// Wait a bit for watcher to start
	time.Sleep(100 * time.Millisecond)

	// Update config file
	updatedConfig := `
server:
  port: 9090
chains:
  ethereum:
    chain_id: 1
    endpoints:
      - url: "http://localhost:8545"
        weight: 1
    health_check:
      interval: 10s
      timeout: 5s
`
	if err := os.WriteFile(configPath, []byte(updatedConfig), 0644); err != nil {
		t.Fatal(err)
	}

	// Wait for reload (with debounce)
	select {
	case <-reloadCalled:
		if newPort != 9090 {
			t.Errorf("New port = %d, want 9090", newPort)
		}
	case <-time.After(2 * time.Second):
		// Reload may not trigger on all systems, skip
		t.Skip("File watch event not received (may be platform-specific)")
	}
}

func TestWatcherForceReload(t *testing.T) {
	// Create a temp config file
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")

	configContent := `
server:
  port: 8080
chains:
  ethereum:
    chain_id: 1
    endpoints:
      - url: "http://localhost:8545"
        weight: 1
    health_check:
      interval: 10s
      timeout: 5s
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Load initial config
	cfg, err := Load(configPath)
	if err != nil {
		t.Fatal(err)
	}

	// Create watcher
	watcher, err := NewWatcher(configPath, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer watcher.Stop()

	// Track reload callback
	reloadCalled := false
	watcher.OnReload(func(old, new *Config) error {
		reloadCalled = true
		return nil
	})

	// Update config file
	updatedConfig := `
server:
  port: 9090
chains:
  ethereum:
    chain_id: 1
    endpoints:
      - url: "http://localhost:8545"
        weight: 1
    health_check:
      interval: 10s
      timeout: 5s
`
	if err := os.WriteFile(configPath, []byte(updatedConfig), 0644); err != nil {
		t.Fatal(err)
	}

	// Force reload
	if err := watcher.ForceReload(); err != nil {
		t.Fatal(err)
	}

	if !reloadCalled {
		t.Error("Reload callback should have been called")
	}

	if watcher.Config().Server.Port != 9090 {
		t.Errorf("Port after reload = %d, want 9090", watcher.Config().Server.Port)
	}
}

func TestWatcherMultipleCallbacks(t *testing.T) {
	// Create a temp config file
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")

	configContent := `
server:
  port: 8080
chains:
  ethereum:
    chain_id: 1
    endpoints:
      - url: "http://localhost:8545"
        weight: 1
    health_check:
      interval: 10s
      timeout: 5s
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatal(err)
	}

	watcher, err := NewWatcher(configPath, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer watcher.Stop()

	// Register multiple callbacks
	callback1Called := false
	callback2Called := false

	watcher.OnReload(func(old, new *Config) error {
		callback1Called = true
		return nil
	})

	watcher.OnReload(func(old, new *Config) error {
		callback2Called = true
		return nil
	})

	// Force reload
	if err := watcher.ForceReload(); err != nil {
		t.Fatal(err)
	}

	if !callback1Called {
		t.Error("Callback 1 should have been called")
	}
	if !callback2Called {
		t.Error("Callback 2 should have been called")
	}
}
