package catalog

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/laposa/tmock/app-go/internal/logx"
)

// Options configures a Cache.
type Options struct {
	TTL       time.Duration     // CACHE_TTL; a snapshot older than this is rebuilt
	Disabled  bool              // DISABLE_CACHE: every Get builds a fresh snapshot
	Overrides map[string]string // SERVICES_UPSTREAM_OVERRIDES
	Logger    *slog.Logger      // nil means slog.Default()
}

const (
	// rebuildTimeout bounds one rebuild. It does not use the request's context, so a
	// client that goes away does not fail the rebuild the other requests wait for.
	rebuildTimeout = 10 * time.Second
	// retryAfterFailure is how long a failed rebuild is not retried while an older
	// snapshot can still be served, so a database outage is not one error per request.
	retryAfterFailure = time.Second
)

// Cache holds the current snapshot and rebuilds it lazily (spec §7.2).
type Cache struct {
	loader Loader
	opts   Options
	logger *slog.Logger

	cur atomic.Pointer[entry]
	// gen counts invalidations. A snapshot built from data read before the latest
	// invalidation is stale, even when its rebuild finished after it.
	gen atomic.Uint64
	// rebuild holds a token while a rebuild runs, so only one runs at a time. A channel
	// rather than a sync.Mutex: a goroutine waiting on a channel counts as blocked for
	// testing/synctest, one waiting on a mutex does not.
	rebuild chan struct{}
	// failedAt is when the last rebuild failed (zero after a success), and failure its
	// error. Guarded by rebuild.
	failedAt time.Time
	failure  error
}

type entry struct {
	snap  *Snapshot
	gen   uint64
	built time.Time
}

// NewCache returns a cache that builds snapshots from loader. Nothing is read until the
// first Get.
func NewCache(loader Loader, opts Options) *Cache {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Cache{loader: loader, opts: opts, logger: logger, rebuild: make(chan struct{}, 1)}
}

// Invalidate marks the current snapshot as stale: the next Get rebuilds it, so a change
// is visible to the next proxy request. internal/app calls it after every mutation of
// services, scenarios and clients.
func (c *Cache) Invalidate() { c.gen.Add(1) }

// Get returns a snapshot that is fresh (younger than the TTL and built after the latest
// Invalidate), rebuilding it if needed.
//
// While another request is rebuilding, Get returns the previous snapshot instead of
// waiting, but only if that snapshot is merely past its TTL. An invalidated snapshot is
// never served that way, so a change is visible on the next request: Get waits for the
// rebuild (or until ctx is done) and, if that rebuild read its data before the change,
// rebuilds again.
//
// If a rebuild fails, the failure is logged and the previous snapshot is returned; the
// error is returned only when there is no snapshot at all. After a failure, rebuilding is
// not retried for retryAfterFailure, so requests queued behind a failing rebuild do not
// each try again.
func (c *Cache) Get(ctx context.Context) (*Snapshot, error) {
	if c.opts.Disabled {
		return c.build(ctx)
	}
	e := c.cur.Load()
	if c.fresh(e) {
		return e.snap, nil
	}
	select {
	case c.rebuild <- struct{}{}:
	default:
		if e != nil && e.gen == c.gen.Load() {
			return e.snap, nil // only past its TTL: use it while the rebuild runs
		}
		select { // no snapshot yet, or an invalidated one: wait for the rebuild
		case c.rebuild <- struct{}{}:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	defer func() { <-c.rebuild }()

	// Another request may have rebuilt it while this one waited.
	if e = c.cur.Load(); c.fresh(e) {
		return e.snap, nil
	}
	if !c.failedAt.IsZero() && time.Since(c.failedAt) < retryAfterFailure {
		if e != nil {
			return e.snap, nil
		}
		return nil, c.failure
	}
	gen := c.gen.Load()
	snap, err := c.build(ctx)
	if err != nil {
		c.failedAt, c.failure = time.Now(), err
		if e != nil {
			return e.snap, nil
		}
		return nil, err
	}
	c.failedAt, c.failure = time.Time{}, nil
	c.cur.Store(&entry{snap: snap, gen: gen, built: time.Now()})
	return snap, nil
}

// fresh reports whether e can be served without a rebuild.
func (c *Cache) fresh(e *entry) bool {
	return e != nil && e.gen == c.gen.Load() && time.Since(e.built) < c.opts.TTL
}

// build loads the data and builds a snapshot, logging tmock.catalog.rebuilt or
// tmock.catalog.rebuild_failed (spec §2.4.2).
func (c *Cache) build(ctx context.Context) (*Snapshot, error) {
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), rebuildTimeout)
	defer cancel()
	data, err := c.loader.Load(ctx)
	if err != nil {
		c.logger.LogAttrs(ctx, slog.LevelError, "catalog rebuild failed: "+err.Error(),
			slog.String(logx.AttrEventName, logx.EventCatalogFailed),
			slog.String(logx.AttrErrorType, fmt.Sprintf("%T", err)),
			slog.String(logx.AttrExceptionMessage, err.Error()),
		)
		return nil, fmt.Errorf("catalog: %w", err)
	}
	snap := Build(data, c.opts.Overrides, c.logger)
	services, scenarios, clients := snap.Counts()
	ms := time.Since(start).Milliseconds()
	c.logger.LogAttrs(ctx, slog.LevelDebug,
		fmt.Sprintf("catalog rebuilt in %dms: %d services, %d scenarios, %d clients", ms, services, scenarios, clients),
		slog.String(logx.AttrEventName, logx.EventCatalogRebuilt),
		slog.Int64(logx.AttrDurationMS, ms),
		slog.Int(logx.AttrCatalogServices, services),
		slog.Int(logx.AttrCatalogScenarios, scenarios),
		slog.Int(logx.AttrCatalogClients, clients),
	)
	return snap, nil
}
