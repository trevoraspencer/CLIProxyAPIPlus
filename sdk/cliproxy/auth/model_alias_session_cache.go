package auth

import (
	"strings"
	"sync"
	"time"
)

// modelAliasSessionEntry stores a session-to-model binding with expiration.
type modelAliasSessionEntry struct {
	model     string
	expiresAt time.Time
}

func makeModelAliasSessionKey(sessionID, aliasName string) string {
	return strings.TrimSpace(sessionID) + ":" + strings.ToLower(strings.TrimSpace(aliasName))
}

// ModelAliasSessionCache provides TTL-based session-to-model mapping for alias pool fill-first strategy.
// It pins a session+alias to a specific upstream model until exhaustion or TTL expiry.
type ModelAliasSessionCache struct {
	mu      sync.RWMutex
	entries map[string]modelAliasSessionEntry
	ttl     time.Duration
	stopCh  chan struct{}
}

// NewModelAliasSessionCache creates a cache with the specified TTL.
// A background goroutine periodically cleans expired entries.
func NewModelAliasSessionCache(ttl time.Duration) *ModelAliasSessionCache {
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	c := &ModelAliasSessionCache{
		entries: make(map[string]modelAliasSessionEntry),
		ttl:     ttl,
		stopCh:  make(chan struct{}),
	}
	go c.cleanupLoop()
	return c
}

// Get retrieves the model pinned for a session+alias combination, if still valid.
func (c *ModelAliasSessionCache) Get(sessionID, aliasName string) (string, bool) {
	if sessionID == "" || aliasName == "" {
		return "", false
	}
	key := makeModelAliasSessionKey(sessionID, aliasName)
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok {
		return "", false
	}
	if time.Now().After(entry.expiresAt) {
		c.mu.Lock()
		delete(c.entries, key)
		c.mu.Unlock()
		return "", false
	}
	return entry.model, true
}

// Set pins a session+alias to a specific model with TTL refresh.
func (c *ModelAliasSessionCache) Set(sessionID, aliasName, model string) {
	if sessionID == "" || aliasName == "" || model == "" {
		return
	}
	key := makeModelAliasSessionKey(sessionID, aliasName)
	c.mu.Lock()
	c.entries[key] = modelAliasSessionEntry{
		model:     model,
		expiresAt: time.Now().Add(c.ttl),
	}
	c.mu.Unlock()
}

// Invalidate removes a specific session+alias binding.
func (c *ModelAliasSessionCache) Invalidate(sessionID, aliasName string) {
	if sessionID == "" || aliasName == "" {
		return
	}
	key := makeModelAliasSessionKey(sessionID, aliasName)
	c.mu.Lock()
	delete(c.entries, key)
	c.mu.Unlock()
}

// Stop terminates the background cleanup goroutine.
func (c *ModelAliasSessionCache) Stop() {
	select {
	case <-c.stopCh:
	default:
		close(c.stopCh)
	}
}

func (c *ModelAliasSessionCache) cleanupLoop() {
	ticker := time.NewTicker(c.ttl / 2)
	defer ticker.Stop()
	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.cleanup()
		}
	}
}

func (c *ModelAliasSessionCache) cleanup() {
	now := time.Now()
	c.mu.Lock()
	for key, entry := range c.entries {
		if now.After(entry.expiresAt) {
			delete(c.entries, key)
		}
	}
	c.mu.Unlock()
}
