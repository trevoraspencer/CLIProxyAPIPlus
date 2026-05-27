// Package xiaomi implements thinking configuration for Xiaomi MiMo models.
package xiaomi

import (
	"fmt"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/thinking"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Applier maps canonical thinking config to Xiaomi MiMo thinking.type.
type Applier struct{}

var _ thinking.ProviderApplier = (*Applier)(nil)

// NewApplier creates a new Xiaomi thinking applier.
func NewApplier() *Applier {
	return &Applier{}
}

func init() {
	thinking.RegisterProvider("xiaomi", NewApplier())
}

// Apply emits Xiaomi's official API Console format:
//
//	{"thinking":{"type":"enabled"}}
//	{"thinking":{"type":"disabled"}}
func (a *Applier) Apply(body []byte, config thinking.ThinkingConfig, modelInfo *registry.ModelInfo) ([]byte, error) {
	if !thinking.IsUserDefinedModel(modelInfo) && modelInfo.Thinking == nil {
		return body, nil
	}
	if len(body) == 0 || !gjson.ValidBytes(body) {
		body = []byte(`{}`)
	}

	thinkingType := "enabled"
	if config.Mode == thinking.ModeNone || (config.Mode == thinking.ModeBudget && config.Budget == 0) {
		thinkingType = "disabled"
	}

	result := body
	for _, path := range []string{"reasoning_effort", "reasoning", "output_config"} {
		var errDelete error
		result, errDelete = sjson.DeleteBytes(result, path)
		if errDelete != nil {
			return body, fmt.Errorf("xiaomi thinking: failed to clear %s: %w", path, errDelete)
		}
	}
	result, errSet := sjson.SetBytes(result, "thinking.type", thinkingType)
	if errSet != nil {
		return body, fmt.Errorf("xiaomi thinking: failed to set thinking.type: %w", errSet)
	}
	return result, nil
}
