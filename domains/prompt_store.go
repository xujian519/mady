package domains

import (
	"strings"

	"github.com/xujian519/mady/prompt"
)

const promptTemplatePrefix = "prompt://"

// globalPromptStore 是 PromptTemplate Store 的全局实例，由 SetupPromptStore
// 在启动期注入。遵循与 globalTemplateStore 一致的全局注入模式，供
// PatentAgentConfig、LegalAgentConfig 以及工作流节点在构造 Agent 时解析
// prompt://<name> 模板引用。
var globalPromptStore *prompt.PromptStore

// SetupPromptStore 在启动期注入提示词模板仓库实例。
// 必须在任何依赖模板解析的 Agent 创建前调用。
func SetupPromptStore(store *prompt.PromptStore) {
	globalPromptStore = store
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
	if globalPromptStore == nil || !strings.HasPrefix(raw, promptTemplatePrefix) {
		return raw
	}

	name := strings.TrimSpace(strings.TrimPrefix(raw, promptTemplatePrefix))
	if name == "" {
		return raw
	}

	resolved, ok := globalPromptStore.Resolve(name, nil)
	if !ok {
		return raw
	}
	return resolved.SystemPrompt
}
