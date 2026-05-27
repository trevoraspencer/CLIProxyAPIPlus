package executor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	"github.com/tidwall/gjson"
)

const (
	xiaomiReasoningCacheDefaultMaxEntries = 1024
	xiaomiReasoningCacheDefaultTTL        = 30 * time.Minute
)

var xiaomiAllowedHosts = map[string]struct{}{
	"api.xiaomimimo.com":            {},
	"token-plan-cn.xiaomimimo.com":  {},
	"token-plan-sgp.xiaomimimo.com": {},
	"token-plan-ams.xiaomimimo.com": {},
}

var defaultXiaomiReasoningCache = newDeepSeekReasoningCache(xiaomiReasoningCacheDefaultMaxEntries, xiaomiReasoningCacheDefaultTTL)

func (e *OpenAICompatExecutor) xiaomiEnabled(auth *cliproxyauth.Auth, baseURL string) bool {
	if xiaomiIdentityString(e.provider) {
		return true
	}
	if auth != nil {
		if xiaomiIdentityString(auth.Provider) {
			return true
		}
		for _, key := range []string{"compat_name", "provider_key", "provider"} {
			if auth.Attributes != nil && xiaomiIdentityString(auth.Attributes[key]) {
				return true
			}
		}
	}
	if compat := e.resolveCompatConfig(auth); compat != nil && xiaomiCompatConfig(compat) {
		return true
	}
	return xiaomiBaseURL(baseURL)
}

func xiaomiCompatConfig(compat *config.OpenAICompatibility) bool {
	if compat == nil {
		return false
	}
	return xiaomiIdentityString(compat.Name) || xiaomiBaseURL(compat.BaseURL)
}

func xiaomiIdentityString(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "xiaomi", "xiaomi-mimo", "xiaomi_mimo", "mimo", "mimo-api", "xiaomi-openai-compatibility":
		return true
	default:
		return false
	}
}

func xiaomiBaseURL(raw string) bool {
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
	_, ok := xiaomiAllowedHosts[host]
	return ok
}

func xiaomiReasoningScopeFor(e *OpenAICompatExecutor, auth *cliproxyauth.Auth, model string, payload []byte, opts cliproxyexecutor.Options) deepSeekReasoningScope {
	scope := deepSeekReasoningScopeFor(e, auth, model, payload, opts)
	scope.Provider = "xiaomi"
	return scope
}

func xiaomiPatchRequestReasoning(payload []byte, scope deepSeekReasoningScope, cache *deepSeekReasoningCache) []byte {
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
		if reasoning, exists := message["reasoning_content"]; exists && strings.TrimSpace(stringFromAny(reasoning)) != "" {
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

func (e *OpenAICompatExecutor) xiaomiWebSearchEnabled(auth *cliproxyauth.Auth) bool {
	if auth != nil && auth.Attributes != nil {
		if raw, ok := auth.Attributes["web_search_enabled"]; ok {
			enabled, errParse := strconv.ParseBool(strings.TrimSpace(raw))
			return errParse == nil && enabled
		}
	}
	if compat := e.resolveCompatConfig(auth); compat != nil {
		return compat.WebSearchEnabled
	}
	return false
}

func normalizeXiaomiChatPayload(payload []byte, original []byte, webSearchEnabled bool) []byte {
	if len(payload) == 0 {
		return payload
	}
	var root map[string]any
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	if err := decoder.Decode(&root); err != nil {
		return payload
	}

	changed := false
	for _, field := range []string{"reasoning_effort", "reasoning", "output_config"} {
		if _, exists := root[field]; exists {
			delete(root, field)
			changed = true
		}
	}

	tools, toolsChanged := normalizeXiaomiTools(root["tools"], original, webSearchEnabled)
	if toolsChanged {
		changed = true
		if len(tools) == 0 {
			delete(root, "tools")
			delete(root, "tool_choice")
		} else {
			root["tools"] = tools
		}
	}
	if currentTools, ok := root["tools"].([]any); ok && len(currentTools) > 0 {
		if choice, exists := root["tool_choice"]; exists && !xiaomiToolChoiceIsAuto(choice) {
			root["tool_choice"] = "auto"
			changed = true
		}
	} else if _, exists := root["tool_choice"]; exists {
		delete(root, "tool_choice")
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

func normalizeXiaomiTools(raw any, original []byte, webSearchEnabled bool) ([]any, bool) {
	rawTools, hasTools := raw.([]any)
	tools := make([]any, 0, len(rawTools)+1)
	if hasTools {
		tools = append(tools, rawTools...)
	}
	if webSearchEnabled {
		tools = append(tools, extractXiaomiWebSearchTools(original)...)
	}
	if len(tools) == 0 {
		return nil, false
	}

	filtered := make([]any, 0, len(tools))
	seenFunction := make(map[string]struct{}, len(tools))
	seenWebSearch := false
	changed := !hasTools
	for _, rawTool := range tools {
		tool, okTool := rawTool.(map[string]any)
		if !okTool {
			changed = true
			continue
		}
		toolType := strings.TrimSpace(stringFromAny(tool["type"]))
		if toolType == "" {
			if _, okFunction := tool["function"]; okFunction {
				toolType = "function"
				tool["type"] = toolType
				changed = true
			}
		}
		switch toolType {
		case "function":
			normalized, okNormalize, didChange := normalizeXiaomiFunctionTool(tool, seenFunction)
			changed = changed || didChange
			if okNormalize {
				filtered = append(filtered, normalized)
			}
		case "web_search":
			if !webSearchEnabled {
				changed = true
				continue
			}
			if seenWebSearch {
				changed = true
				continue
			}
			seenWebSearch = true
			filtered = append(filtered, sanitizeXiaomiWebSearchTool(tool))
			changed = true
		default:
			changed = true
		}
	}
	if len(filtered) != len(tools) {
		changed = true
	}
	return filtered, changed
}

func normalizeXiaomiFunctionTool(tool map[string]any, seen map[string]struct{}) (map[string]any, bool, bool) {
	changed := false
	fn, okFunction := tool["function"].(map[string]any)
	if !okFunction {
		return nil, false, true
	}
	name := strings.TrimSpace(stringFromAny(fn["name"]))
	if name == "" {
		return nil, false, true
	}
	key := strings.ToLower(name)
	if _, exists := seen[key]; exists {
		return nil, false, true
	}
	seen[key] = struct{}{}

	if strict, exists := fn["strict"]; exists {
		if _, okStrict := strict.(bool); !okStrict {
			delete(fn, "strict")
			changed = true
		}
	}
	return tool, true, changed
}

func sanitizeXiaomiWebSearchTool(tool map[string]any) map[string]any {
	out := map[string]any{"type": "web_search"}
	for _, key := range []string{"max_keyword", "force_search", "limit", "user_location"} {
		if value, exists := tool[key]; exists {
			out[key] = value
		}
	}
	return out
}

func extractXiaomiWebSearchTools(original []byte) []any {
	if len(original) == 0 || !gjson.ValidBytes(original) {
		return nil
	}
	var out []any
	gjson.GetBytes(original, "tools").ForEach(func(_, tool gjson.Result) bool {
		if strings.TrimSpace(tool.Get("type").String()) != "web_search" {
			return true
		}
		entry := map[string]any{"type": "web_search"}
		for _, key := range []string{"max_keyword", "force_search", "limit", "user_location"} {
			if value := tool.Get(key); value.Exists() {
				entry[key] = xiaomiGJSONValue(value)
			}
		}
		out = append(out, entry)
		return true
	})
	return out
}

func xiaomiGJSONValue(value gjson.Result) any {
	switch value.Type {
	case gjson.True:
		return true
	case gjson.False:
		return false
	case gjson.Number:
		return json.Number(value.Raw)
	case gjson.JSON:
		var decoded any
		decoder := json.NewDecoder(strings.NewReader(value.Raw))
		decoder.UseNumber()
		if err := decoder.Decode(&decoded); err == nil {
			return decoded
		}
		return value.Raw
	default:
		return value.String()
	}
}

func xiaomiToolChoiceIsAuto(choice any) bool {
	if strings.EqualFold(strings.TrimSpace(stringFromAny(choice)), "auto") {
		return true
	}
	choiceMap, ok := choice.(map[string]any)
	if !ok {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(stringFromAny(choiceMap["type"])), "auto")
}

func validateXiaomiMultimodalPayload(payload []byte) error {
	if len(payload) == 0 {
		return nil
	}
	var root map[string]any
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	if err := decoder.Decode(&root); err != nil {
		return nil
	}
	model := strings.TrimSpace(stringFromAny(root["model"]))
	if model == "" || !xiaomiPayloadContainsImage(root["messages"]) || xiaomiModelSupportsImageInput(model) {
		return nil
	}
	return statusErr{
		code: http.StatusBadRequest,
		msg:  fmt.Sprintf("xiaomi mimo model %q does not support image input; use mimo-v2.5 or an omni model for multimodal chat", model),
	}
}

func xiaomiModelSupportsImageInput(model string) bool {
	normalized := strings.ToLower(strings.TrimSpace(model))
	if normalized == "" {
		return false
	}
	if strings.Contains(normalized, "omni") {
		return true
	}
	if strings.Contains(normalized, "tts") {
		return false
	}
	return strings.HasPrefix(normalized, "mimo-v2.5") && !strings.Contains(normalized, "pro")
}

func xiaomiPayloadContainsImage(raw any) bool {
	switch value := raw.(type) {
	case []any:
		for _, item := range value {
			if xiaomiPayloadContainsImage(item) {
				return true
			}
		}
	case map[string]any:
		if toolType := strings.ToLower(strings.TrimSpace(stringFromAny(value["type"]))); toolType == "image_url" || toolType == "input_image" || toolType == "image" {
			return true
		}
		if _, exists := value["image_url"]; exists {
			return true
		}
		for key, nested := range value {
			switch key {
			case "content", "messages", "parts":
				if xiaomiPayloadContainsImage(nested) {
					return true
				}
			}
		}
	}
	return false
}

func replaceDefaultXiaomiCacheForTest() func() {
	previous := defaultXiaomiReasoningCache
	defaultXiaomiReasoningCache = newDeepSeekReasoningCache(xiaomiReasoningCacheDefaultMaxEntries, xiaomiReasoningCacheDefaultTTL)
	return func() {
		defaultXiaomiReasoningCache = previous
	}
}
