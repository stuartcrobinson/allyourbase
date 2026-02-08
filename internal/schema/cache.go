package schema

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/jackc/pgx/v5/pgxpool"
)

// CacheHolder provides thread-safe access to the current SchemaCache.
// Reads are lock-free (atomic pointer load). Writes build a new immutable
// SchemaCache and swap it in atomically.
type CacheHolder struct {
	cache   atomic.Pointer[SchemaCache]
	mu      sync.Mutex   // serializes reloads
	loading atomic.Bool  // prevents concurrent reloads
	pool    *pgxpool.Pool
	logger  *slog.Logger
	ready   chan struct{} // closed after the first successful load
}

// NewCacheHolder creates a CacheHolder. Call Load() to perform the initial introspection.
func NewCacheHolder(pool *pgxpool.Pool, logger *slog.Logger) *CacheHolder {
	return &CacheHolder{
		pool:   pool,
		logger: logger,
		ready:  make(chan struct{}),
	}
}

// Ready returns a channel that is closed once the first schema load completes.
func (h *CacheHolder) Ready() <-chan struct{} {
	return h.ready
}

// Get returns the current schema cache. Lock-free, safe for concurrent use.
// Returns nil if the cache has not been loaded yet.
func (h *CacheHolder) Get() *SchemaCache {
	return h.cache.Load()
}

// Load performs the initial schema introspection. Must be called before Get().
func (h *CacheHolder) Load(ctx context.Context) error {
	return h.Reload(ctx)
}

// SetForTesting directly sets the schema cache. Intended for unit tests that
// cannot provide a real database connection.
func (h *CacheHolder) SetForTesting(sc *SchemaCache) {
	first := h.cache.Load() == nil
	h.cache.Store(sc)
	if first && sc != nil {
		close(h.ready)
	}
}

// Reload re-introspects the database and atomically swaps the cache.
// Returns immediately if a reload is already in progress.
func (h *CacheHolder) Reload(ctx context.Context) error {
	if !h.loading.CompareAndSwap(false, true) {
		h.logger.Debug("schema reload already in progress, skipping")
		return nil
	}
	defer h.loading.Store(false)

	h.mu.Lock()
	defer h.mu.Unlock()

	sc, err := BuildCache(ctx, h.pool)
	if err != nil {
		return fmt.Errorf("building schema cache: %w", err)
	}

	tableCount := len(sc.Tables)
	first := h.cache.Load() == nil
	h.cache.Store(sc)

	// Signal readiness on first successful load.
	if first {
		close(h.ready)
	}

	h.logger.Info("schema cache loaded",
		"tables", tableCount,
		"schemas", sc.Schemas,
		"builtAt", sc.BuiltAt,
	)

	return nil
}
