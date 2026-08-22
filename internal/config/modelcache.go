package config

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/matteo-psnt/termwise/internal/provider"
)

const (
	modelCacheTTL            = 6 * time.Hour
	modelCacheRetention      = 7 * 24 * time.Hour
	modelCacheRefreshTimeout = 30 * time.Second
	modelMemoryCacheMax      = 32
	modelDiskCacheMax        = 64
)

type modelCacheFetcher func(context.Context) ([]provider.Model, error)

type modelCacheDescriptor struct {
	Key      string
	Provider string
	BaseURL  string
	AuthHash string
}

type providerModelsCache struct {
	mu       sync.Mutex
	diskMu   sync.Mutex
	entries  map[string]*modelMemoEntry
	inFlight map[string]*modelFetchCall
	now      func() time.Time
	path     func() (string, error)
}

type modelMemoEntry struct {
	desc       modelCacheDescriptor
	models     []provider.Model
	fetchedAt  time.Time
	lastUsedAt time.Time
	refreshing bool
}

type modelFetchCall struct {
	done   chan struct{}
	models []provider.Model
	err    error
}

type modelCacheFile struct {
	Entries map[string]modelDiskEntry `yaml:"entries,omitempty"`
}

type modelDiskEntry struct {
	Provider   string        `yaml:"provider"`
	BaseURL    string        `yaml:"base_url,omitempty"`
	AuthHash   string        `yaml:"auth_hash"`
	Models     []cachedModel `yaml:"models"`
	FetchedAt  time.Time     `yaml:"fetched_at"`
	LastUsedAt time.Time     `yaml:"last_used_at"`
}

type cachedModel struct {
	ID          string `yaml:"id"`
	DisplayName string `yaml:"display_name,omitempty"`
}

var defaultProviderModelsCache = newProviderModelsCache(DefaultModelsCachePath, time.Now)

func newProviderModelsCache(path func() (string, error), now func() time.Time) *providerModelsCache {
	return &providerModelsCache{
		entries:  make(map[string]*modelMemoEntry),
		inFlight: make(map[string]*modelFetchCall),
		now:      now,
		path:     path,
	}
}

func DefaultModelsCachePath() (string, error) {
	cfgPath, err := DefaultConfigPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(cfgPath), "models-cache.yaml"), nil
}

func modelCacheDescriptorFor(providerName string, auth ResolvedAuth) modelCacheDescriptor {
	authSum := sha256.Sum256([]byte(auth.APIKey))
	authHash := hex.EncodeToString(authSum[:])

	keySum := sha256.Sum256([]byte(providerName + "\x00" + auth.BaseURL + "\x00" + authHash))
	return modelCacheDescriptor{
		Key:      hex.EncodeToString(keySum[:]),
		Provider: providerName,
		BaseURL:  auth.BaseURL,
		AuthHash: authHash,
	}
}

func (c *providerModelsCache) getOrFetch(ctx context.Context, desc modelCacheDescriptor, fetch modelCacheFetcher) ([]provider.Model, error) {
	now := c.now()

	c.mu.Lock()
	c.evictMemoryLocked(now)
	if models, refresh, ok := c.lookupMemoryLocked(desc.Key, now); ok {
		var entry *modelMemoEntry
		if refresh {
			entry = c.entries[desc.Key]
			entry.refreshing = true
		}
		c.mu.Unlock()
		if refresh {
			c.startRefresh(desc, entry, fetch)
		}
		return models, nil
	}
	if call, ok := c.inFlight[desc.Key]; ok {
		c.mu.Unlock()
		return waitForModelFetch(ctx, call)
	}
	c.mu.Unlock()

	diskEntry, ok := c.loadDiskEntry(desc.Key)

	c.mu.Lock()
	c.evictMemoryLocked(now)
	if models, refresh, ok := c.lookupMemoryLocked(desc.Key, now); ok {
		var entry *modelMemoEntry
		if refresh {
			entry = c.entries[desc.Key]
			entry.refreshing = true
		}
		c.mu.Unlock()
		if refresh {
			c.startRefresh(desc, entry, fetch)
		}
		return models, nil
	}
	if call, ok := c.inFlight[desc.Key]; ok {
		c.mu.Unlock()
		return waitForModelFetch(ctx, call)
	}
	if ok && now.Sub(diskEntry.FetchedAt) <= modelCacheRetention && len(diskEntry.Models) > 0 {
		entry := &modelMemoEntry{
			desc:       desc,
			models:     diskEntry.toProviderModels(),
			fetchedAt:  diskEntry.FetchedAt,
			lastUsedAt: now,
		}
		c.entries[desc.Key] = entry
		c.evictMemoryLocked(now)

		models, refresh, _ := c.lookupMemoryLocked(desc.Key, now)
		if refresh {
			entry.refreshing = true
		}
		c.mu.Unlock()
		if refresh {
			c.startRefresh(desc, entry, fetch)
		}
		return models, nil
	}

	call := &modelFetchCall{done: make(chan struct{})}
	c.inFlight[desc.Key] = call
	c.mu.Unlock()

	models, err := fetch(ctx)
	now = c.now()

	c.mu.Lock()
	delete(c.inFlight, desc.Key)
	if err == nil {
		c.entries[desc.Key] = &modelMemoEntry{
			desc:       desc,
			models:     cloneModels(models),
			fetchedAt:  now,
			lastUsedAt: now,
		}
		c.evictMemoryLocked(now)
	}
	call.models = cloneModels(models)
	call.err = err
	close(call.done)
	c.mu.Unlock()

	if err == nil {
		_ = c.saveDiskEntry(desc, models, now)
	}
	return models, err
}

func (c *providerModelsCache) lookupMemoryLocked(key string, now time.Time) ([]provider.Model, bool, bool) {
	entry, ok := c.entries[key]
	if !ok || len(entry.models) == 0 {
		return nil, false, false
	}
	entry.lastUsedAt = now
	if now.Sub(entry.fetchedAt) <= modelCacheTTL {
		return cloneModels(entry.models), false, true
	}
	if entry.refreshing {
		return cloneModels(entry.models), false, true
	}
	return cloneModels(entry.models), true, true
}

func (c *providerModelsCache) startRefresh(desc modelCacheDescriptor, entry *modelMemoEntry, fetch modelCacheFetcher) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), modelCacheRefreshTimeout)
		defer cancel()

		models, err := fetch(ctx)
		now := c.now()

		c.mu.Lock()
		current := c.entries[desc.Key]
		if current != entry {
			entry.refreshing = false
			c.mu.Unlock()
			return
		}
		current.refreshing = false
		if err != nil {
			c.mu.Unlock()
			return
		}
		current.models = cloneModels(models)
		current.fetchedAt = now
		current.lastUsedAt = now
		current.desc = desc
		c.evictMemoryLocked(now)
		c.mu.Unlock()

		_ = c.saveDiskEntry(desc, models, now)
	}()
}

func (c *providerModelsCache) evictMemoryLocked(now time.Time) {
	evictCacheEntries(c.entries, func(e *modelMemoEntry) bool { return len(e.models) == 0 }, modelMemoryCacheMax, now)
}

func (e *modelMemoEntry) lastSeenAt() time.Time { return latestOf(e.fetchedAt, e.lastUsedAt) }

func (c *providerModelsCache) loadDiskEntry(key string) (modelDiskEntry, bool) {
	path, err := c.path()
	if err != nil {
		return modelDiskEntry{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return modelDiskEntry{}, false
	}

	var file modelCacheFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return modelDiskEntry{}, false
	}
	entry, ok := file.Entries[key]
	return entry, ok
}

func (c *providerModelsCache) saveDiskEntry(desc modelCacheDescriptor, models []provider.Model, now time.Time) error {
	if len(models) == 0 {
		return nil
	}

	path, err := c.path()
	if err != nil {
		return err
	}

	c.diskMu.Lock()
	defer c.diskMu.Unlock()

	file, err := loadModelCacheFile(path)
	if err != nil {
		return err
	}
	if file.Entries == nil {
		file.Entries = make(map[string]modelDiskEntry)
	}
	file.Entries[desc.Key] = modelDiskEntry{
		Provider:   desc.Provider,
		BaseURL:    desc.BaseURL,
		AuthHash:   desc.AuthHash,
		Models:     toCachedModels(models),
		FetchedAt:  now,
		LastUsedAt: now,
	}
	evictDiskEntries(file.Entries, now)
	return saveModelCacheFile(path, file)
}

func loadModelCacheFile(path string) (modelCacheFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return modelCacheFile{}, nil
		}
		return modelCacheFile{}, fmt.Errorf("reading model cache: %w", err)
	}

	var file modelCacheFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return modelCacheFile{}, fmt.Errorf("parsing model cache: %w", err)
	}
	return file, nil
}

func saveModelCacheFile(path string, file modelCacheFile) error {
	data, err := yaml.Marshal(file)
	if err != nil {
		return fmt.Errorf("marshaling model cache: %w", err)
	}
	return atomicWriteFile(path, data)
}

func evictDiskEntries(entries map[string]modelDiskEntry, now time.Time) {
	evictCacheEntries(entries, func(e modelDiskEntry) bool { return len(e.Models) == 0 }, modelDiskCacheMax, now)
}

func (e modelDiskEntry) lastSeenAt() time.Time { return latestOf(e.FetchedAt, e.LastUsedAt) }

func latestOf(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

func evictCacheEntries[T interface{ lastSeenAt() time.Time }](
	entries map[string]T,
	isEmpty func(T) bool,
	limit int,
	now time.Time,
) {
	for key, entry := range entries {
		if isEmpty(entry) || now.Sub(entry.lastSeenAt()) > modelCacheRetention {
			delete(entries, key)
		}
	}
	if len(entries) <= limit {
		return
	}
	type candidate struct {
		key      string
		lastUsed time.Time
	}
	cs := make([]candidate, 0, len(entries))
	for key, entry := range entries {
		cs = append(cs, candidate{key: key, lastUsed: entry.lastSeenAt()})
	}
	sort.Slice(cs, func(i, j int) bool { return cs[i].lastUsed.Before(cs[j].lastUsed) })
	for _, c := range cs[:len(cs)-limit] {
		delete(entries, c.key)
	}
}

func (e modelDiskEntry) toProviderModels() []provider.Model {
	models := make([]provider.Model, 0, len(e.Models))
	for _, m := range e.Models {
		models = append(models, provider.Model{ID: m.ID, DisplayName: m.DisplayName})
	}
	return models
}

func toCachedModels(models []provider.Model) []cachedModel {
	out := make([]cachedModel, 0, len(models))
	for _, m := range models {
		out = append(out, cachedModel{ID: m.ID, DisplayName: m.DisplayName})
	}
	return out
}

func waitForModelFetch(ctx context.Context, call *modelFetchCall) ([]provider.Model, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-call.done:
		return cloneModels(call.models), call.err
	}
}

func cloneModels(models []provider.Model) []provider.Model {
	if len(models) == 0 {
		return nil
	}
	out := make([]provider.Model, len(models))
	copy(out, models)
	return out
}
