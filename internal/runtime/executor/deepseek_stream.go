package executor

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"
)

type deepSeekStreamCapture struct {
	scope     deepSeekReasoningScope
	cache     *deepSeekReasoningCache
	choices   map[int]*deepSeekStreamChoice
	failed    bool
	committed bool
}

type deepSeekStreamChoice struct {
	reasoning strings.Builder
	content   strings.Builder
	tools     map[int]*deepSeekStreamToolCall
	failed    bool
}

type deepSeekStreamToolCall struct {
	id        strings.Builder
	typ       strings.Builder
	name      strings.Builder
	arguments strings.Builder
}

func newDeepSeekStreamCapture(scope deepSeekReasoningScope, cache *deepSeekReasoningCache) *deepSeekStreamCapture {
	if cache == nil {
		return nil
	}
	return &deepSeekStreamCapture{scope: scope, cache: cache, choices: make(map[int]*deepSeekStreamChoice)}
}

func (c *deepSeekStreamCapture) ObserveLine(line []byte) {
	if c == nil || c.failed || c.committed {
		return
	}
	trimmed := bytes.TrimSpace(line)
	if !bytes.HasPrefix(trimmed, []byte("data:")) {
		return
	}
	data := bytes.TrimSpace(bytes.TrimPrefix(trimmed, []byte("data:")))
	if len(data) == 0 {
		return
	}
	if bytes.Equal(data, []byte("[DONE]")) {
		c.Commit()
		return
	}
	var root map[string]any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&root); err != nil {
		c.failed = true
		return
	}
	choices, ok := root["choices"].([]any)
	if !ok || len(choices) == 0 {
		return
	}
	for _, rawChoice := range choices {
		choice, okChoice := rawChoice.(map[string]any)
		if !okChoice {
			c.failed = true
			return
		}
		choiceIndex, okIndex := deepSeekNumberIndex(choice["index"])
		if !okIndex || choiceIndex < 0 {
			c.failed = true
			return
		}
		delta, okDelta := choice["delta"].(map[string]any)
		if !okDelta {
			continue
		}
		state := c.choice(choiceIndex)
		if rawReasoning, exists := delta["reasoning_content"]; exists {
			reasoning, okReasoning := rawReasoning.(string)
			if !okReasoning {
				state.failed = true
			} else {
				state.reasoning.WriteString(reasoning)
			}
		}
		if content, okContent := delta["content"].(string); okContent {
			state.content.WriteString(content)
		}
		rawTools, okTools := delta["tool_calls"].([]any)
		if !okTools {
			continue
		}
		for _, rawTool := range rawTools {
			tool, okTool := rawTool.(map[string]any)
			if !okTool {
				state.failed = true
				continue
			}
			toolIndex, okToolIndex := deepSeekNumberIndex(tool["index"])
			if !okToolIndex || toolIndex < 0 {
				state.failed = true
				continue
			}
			toolState := state.tool(toolIndex)
			if id, okID := tool["id"].(string); okID {
				toolState.id.WriteString(id)
			}
			if typ, okType := tool["type"].(string); okType {
				toolState.typ.WriteString(typ)
			}
			if fn, okFunction := tool["function"].(map[string]any); okFunction {
				if name, okName := fn["name"].(string); okName {
					toolState.name.WriteString(name)
				}
				if arguments, okArguments := fn["arguments"].(string); okArguments {
					toolState.arguments.WriteString(arguments)
				}
			}
		}
	}
}

func (c *deepSeekStreamCapture) Commit() {
	if c == nil || c.failed || c.committed {
		return
	}
	c.committed = true
	for _, choice := range c.choices {
		if choice == nil || choice.failed || strings.TrimSpace(choice.reasoning.String()) == "" || len(choice.tools) == 0 {
			continue
		}
		indexes := make([]int, 0, len(choice.tools))
		for index := range choice.tools {
			indexes = append(indexes, index)
		}
		sort.Ints(indexes)
		toolCalls := make([]any, 0, len(indexes))
		for _, index := range indexes {
			tool := choice.tools[index]
			if tool == nil || strings.TrimSpace(tool.id.String()) == "" {
				toolCalls = nil
				break
			}
			toolType := tool.typ.String()
			if toolType == "" {
				toolType = "function"
			}
			toolCalls = append(toolCalls, map[string]any{
				"id":   tool.id.String(),
				"type": toolType,
				"function": map[string]any{
					"name":      tool.name.String(),
					"arguments": tool.arguments.String(),
				},
			})
		}
		if len(toolCalls) == 0 {
			continue
		}
		c.cache.Store(deepSeekReasoningKeyForMessage(c.scope, map[string]any{
			"role":       "assistant",
			"content":    choice.content.String(),
			"tool_calls": toolCalls,
		}), choice.reasoning.String())
	}
}

func (c *deepSeekStreamCapture) choice(index int) *deepSeekStreamChoice {
	if choice := c.choices[index]; choice != nil {
		return choice
	}
	choice := &deepSeekStreamChoice{tools: make(map[int]*deepSeekStreamToolCall)}
	c.choices[index] = choice
	return choice
}

func (c *deepSeekStreamChoice) tool(index int) *deepSeekStreamToolCall {
	if tool := c.tools[index]; tool != nil {
		return tool
	}
	tool := &deepSeekStreamToolCall{}
	c.tools[index] = tool
	return tool
}

func deepSeekNumberIndex(raw any) (int, bool) {
	switch v := raw.(type) {
	case json.Number:
		i, err := v.Int64()
		if err != nil {
			return 0, false
		}
		return int(i), true
	case float64:
		i := int(v)
		return i, float64(i) == v
	case int:
		return v, true
	default:
		return 0, false
	}
}
