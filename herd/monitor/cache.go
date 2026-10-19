package monitor

import (
	"context"
	"sync"
	"time"
)

// Detail cache limits: entries that have not finished are re-fetched after
// cacheTTL; a refresh re-fetches entries older than cacheRefreshMin.
const (
	cacheTTL        = 30 * time.Second
	cacheRefreshMin = 5 * time.Second
	maxCacheEntries = 256
)

// cacheEntry is a cached detail or campaign.
type cacheEntry struct {
	value    any
	at       time.Time // when it was fetched
	used     time.Time // when it was last returned
	finished bool
}

// fetchCall is a fetch in progress; callers of the same key wait for it.
type fetchCall struct {
	done  chan struct{}
	entry *cacheEntry
	err   error
}

// detailCache caches run details and campaigns: at most maxCacheEntries
// (least recently used ones go first), one fetch per key at a time.
type detailCache struct {
	now func() time.Time

	mu       sync.Mutex
	entries  map[string]*cacheEntry
	inflight map[string]*fetchCall
}

func newDetailCache(now func() time.Time) *detailCache {
	return &detailCache{now: now, entries: map[string]*cacheEntry{}, inflight: map[string]*fetchCall{}}
}

// get returns the entry of key, calling fetch when it is missing or stale:
// finished entries stay until a refresh, others for cacheTTL; a refresh
// re-fetches entries older than cacheRefreshMin. The fetch runs detached
// from ctx with its own timeout, so that a caller that goes away does not
// cancel it for the others waiting; ctx only bounds this caller's wait.
func (c *detailCache) get(ctx context.Context, key string, refresh bool, timeout time.Duration,
	fetch func(ctx context.Context) (any, bool, error),
) (*cacheEntry, error) {
	c.mu.Lock()

	if e := c.entries[key]; e != nil {
		age := c.now().Sub(e.at)
		if (refresh && age < cacheRefreshMin) || (!refresh && (e.finished || age < cacheTTL)) {
			e.used = c.now()
			c.mu.Unlock()

			return e, nil
		}
	}

	call := c.inflight[key]
	if call == nil {
		call = &fetchCall{done: make(chan struct{})}
		c.inflight[key] = call

		go c.fetch(context.WithoutCancel(ctx), key, call, timeout, fetch)
	}

	c.mu.Unlock()

	select {
	case <-call.done:
		return call.entry, call.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *detailCache) fetch(ctx context.Context, key string, call *fetchCall, timeout time.Duration,
	fetch func(ctx context.Context) (any, bool, error),
) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	v, finished, err := fetch(ctx)

	c.mu.Lock()

	if err == nil {
		now := c.now()
		call.entry = &cacheEntry{value: v, at: now, used: now, finished: finished}
		c.entries[key] = call.entry
		c.evict()
	}

	call.err = err
	delete(c.inflight, key)
	c.mu.Unlock()
	close(call.done)
}

// evict drops the least recently used entries beyond maxCacheEntries.
func (c *detailCache) evict() {
	for len(c.entries) > maxCacheEntries {
		var (
			oldest string
			at     time.Time
		)

		for k, e := range c.entries {
			if oldest == "" || e.used.Before(at) {
				oldest, at = k, e.used
			}
		}

		delete(c.entries, oldest)
	}
}
