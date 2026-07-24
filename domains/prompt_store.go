package domains

import (
	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/prompt"
)

// globalPromptStore 是 PromptTemplate Store 的全局实例，由 SetupPromptStore
// 在启动期注入。遵循与 globalTemplateStore 一致的全局注入模式，供
// PatentAgentConfig、LegalAgentConfig 以及工作流节点在构造 Agent 时解析
// prompt://<name> 模板引用。
var globalPromptStore *prompt.PromptStore

// SetupPromptStore 在启动期注入提示词模板仓库实例，并同步注册为
// prompt 包的全局默认 Store，供 disclosure 等底层包无需反向 import
// domains 即可解析模板引用。
// 必须在任何依赖模板解析的 Agent 创建前调用。
func SetupPromptStore(store *prompt.PromptStore) {
	globalPromptStore = store
	prompt.SetDefaultStore(store)
}

// PromptStore 返回已注入的全局提示词模板仓库。未注入时返回 nil。
func PromptStore() *prompt.PromptStore {
	return globalPromptStore
}

// ResolveSystemPrompt interprets a raw system prompt value. If it starts
// with "prompt://<name>", the named template's system_prompt field is
// returned. Otherwise the raw value is returned unchanged. When the template
// is not found or the store is nil, the raw value is returned as a safe
// fallback.
func ResolveSystemPrompt(raw string) string {
	return prompt.ResolveSystemPrompt(raw)
}

// ResolveSystemPromptOr is like ResolveSystemPrompt but returns fallback when
// the referenced template cannot be resolved.
func ResolveSystemPromptOr(raw, fallback string) string {
	return prompt.ResolveSystemPromptOr(raw, fallback)
}

// injectPromptTools 向 Agent 配置注册提示词模板相关工具。
// 当 globalPromptStore 未配置（nil）时静默跳过，不影响现有行为。
func injectPromptTools(cfg *agentcore.Config) {
	if globalPromptStore != nil {
		cfg.Tools = append(cfg.Tools, prompt.NewListPromptsTool(globalPromptStore))
	}
}
