/*
 * Copyright (c) 2025 Bima Kharisma Wicaksana
 * GitHub: https://github.com/bimakw
 *
 * Licensed under MIT License.
 * See LICENSE file for details.
 */

package config

import (
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog/log"
)

// ReloadCallback is called when config is reloaded
type ReloadCallback func(old, new *Config) error

// Watcher watches config file for changes
type Watcher struct {
	configPath string
	watcher    *fsnotify.Watcher
	config     *Config
	mu         sync.RWMutex
	callbacks  []ReloadCallback
	debounce   time.Duration
	stopCh     chan struct{}
}

// NewWatcher creates a new config watcher
func NewWatcher(configPath string, initialConfig *Config) (*Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	// Watch the directory, not the file directly (handles atomic renames)
	dir := filepath.Dir(configPath)
	if err := watcher.Add(dir); err != nil {
		watcher.Close()
		return nil, err
	}

	return &Watcher{
		configPath: configPath,
		watcher:    watcher,
		config:     initialConfig,
		debounce:   500 * time.Millisecond,
		stopCh:     make(chan struct{}),
	}, nil
}

// OnReload registers a callback for config reload events
func (w *Watcher) OnReload(cb ReloadCallback) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.callbacks = append(w.callbacks, cb)
}

// Config returns the current config
func (w *Watcher) Config() *Config {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.config
}

// Start starts watching for config changes
func (w *Watcher) Start() {
	go w.watch()
}

// Stop stops the watcher
func (w *Watcher) Stop() error {
	close(w.stopCh)
	return w.watcher.Close()
}

func (w *Watcher) watch() {
	var debounceTimer *time.Timer
	filename := filepath.Base(w.configPath)

	for {
		select {
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}

			// Only handle our config file
			if filepath.Base(event.Name) != filename {
				continue
			}

			// Handle write and create events (create handles atomic renames)
			if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
				continue
			}

			// Debounce rapid changes
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			debounceTimer = time.AfterFunc(w.debounce, func() {
				w.reload()
			})

		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			log.Error().Err(err).Msg("Config watcher error")

		case <-w.stopCh:
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			return
		}
	}
}

func (w *Watcher) reload() {
	log.Info().Str("path", w.configPath).Msg("Reloading config")

	newConfig, err := Load(w.configPath)
	if err != nil {
		log.Error().Err(err).Msg("Failed to reload config")
		return
	}

	w.mu.Lock()
	oldConfig := w.config
	w.config = newConfig
	callbacks := make([]ReloadCallback, len(w.callbacks))
	copy(callbacks, w.callbacks)
	w.mu.Unlock()

	// Notify callbacks
	for _, cb := range callbacks {
		if err := cb(oldConfig, newConfig); err != nil {
			log.Error().Err(err).Msg("Config reload callback failed")
		}
	}

	log.Info().Msg("Config reloaded successfully")
}

// ForceReload forces an immediate reload
func (w *Watcher) ForceReload() error {
	log.Info().Str("path", w.configPath).Msg("Force reloading config")

	newConfig, err := Load(w.configPath)
	if err != nil {
		return err
	}

	w.mu.Lock()
	oldConfig := w.config
	w.config = newConfig
	callbacks := make([]ReloadCallback, len(w.callbacks))
	copy(callbacks, w.callbacks)
	w.mu.Unlock()

	// Notify callbacks
	for _, cb := range callbacks {
		if err := cb(oldConfig, newConfig); err != nil {
			log.Error().Err(err).Msg("Config reload callback failed")
		}
	}

	log.Info().Msg("Config reloaded successfully")
	return nil
}
