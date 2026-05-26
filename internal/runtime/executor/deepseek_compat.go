package executor

import (
	"bytes"
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
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	deepSeekProviderName              = "deepseek"
	defaultDeepSeekReasoningCacheTTL  = 30 * time.Minute
	defaultDeepSeekReasoningCacheSize = 512
)

var deepSeekAllowedHosts = map[string]struct{}{
	"api.deepseek.com": {},
}

var defaultDeepSeekReasoningCache = newDeepSeekReasoningCache(defaultDeepSeekReasoningCacheTTL, defaultDeepSeekReasoningCacheSize)

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

type deepSeekStreamCapture struct {
	cache    *deepSeekReasoningCache
	identity deepSeekCompatIdentity
	choices  map[int]*deepSeekStreamChoiceCapture
	aborted  bool
}

type deepSeekStreamChoiceCapture struct {
	reasoning strings.Builder
	content   strings.Builder
	tools     map[int]*deepSeekStreamToolCapture
}

type deepSeekStreamToolCapture struct {
	id        strings.Builder
	name      strings.Builder
	arguments strings.Builder
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
		AssistantTurnHash: deepSeekAssistantTurnHash(raw),
	}
	if len(turn.ToolCallIDs) == 0 && turn.AssistantTurnHash == "" {
		return deepSeekReasoningTurn{}, false
	}
	return turn, true
}

func deepSeekRequestTurnFromAssistantMessage(raw []byte) (deepSeekReasoningTurn, bool) {
	var msg struct {
		Role      string `json:"role"`
		ToolCalls []struct {
			ID string `json:"id"`
		} `json:"tool_calls"`
	}
	if err := json.Unmarshal(raw, &msg); err != nil {
		return deepSeekReasoningTurn{}, false
	}
	if strings.TrimSpace(msg.Role) != "assistant" || len(msg.ToolCalls) == 0 {
		return deepSeekReasoningTurn{}, false
	}
	ids := make([]string, 0, len(msg.ToolCalls))
	for _, toolCall := range msg.ToolCalls {
		if id := strings.TrimSpace(toolCall.ID); id != "" {
			ids = append(ids, id)
		}
	}
	return deepSeekReasoningTurn{
		ToolCallIDs:       ids,
		AssistantTurnHash: deepSeekAssistantTurnHash(raw),
	}, true
}

func deepSeekPatchRequestPayload(cache *deepSeekReasoningCache, identity deepSeekCompatIdentity, payload []byte) []byte {
	if cache == nil || !identity.Enabled || len(payload) == 0 || !json.Valid(payload) {
		return payload
	}
	messages := gjson.GetBytes(payload, "messages")
	if !messages.IsArray() {
		return payload
	}
	patched := payload
	messages.ForEach(func(key, value gjson.Result) bool {
		if strings.TrimSpace(gjson.Get(value.Raw, "role").String()) != "assistant" {
			return true
		}
		if gjson.Get(value.Raw, "reasoning_content").Exists() {
			return true
		}
		toolCalls := gjson.Get(value.Raw, "tool_calls")
		if !toolCalls.IsArray() || len(toolCalls.Array()) == 0 {
			return true
		}
		turn, ok := deepSeekRequestTurnFromAssistantMessage([]byte(value.Raw))
		if !ok {
			return true
		}
		reasoning, ok := cache.Get(identity, turn)
		if !ok {
			return true
		}
		updated, err := sjson.SetBytes(patched, "messages."+key.String()+".reasoning_content", reasoning)
		if err == nil {
			patched = updated
		}
		return true
	})
	return patched
}

func deepSeekCaptureNonStreamResponse(cache *deepSeekReasoningCache, identity deepSeekCompatIdentity, responseBody []byte) {
	if cache == nil || !identity.Enabled || len(responseBody) == 0 || !json.Valid(responseBody) {
		return
	}
	choices := gjson.GetBytes(responseBody, "choices")
	if !choices.IsArray() {
		return
	}
	choices.ForEach(func(_, choice gjson.Result) bool {
		message := gjson.Get(choice.Raw, "message")
		if !message.Exists() || !message.IsObject() {
			return true
		}
		if turn, ok := deepSeekReasoningTurnFromAssistantMessage([]byte(message.Raw)); ok {
			cache.Put(identity, turn)
		}
		return true
	})
}

func newDeepSeekStreamCapture(cache *deepSeekReasoningCache, identity deepSeekCompatIdentity) *deepSeekStreamCapture {
	if cache == nil || !identity.Enabled {
		return nil
	}
	return &deepSeekStreamCapture{
		cache:    cache,
		identity: identity,
		choices:  make(map[int]*deepSeekStreamChoiceCapture),
	}
}

func (c *deepSeekStreamCapture) ObserveSSELine(line []byte) {
	if c == nil || c.aborted {
		return
	}
	trimmed := bytes.TrimSpace(line)
	if len(trimmed) == 0 || !bytes.HasPrefix(trimmed, []byte("data:")) {
		return
	}
	payload := strings.TrimSpace(string(trimmed[len("data:"):]))
	if payload == "[DONE]" {
		c.Commit()
		return
	}
	if !json.Valid([]byte(payload)) {
		c.aborted = true
		return
	}
	choices := gjson.Get(payload, "choices")
	if !choices.IsArray() {
		return
	}
	choices.ForEach(func(_, choice gjson.Result) bool {
		if c.aborted {
			return false
		}
		indexResult := gjson.Get(choice.Raw, "index")
		if !indexResult.Exists() {
			return true
		}
		choiceIndex := int(indexResult.Int())
		state := c.choice(choiceIndex)
		reasoning := gjson.Get(choice.Raw, "delta.reasoning_content")
		if reasoning.Exists() && reasoning.Type != gjson.String {
			c.aborted = true
			return false
		}
		if reasoning.Exists() {
			state.reasoning.WriteString(reasoning.String())
		}
		content := gjson.Get(choice.Raw, "delta.content")
		if content.Exists() && content.Type == gjson.String {
			state.content.WriteString(content.String())
		}
		toolCalls := gjson.Get(choice.Raw, "delta.tool_calls")
		if toolCalls.IsArray() {
			toolCalls.ForEach(func(_, toolCall gjson.Result) bool {
				if c.aborted {
					return false
				}
				toolIndexResult := gjson.Get(toolCall.Raw, "index")
				if !toolIndexResult.Exists() {
					return true
				}
				tool := state.tool(int(toolIndexResult.Int()))
				if id := gjson.Get(toolCall.Raw, "id"); id.Exists() && id.Type == gjson.String {
					tool.id.WriteString(id.String())
				}
				if name := gjson.Get(toolCall.Raw, "function.name"); name.Exists() && name.Type == gjson.String {
					tool.name.WriteString(name.String())
				}
				if args := gjson.Get(toolCall.Raw, "function.arguments"); args.Exists() && args.Type == gjson.String {
					tool.arguments.WriteString(args.String())
				}
				return true
			})
		}
		return true
	})
}

func (c *deepSeekStreamCapture) Abort() {
	if c != nil {
		c.aborted = true
	}
}

func (c *deepSeekStreamCapture) Commit() {
	if c == nil || c.aborted {
		return
	}
	for _, choice := range c.choices {
		turn, ok := choice.turn()
		if !ok {
			continue
		}
		c.cache.Put(c.identity, turn)
	}
}

func (c *deepSeekStreamCapture) choice(index int) *deepSeekStreamChoiceCapture {
	state := c.choices[index]
	if state == nil {
		state = &deepSeekStreamChoiceCapture{tools: make(map[int]*deepSeekStreamToolCapture)}
		c.choices[index] = state
	}
	return state
}

func (c *deepSeekStreamChoiceCapture) tool(index int) *deepSeekStreamToolCapture {
	tool := c.tools[index]
	if tool == nil {
		tool = &deepSeekStreamToolCapture{}
		c.tools[index] = tool
	}
	return tool
}

func (c *deepSeekStreamChoiceCapture) turn() (deepSeekReasoningTurn, bool) {
	if c == nil || strings.TrimSpace(c.reasoning.String()) == "" || len(c.tools) == 0 {
		return deepSeekReasoningTurn{}, false
	}
	indexes := make([]int, 0, len(c.tools))
	for index := range c.tools {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	ids := make([]string, 0, len(indexes))
	toolCalls := make([]map[string]any, 0, len(indexes))
	for _, index := range indexes {
		tool := c.tools[index]
		id := strings.TrimSpace(tool.id.String())
		name := strings.TrimSpace(tool.name.String())
		if id == "" || name == "" {
			return deepSeekReasoningTurn{}, false
		}
		ids = append(ids, id)
		toolCalls = append(toolCalls, map[string]any{
			"id":   id,
			"type": "function",
			"function": map[string]any{
				"name":      name,
				"arguments": tool.arguments.String(),
			},
		})
	}
	msg := map[string]any{
		"role":              "assistant",
		"content":           c.content.String(),
		"reasoning_content": c.reasoning.String(),
		"tool_calls":        toolCalls,
	}
	raw, err := json.Marshal(msg)
	if err != nil {
		return deepSeekReasoningTurn{}, false
	}
	return deepSeekReasoningTurn{
		Reasoning:         c.reasoning.String(),
		ToolCallIDs:       ids,
		AssistantTurnHash: deepSeekAssistantTurnHash(raw),
	}, true
}

func deepSeekAssistantTurnHash(raw []byte) string {
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return ""
	}
	delete(obj, "reasoning_content")
	normalized, err := json.Marshal(obj)
	if err != nil {
		return ""
	}
	return hashDeepSeekParts(string(normalized))
}
