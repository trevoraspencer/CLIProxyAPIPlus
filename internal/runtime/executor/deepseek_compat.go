package executor

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
)

const (
	deepSeekProviderName              = "deepseek"
	defaultDeepSeekReasoningCacheTTL  = 30 * time.Minute
	defaultDeepSeekReasoningCacheSize = 512
)

var deepSeekAllowedHosts = map[string]struct{}{
	"api.deepseek.com": {},
}

type deepSeekCompatIdentity struct {
	Enabled      bool
	Provider     string
	AuthScope    string
	Model        string
	SessionScope string
}

func newDeepSeekCompatIdentity(provider string, auth *cliproxyauth.Auth, compat *config.OpenAICompatibility, baseURL string, model string, opts cliproxyexecutor.Options) deepSeekCompatIdentity {
	providerScope := canonicalDeepSeekScope(provider)
	authProvider := ""
	authScope := ""
	if auth != nil {
		authProvider = canonicalDeepSeekScope(auth.Provider)
		authScope = deepSeekAuthScope(auth)
	}
	configName := ""
	configBaseURL := ""
	if compat != nil {
		configName = canonicalDeepSeekScope(compat.Name)
		configBaseURL = compat.BaseURL
	}
	attrCompatName := ""
	attrProviderKey := ""
	if auth != nil && auth.Attributes != nil {
		attrCompatName = canonicalDeepSeekScope(auth.Attributes["compat_name"])
		attrProviderKey = canonicalDeepSeekScope(auth.Attributes["provider_key"])
		if strings.TrimSpace(baseURL) == "" {
			baseURL = auth.Attributes["base_url"]
		}
	}

	enabled := providerScope == deepSeekProviderName ||
		authProvider == deepSeekProviderName ||
		configName == deepSeekProviderName ||
		attrCompatName == deepSeekProviderName ||
		attrProviderKey == deepSeekProviderName ||
		isDeepSeekAllowedBaseURL(baseURL) ||
		isDeepSeekAllowedBaseURL(configBaseURL)

	return deepSeekCompatIdentity{
		Enabled:      enabled,
		Provider:     firstNonEmpty(providerScope, configName, authProvider, attrCompatName, attrProviderKey),
		AuthScope:    authScope,
		Model:        strings.TrimSpace(model),
		SessionScope: deepSeekExecutionSessionScope(opts),
	}
}

func canonicalDeepSeekScope(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func isDeepSeekAllowedBaseURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed == nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil {
		return false
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	_, ok := deepSeekAllowedHosts[host]
	return ok
}

func deepSeekExecutionSessionScope(opts cliproxyexecutor.Options) string {
	if len(opts.Metadata) == 0 {
		return ""
	}
	raw, ok := opts.Metadata[cliproxyexecutor.ExecutionSessionMetadataKey]
	if !ok {
		return ""
	}
	switch v := raw.(type) {
	case string:
		return strings.TrimSpace(v)
	case []byte:
		return strings.TrimSpace(string(v))
	default:
		return ""
	}
}

func deepSeekAuthScope(auth *cliproxyauth.Auth) string {
	if auth == nil {
		return "auth:none"
	}
	if value := strings.TrimSpace(auth.ID); value != "" {
		return "id:" + value
	}
	if value := strings.TrimSpace(auth.Index); value != "" {
		return "index:" + value
	}
	return "auth:unspecified"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

type deepSeekReasoningTurn struct {
	Reasoning         string
	ToolCallIDs       []string
	AssistantTurnHash string
}

type deepSeekReasoningCacheKey struct {
	Provider          string
	AuthScope         string
	Model             string
	SessionScope      string
	ToolCallIDHash    string
	AssistantTurnHash string
}

type deepSeekReasoningCacheEntry struct {
	key       deepSeekReasoningCacheKey
	reasoning string
	expiresAt time.Time
	createdAt time.Time
}

type deepSeekReasoningCache struct {
	mu      sync.RWMutex
	ttl     time.Duration
	maxSize int
	now     func() time.Time
	entries map[deepSeekReasoningCacheKey]deepSeekReasoningCacheEntry
}

func newDeepSeekReasoningCache(ttl time.Duration, maxSize int) *deepSeekReasoningCache {
	if ttl <= 0 {
		ttl = defaultDeepSeekReasoningCacheTTL
	}
	if maxSize <= 0 {
		maxSize = defaultDeepSeekReasoningCacheSize
	}
	return &deepSeekReasoningCache{
		ttl:     ttl,
		maxSize: maxSize,
		now:     time.Now,
		entries: make(map[deepSeekReasoningCacheKey]deepSeekReasoningCacheEntry),
	}
}

func (c *deepSeekReasoningCache) Put(identity deepSeekCompatIdentity, turn deepSeekReasoningTurn) bool {
	if c == nil || !identity.Enabled {
		return false
	}
	reasoning := strings.TrimSpace(turn.Reasoning)
	if reasoning == "" {
		return false
	}
	key, ok := deepSeekReasoningKey(identity, turn)
	if !ok {
		return false
	}
	now := c.now()
	entry := deepSeekReasoningCacheEntry{
		key:       key,
		reasoning: turn.Reasoning,
		createdAt: now,
		expiresAt: now.Add(c.ttl),
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.evictExpiredLocked(now)
	c.entries[key] = entry
	c.evictOverflowLocked()
	return true
}

func (c *deepSeekReasoningCache) Get(identity deepSeekCompatIdentity, turn deepSeekReasoningTurn) (string, bool) {
	if c == nil || !identity.Enabled {
		return "", false
	}
	key, ok := deepSeekReasoningKey(identity, turn)
	if !ok {
		return "", false
	}
	now := c.now()

	c.mu.RLock()
	entry, found := c.entries[key]
	c.mu.RUnlock()
	if !found {
		return "", false
	}
	if !now.Before(entry.expiresAt) {
		c.mu.Lock()
		if current, ok := c.entries[key]; ok && !now.Before(current.expiresAt) {
			delete(c.entries, key)
		}
		c.mu.Unlock()
		return "", false
	}
	return entry.reasoning, true
}

func (c *deepSeekReasoningCache) Len() int {
	if c == nil {
		return 0
	}
	now := c.now()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.evictExpiredLocked(now)
	return len(c.entries)
}

func (c *deepSeekReasoningCache) evictExpiredLocked(now time.Time) {
	for key, entry := range c.entries {
		if !now.Before(entry.expiresAt) {
			delete(c.entries, key)
		}
	}
}

func (c *deepSeekReasoningCache) evictOverflowLocked() {
	for len(c.entries) > c.maxSize {
		var oldestKey deepSeekReasoningCacheKey
		var oldestTime time.Time
		first := true
		for key, entry := range c.entries {
			if first || entry.createdAt.Before(oldestTime) {
				first = false
				oldestKey = key
				oldestTime = entry.createdAt
			}
		}
		delete(c.entries, oldestKey)
	}
}

func deepSeekReasoningKey(identity deepSeekCompatIdentity, turn deepSeekReasoningTurn) (deepSeekReasoningCacheKey, bool) {
	provider := canonicalDeepSeekScope(identity.Provider)
	if provider == "" {
		provider = deepSeekProviderName
	}
	key := deepSeekReasoningCacheKey{
		Provider:     provider,
		AuthScope:    strings.TrimSpace(identity.AuthScope),
		Model:        strings.TrimSpace(identity.Model),
		SessionScope: strings.TrimSpace(identity.SessionScope),
	}
	if len(turn.ToolCallIDs) > 0 {
		key.ToolCallIDHash = hashDeepSeekParts(normalizedToolCallIDs(turn.ToolCallIDs)...)
		return key, true
	}
	if strings.TrimSpace(identity.SessionScope) == "" || strings.TrimSpace(turn.AssistantTurnHash) == "" {
		return deepSeekReasoningCacheKey{}, false
	}
	key.AssistantTurnHash = strings.TrimSpace(turn.AssistantTurnHash)
	return key, true
}

func normalizedToolCallIDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if value := strings.TrimSpace(id); value != "" {
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}

func hashDeepSeekParts(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(part))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func deepSeekReasoningTurnFromAssistantMessage(raw []byte) (deepSeekReasoningTurn, bool) {
	var msg struct {
		Role             string `json:"role"`
		Content          any    `json:"content"`
		ReasoningContent string `json:"reasoning_content"`
		ToolCalls        []struct {
			ID string `json:"id"`
		} `json:"tool_calls"`
	}
	if err := json.Unmarshal(raw, &msg); err != nil {
		return deepSeekReasoningTurn{}, false
	}
	if strings.TrimSpace(msg.Role) != "assistant" || strings.TrimSpace(msg.ReasoningContent) == "" || len(msg.ToolCalls) == 0 {
		return deepSeekReasoningTurn{}, false
	}
	ids := make([]string, 0, len(msg.ToolCalls))
	for _, toolCall := range msg.ToolCalls {
		if id := strings.TrimSpace(toolCall.ID); id != "" {
			ids = append(ids, id)
		}
	}
	turn := deepSeekReasoningTurn{
		Reasoning:         msg.ReasoningContent,
		ToolCallIDs:       ids,
		AssistantTurnHash: hashDeepSeekParts(string(raw)),
	}
	if len(turn.ToolCallIDs) == 0 && turn.AssistantTurnHash == "" {
		return deepSeekReasoningTurn{}, false
	}
	return turn, true
}
