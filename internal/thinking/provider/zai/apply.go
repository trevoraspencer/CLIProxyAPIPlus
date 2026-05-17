// Package zai implements thinking configuration for Z.AI GLM models.
package zai

import (
	"fmt"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/thinking"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Applier implements thinking.ProviderApplier for Z.AI.
type Applier struct{}

var _ thinking.ProviderApplier = (*Applier)(nil)

// NewApplier creates a new Z.AI thinking applier.
func NewApplier() *Applier {
	return &Applier{}
}

func init() {
	thinking.RegisterProvider("zai", NewApplier())
}

// Apply maps canonical thinking config to Z.AI thinking.type.
func (a *Applier) Apply(body []byte, config thinking.ThinkingConfig, modelInfo *registry.ModelInfo) ([]byte, error) {
	if !thinking.IsUserDefinedModel(modelInfo) && modelInfo.Thinking == nil {
		return body, nil
	}
	if len(body) == 0 || !gjson.ValidBytes(body) {
		body = []byte(`{}`)
	}

	thinkingType := "enabled"
	if config.Mode == thinking.ModeNone {
		thinkingType = "disabled"
	}

	result, errDeleteEffort := sjson.DeleteBytes(body, "reasoning_effort")
	if errDeleteEffort != nil {
		return body, fmt.Errorf("zai thinking: failed to clear reasoning_effort: %w", errDeleteEffort)
	}
	result, errSetType := sjson.SetBytes(result, "thinking.type", thinkingType)
	if errSetType != nil {
		return body, fmt.Errorf("zai thinking: failed to set thinking.type: %w", errSetType)
	}
	return result, nil
}
