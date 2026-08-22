package config

import (
	"context"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/matteo-psnt/termwise/internal/provider"
)

func TestProviderModelsCacheDeduplicatesConcurrentColdMisses(t *testing.T) {
	t.Parallel()

	cache := newProviderModelsCache(
		func() (string, error) { return filepath.Join(t.TempDir(), "models-cache.yaml"), nil },
		time.Now,
	)
	desc := modelCacheDescriptor{Key: "shared", Provider: "openai", AuthHash: "abc"}

	var calls atomic.Int32
	release := make(chan struct{})
	fetch := func(context.Context) ([]provider.Model, error) { //nolint:unparam // signature matches getOrFetch fetcher type
		calls.Add(1)
		<-release
		return []provider.Model{{ID: "gpt-4o"}}, nil
	}

	results := make([][]provider.Model, 2)
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range 2 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = cache.getOrFetch(context.Background(), desc, fetch)
		}(i)
	}

	waitUntil(t, "cold miss fetch starts", func() bool { return calls.Load() == 1 })
	close(release)
	wg.Wait()

	if got := calls.Load(); got != 1 {
		t.Fatalf("fetch called %d times; want 1", got)
	}
	for i, err := range errs {
		if err != nil {
			t.Fatalf("result %d err = %v", i, err)
		}
		if len(results[i]) != 1 || results[i][0].ID != "gpt-4o" {
			t.Fatalf("result %d = %#v; want gpt-4o", i, results[i])
		}
	}
}

func TestProviderModelsCacheReturnsStaleDiskEntryAndRefreshes(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "models-cache.yaml")
	cache := newProviderModelsCache(
		func() (string, error) { return path, nil },
		func() time.Time { return now },
	)
	desc := modelCacheDescriptor{Key: "stale", Provider: "anthropic", AuthHash: "def"}

	if err := cache.saveDiskEntry(desc, []provider.Model{{ID: "claude-old"}}, now.Add(-modelCacheTTL-time.Minute)); err != nil {
		t.Fatalf("seed disk cache: %v", err)
	}

	started := make(chan struct{}, 1)
	release := make(chan struct{})
	fetch := func(context.Context) ([]provider.Model, error) {
		started <- struct{}{}
		<-release
		return []provider.Model{{ID: "claude-new"}}, nil
	}

	got, err := cache.getOrFetch(context.Background(), desc, fetch)
	if err != nil {
		t.Fatalf("getOrFetch: %v", err)
	}
	if len(got) != 1 || got[0].ID != "claude-old" {
		t.Fatalf("returned %#v; want stale cached models", got)
	}

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("background refresh did not start")
	}
	close(release)

	waitUntil(t, "memory refresh completes", func() bool {
		cache.mu.Lock()
		defer cache.mu.Unlock()
		entry := cache.entries[desc.Key]
		return entry != nil && len(entry.models) == 1 && entry.models[0].ID == "claude-new"
	})

	waitUntil(t, "disk refresh completes", func() bool {
		file, err := loadModelCacheFile(path)
		if err != nil {
			return false
		}
		entry, ok := file.Entries[desc.Key]
		if !ok {
			return false
		}
		models := entry.toProviderModels()
		return len(models) == 1 && models[0].ID == "claude-new"
	})
}

func TestProviderModelsCacheBackgroundRefreshDoesNotOverwriteReplacedEntry(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)
	cache := newProviderModelsCache(
		func() (string, error) { return filepath.Join(t.TempDir(), "models-cache.yaml"), nil },
		func() time.Time { return now },
	)
	desc := modelCacheDescriptor{Key: "identity", Provider: "openai", AuthHash: "ghi"}

	staleEntry := &modelMemoEntry{
		desc:       desc,
		models:     []provider.Model{{ID: "stale"}},
		fetchedAt:  now.Add(-modelCacheTTL - time.Minute),
		lastUsedAt: now.Add(-modelCacheTTL - time.Minute),
	}
	cache.entries[desc.Key] = staleEntry

	started := make(chan struct{}, 1)
	release := make(chan struct{})
	fetch := func(context.Context) ([]provider.Model, error) {
		started <- struct{}{}
		<-release
		return []provider.Model{{ID: "refreshed"}}, nil
	}

	got, err := cache.getOrFetch(context.Background(), desc, fetch)
	if err != nil {
		t.Fatalf("getOrFetch: %v", err)
	}
	if len(got) != 1 || got[0].ID != "stale" {
		t.Fatalf("returned %#v; want stale entry", got)
	}

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("background refresh did not start")
	}

	cache.mu.Lock()
	cache.entries[desc.Key] = &modelMemoEntry{
		desc:       desc,
		models:     []provider.Model{{ID: "replacement"}},
		fetchedAt:  now,
		lastUsedAt: now,
	}
	cache.mu.Unlock()

	close(release)

	waitUntil(t, "background refresh goroutine exits", func() bool {
		cache.mu.Lock()
		defer cache.mu.Unlock()
		return !staleEntry.refreshing
	})

	cache.mu.Lock()
	defer cache.mu.Unlock()
	entry := cache.entries[desc.Key]
	if entry == nil || len(entry.models) != 1 || entry.models[0].ID != "replacement" {
		t.Fatalf("entry = %#v; want replacement entry preserved", entry)
	}
}

func TestSaveDiskEntryEvictsExpiredAndOldestEntries(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "models-cache.yaml")
	cache := newProviderModelsCache(
		func() (string, error) { return path, nil },
		func() time.Time { return now },
	)

	file := modelCacheFile{Entries: map[string]modelDiskEntry{
		"expired": {
			Provider:   "openai",
			AuthHash:   "expired",
			Models:     []cachedModel{{ID: "too-old"}},
			FetchedAt:  now.Add(-modelCacheRetention - time.Hour),
			LastUsedAt: now.Add(-modelCacheRetention - time.Hour),
		},
	}}
	for i := range modelDiskCacheMax {
		key := "keep-" + time.Date(2000+i, 1, 1, 0, 0, 0, 0, time.UTC).Format("2006")
		file.Entries[key] = modelDiskEntry{
			Provider:   "openai",
			AuthHash:   key,
			Models:     []cachedModel{{ID: key}},
			FetchedAt:  now.Add(-time.Duration(i+1) * time.Minute),
			LastUsedAt: now.Add(-time.Duration(i+1) * time.Minute),
		}
	}
	if err := saveModelCacheFile(path, file); err != nil {
		t.Fatalf("seed model cache: %v", err)
	}

	desc := modelCacheDescriptor{Key: "fresh", Provider: "openai", AuthHash: "fresh"}
	if err := cache.saveDiskEntry(desc, []provider.Model{{ID: "fresh"}}, now); err != nil {
		t.Fatalf("saveDiskEntry: %v", err)
	}

	loaded, err := loadModelCacheFile(path)
	if err != nil {
		t.Fatalf("loadModelCacheFile: %v", err)
	}
	if _, ok := loaded.Entries["expired"]; ok {
		t.Fatal("expired entry should have been evicted")
	}
	if got := len(loaded.Entries); got != modelDiskCacheMax {
		t.Fatalf("disk cache size = %d; want %d", got, modelDiskCacheMax)
	}
	if _, ok := loaded.Entries["fresh"]; !ok {
		t.Fatal("fresh entry missing after eviction")
	}
}

func waitUntil(t *testing.T, name string, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", name)
}
