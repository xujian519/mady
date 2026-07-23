package agentcore

import (
	"fmt"

	"github.com/xujian519/mady/pluginsys"
)

// =============================================================================
// 类型别名 — 向后兼容
// =============================================================================
//
// PluginManifest, PluginPipeline, PluginStage 已迁移到 pluginsys 包。
// 以下类型别名保证 agentcore 外部代码无需改动即可继续使用原来的类型名。

// PluginManifest describes a composable patent/legal workflow plugin.
type PluginManifest = pluginsys.PluginManifest

// PluginPipeline describes a sequence of workflow stages.
type PluginPipeline = pluginsys.PluginPipeline

// PluginStage is a single step in a plugin workflow pipeline.
type PluginStage = pluginsys.PluginStage

// =============================================================================
// 验证
// =============================================================================

// ValidatePlugin validates a PluginManifest's fields, including
// agentcore-specific checks (atom lookup, guardrail level registry).
func ValidatePlugin(p PluginManifest) error {
	err := pluginsys.ValidatePlugin(p, defaultValidateOptions())
	if err != nil {
		return fmt.Errorf("%w", err)
	}
	return nil
}

// LoadPlugin reads a plugin.json file and returns the parsed manifest
// with full agentcore validation.
func LoadPlugin(path string) (*PluginManifest, error) {
	return pluginsys.LoadPlugin(path, defaultValidateOptions())
}

// ScanPlugins discovers all plugin.json files under the given root
// directories. Plugins with the same name keep the first one found.
func ScanPlugins(roots ...string) ([]PluginManifest, error) {
	return pluginsys.ScanPlugins(roots, defaultValidateOptions())
}

// defaultValidateOptions returns a ValidateOptions with atom and guardrail
// lookups wired to the agentcore registries. Shared by ValidatePlugin,
// LoadPlugin, and ScanPlugins to avoid closure duplication.
func defaultValidateOptions() *pluginsys.ValidateOptions {
	return &pluginsys.ValidateOptions{
		AtomLookupFn: func(name string) bool {
			return LookupAtom(name) != nil
		},
		IsValidGuardrailLevel: func(level string) bool {
			validGuardrailLevelsMu.RLock()
			defer validGuardrailLevelsMu.RUnlock()
			return validGuardrailLevels[level]
		},
	}
}
