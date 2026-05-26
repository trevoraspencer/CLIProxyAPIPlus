package executor

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	"github.com/tidwall/gjson"
)

const (
	deepSeekReasoningCacheDefaultMaxEntries = 1024
	deepSeekReasoningCacheDefaultTTL        = 30 * time.Minute
)

var deepSeekAllowedHosts = map[string]struct{}{
	"api.deepseek.com": {},
}

var defaultDeepSeekReasoningCache = newDeepSeekReasoningCache(deepSeekReasoningCacheDefaultMaxEntries, deepSeekReasoningCacheDefaultTTL)

type deepSeekReasoningScope struct {
	Provider string
	Auth     string
	Model    string
	Session  string
}

type deepSeekReasoningKey struct {
	deepSeekReasoningScope
	ToolCallIDs string
	TurnHash    string
}

type deepSeekReasoningEntry struct {
	Reasoning string
	CreatedAt time.Time
}

type deepSeekReasoningCache struct {
	mu         sync.RWMutex
	now        func() time.Time
	ttl        time.Duration
	maxEntries int
	entries    map[deepSeekReasoningKey]deepSeekReasoningEntry
}

func newDeepSeekReasoningCache(maxEntries int, ttl time.Duration) *deepSeekReasoningCache {
	if maxEntries <= 0 {
		maxEntries = 1
	}
	if ttl <= 0 {
		ttl = time.Minute
	}
	return &deepSeekReasoningCache{
		now:        time.Now,
		ttl:        ttl,
		maxEntries: maxEntries,
		entries:    make(map[deepSeekReasoningKey]deepSeekReasoningEntry),
	}
}

func (c *deepSeekReasoningCache) Store(key deepSeekReasoningKey, reasoning string) {
	if c == nil || strings.TrimSpace(reasoning) == "" || !key.valid() {
		return
	}
	now := c.now()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = deepSeekReasoningEntry{Reasoning: reasoning, CreatedAt: now}
	c.evictLocked(now)
}

func (c *deepSeekReasoningCache) Lookup(key deepSeekReasoningKey) (string, bool) {
	if c == nil || !key.valid() {
		return "", false
	}
	now := c.now()
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok {
		return "", false
	}
	if now.Sub(entry.CreatedAt) > c.ttl {
		c.mu.Lock()
		if current, okCurrent := c.entries[key]; okCurrent && now.Sub(current.CreatedAt) > c.ttl {
			delete(c.entries, key)
		}
		c.mu.Unlock()
		return "", false
	}
	return entry.Reasoning, true
}

func (c *deepSeekReasoningCache) Len() int {
	if c == nil {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

func (c *deepSeekReasoningCache) evictLocked(now time.Time) {
	for key, entry := range c.entries {
		if now.Sub(entry.CreatedAt) > c.ttl {
			delete(c.entries, key)
		}
	}
	for len(c.entries) > c.maxEntries {
		var oldestKey deepSeekReasoningKey
		var oldest time.Time
		first := true
		for key, entry := range c.entries {
			if first || entry.CreatedAt.Before(oldest) {
				oldestKey = key
				oldest = entry.CreatedAt
				first = false
			}
		}
		delete(c.entries, oldestKey)
	}
}

func (k deepSeekReasoningKey) valid() bool {
	return k.Provider != "" && k.Auth != "" && k.Model != "" && (k.ToolCallIDs != "" || (k.Session != "" && k.TurnHash != ""))
}

func (e *OpenAICompatExecutor) deepSeekReasoningEnabled(auth *cliproxyauth.Auth, baseURL string) bool {
	if deepSeekIdentityString(e.provider) {
		return true
	}
	if auth != nil {
		if deepSeekIdentityString(auth.Provider) {
			return true
		}
		for _, key := range []string{"compat_name", "provider_key", "provider"} {
			if auth.Attributes != nil && deepSeekIdentityString(auth.Attributes[key]) {
				return true
			}
		}
	}
	if compat := e.resolveCompatConfig(auth); compat != nil && deepSeekCompatConfig(compat) {
		return true
	}
	return deepSeekBaseURL(baseURL)
}

func deepSeekCompatConfig(compat *config.OpenAICompatibility) bool {
	if compat == nil {
		return false
	}
	return deepSeekIdentityString(compat.Name) || deepSeekBaseURL(compat.BaseURL)
}

func deepSeekIdentityString(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	return normalized == "deepseek" || normalized == "deepseek-api" || normalized == "deepseek-openai-compatibility"
}

func deepSeekBaseURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed == nil || parsed.Scheme == "" || parsed.Host == "" {
		return false
	}
	if parsed.User != nil {
		return false
	}
	host := parsed.Hostname()
	if host == "" || net.ParseIP(host) != nil {
		return false
	}
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	_, ok := deepSeekAllowedHosts[host]
	return ok
}

func deepSeekReasoningScopeFor(e *OpenAICompatExecutor, auth *cliproxyauth.Auth, model string, opts cliproxyexecutor.Options) deepSeekReasoningScope {
	provider := strings.ToLower(strings.TrimSpace(e.provider))
	if provider == "" && auth != nil {
		provider = strings.ToLower(strings.TrimSpace(auth.Provider))
	}
	authScope := ""
	if auth != nil {
		authScope = strings.TrimSpace(auth.ID)
		if authScope == "" {
			authScope = strings.TrimSpace(auth.Index)
		}
		if authScope == "" {
			authScope = auth.EnsureIndex()
		}
	}
	session := ""
	if opts.Metadata != nil {
		if raw, ok := opts.Metadata[cliproxyexecutor.ExecutionSessionMetadataKey]; ok {
			session = strings.TrimSpace(stringFromAny(raw))
		}
	}
	return deepSeekReasoningScope{
		Provider: provider,
		Auth:     authScope,
		Model:    strings.TrimSpace(model),
		Session:  session,
	}
}

func stringFromAny(raw any) string {
	switch v := raw.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	default:
		return ""
	}
}

func deepSeekFinalPayloadModel(payload []byte, fallback string) string {
	if got := strings.TrimSpace(gjson.GetBytes(payload, "model").String()); got != "" {
		return got
	}
	return strings.TrimSpace(fallback)
}

func deepSeekPatchRequestReasoning(payload []byte, scope deepSeekReasoningScope, cache *deepSeekReasoningCache) []byte {
	if len(payload) == 0 || cache == nil {
		return payload
	}
	var root map[string]any
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	if err := decoder.Decode(&root); err != nil {
		return payload
	}
	messages, ok := root["messages"].([]any)
	if !ok {
		return payload
	}
	changed := false
	for _, raw := range messages {
		message, okMessage := raw.(map[string]any)
		if !okMessage || strings.TrimSpace(stringFromAny(message["role"])) != "assistant" {
			continue
		}
		if _, exists := message["reasoning_content"]; exists {
			continue
		}
		toolCalls, okTools := message["tool_calls"].([]any)
		if !okTools || len(toolCalls) == 0 {
			continue
		}
		key := deepSeekReasoningKeyForMessage(scope, message)
		reasoning, okReasoning := cache.Lookup(key)
		if !okReasoning {
			continue
		}
		message["reasoning_content"] = reasoning
		changed = true
	}
	if !changed {
		return payload
	}
	out, err := json.Marshal(root)
	if err != nil {
		return payload
	}
	return out
}

func deepSeekCaptureNonStreamReasoning(body []byte, scope deepSeekReasoningScope, cache *deepSeekReasoningCache) {
	if len(body) == 0 || cache == nil {
		return
	}
	var root map[string]any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&root); err != nil {
		return
	}
	choices, ok := root["choices"].([]any)
	if !ok {
		return
	}
	for _, rawChoice := range choices {
		choice, okChoice := rawChoice.(map[string]any)
		if !okChoice {
			continue
		}
		message, okMessage := choice["message"].(map[string]any)
		if !okMessage {
			continue
		}
		if strings.TrimSpace(stringFromAny(message["role"])) != "assistant" {
			continue
		}
		reasoning, okReasoning := message["reasoning_content"].(string)
		if !okReasoning || strings.TrimSpace(reasoning) == "" {
			continue
		}
		if !deepSeekValidToolCalls(message["tool_calls"]) {
			continue
		}
		cache.Store(deepSeekReasoningKeyForMessage(scope, message), reasoning)
	}
}

func deepSeekValidToolCalls(raw any) bool {
	toolCalls, ok := raw.([]any)
	if !ok || len(toolCalls) == 0 {
		return false
	}
	for _, rawTool := range toolCalls {
		tool, okTool := rawTool.(map[string]any)
		if !okTool {
			return false
		}
		if strings.TrimSpace(stringFromAny(tool["id"])) == "" {
			return false
		}
		fn, okFunction := tool["function"].(map[string]any)
		if !okFunction {
			return false
		}
		if strings.TrimSpace(stringFromAny(fn["name"])) == "" {
			return false
		}
		if _, okArguments := fn["arguments"].(string); !okArguments {
			return false
		}
	}
	return true
}

func deepSeekReasoningKeyForMessage(scope deepSeekReasoningScope, message map[string]any) deepSeekReasoningKey {
	toolCallIDs := deepSeekToolCallIDs(message["tool_calls"])
	turnHash := ""
	if toolCallIDs == "" {
		turnHash = deepSeekAssistantTurnHash(message)
	}
	return deepSeekReasoningKey{
		deepSeekReasoningScope: scope,
		ToolCallIDs:            toolCallIDs,
		TurnHash:               turnHash,
	}
}

func deepSeekToolCallIDs(raw any) string {
	toolCalls, ok := raw.([]any)
	if !ok || len(toolCalls) == 0 {
		return ""
	}
	ids := make([]string, 0, len(toolCalls))
	for _, rawTool := range toolCalls {
		tool, okTool := rawTool.(map[string]any)
		if !okTool {
			continue
		}
		id := strings.TrimSpace(stringFromAny(tool["id"]))
		if id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return ""
	}
	sort.Strings(ids)
	return strings.Join(ids, "\x00")
}

func deepSeekAssistantTurnHash(message map[string]any) string {
	if len(message) == 0 {
		return ""
	}
	copyMessage := make(map[string]any, len(message))
	for key, value := range message {
		if key == "reasoning_content" {
			continue
		}
		copyMessage[key] = value
	}
	canonical, err := json.Marshal(copyMessage)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}
