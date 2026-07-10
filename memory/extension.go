package memory

import (
	"context"
	"fmt"
	"strings"

	"time"

	"github.com/xujian519/mady/agentcore"
)

// ExtensionName 是 MemoryExtension 的注册名称。
const ExtensionName = "memory"

// MemoryExtension 集成记忆系统到 Mady Agent。
//
// 借鉴 Mady 现有 extension 模式（如 psychological 扩展）：
//   - 作为 Extension 注册到 Agent
//   - 通过 TransformContextProvider 在每次 LLM 调用前注入记忆上下文
//   - 通过 LifecycleProvider 在 AfterModelCall 时自动提取新记忆
//   - 通过 ToolProvider 暴露 remember/recall/forget 工具（Letta 风格的自我编辑）
type MemoryExtension struct {
	manager *Manager
	scope   MemoryScope
	cfg     ExtensionConfig
}

// ExtensionConfig 控制 MemoryExtension 的行为。
type ExtensionConfig struct {
	// Enabled 是否启用记忆注入。
	Enabled bool `json:"enabled"`

	// AutoExtract 自动从对话中提取记忆（写在 AfterModelCall 中）。
	AutoExtract bool `json:"auto_extract"`

	// InjectMode 控制记忆注入策略。
	InjectMode string `json:"inject_mode"` // "always" | "smart" | "on_demand"

	// MaxMemoryTokens 注入的最大 token 数。
	MaxMemoryTokens int64 `json:"max_memory_tokens"`

	// TopK 每轮检索的最大记忆条数。
	TopK int `json:"top_k"`

	// ExposeTools 是否将 remember/recall/forget 暴露为工具。
	ExposeTools bool `json:"expose_tools"`
}

// DefaultExtensionConfig 返回默认扩展配置。
func DefaultExtensionConfig() ExtensionConfig {
	return ExtensionConfig{
		Enabled:         true,
		AutoExtract:     false, // Phase 1 默认关闭
		InjectMode:      "always",
		MaxMemoryTokens: 2000,
		TopK:            5,
		ExposeTools:     true,
	}
}

// NewExtension 创建一个记忆扩展。
// scope 指定此 Agent 的记忆作用域。
func NewExtension(manager *Manager, scope MemoryScope, cfg ExtensionConfig) *MemoryExtension {
	return &MemoryExtension{
		manager: manager,
		scope:   scope,
		cfg:     cfg,
	}
}

// Ensure 接口实现
var (
	_ agentcore.Extension                = (*MemoryExtension)(nil)
	_ agentcore.TransformContextProvider = (*MemoryExtension)(nil)
	_ agentcore.ToolProvider             = (*MemoryExtension)(nil)
	_ agentcore.LifecycleProvider        = (*MemoryExtension)(nil)
	_ agentcore.LayerProvider            = (*MemoryExtension)(nil)
)

// ---------------------------------------------------------------------------
// Extension 接口
// ---------------------------------------------------------------------------

func (e *MemoryExtension) Name() string { return ExtensionName }

func (e *MemoryExtension) Init(ctx context.Context, agent *agentcore.Agent) error {
	if !e.cfg.Enabled {
		return nil
	}
	return nil
}

func (e *MemoryExtension) Dispose() error {
	return e.manager.Close()
}

// ---------------------------------------------------------------------------
// LayerProvider — ContextBuilder 集成
// ---------------------------------------------------------------------------

func (e *MemoryExtension) Layer() agentcore.ContextLayer { return agentcore.LayerMemory }

func (e *MemoryExtension) Provide(ctx context.Context, input agentcore.BuildInput, _ agentcore.LayerConfig) ([]agentcore.Message, error) {
	if !e.cfg.Enabled || e.manager == nil {
		return nil, nil
	}
	query := lastUserMessage(input.Messages)
	if query == "" {
		return nil, nil
	}

	filter := MemoryFilter{UserID: e.scope.UserID, TopK: e.cfg.TopK}
	results, err := e.manager.Search(ctx, query, filter)
	if err != nil || len(results) == 0 {
		return nil, nil
	}
	memoriesTok := e.cfg.MaxMemoryTokens
	if memoriesTok > 0 {
		results = filterByBudget(results, memoriesTok)
	}
	if len(results) == 0 {
		return nil, nil
	}
	contextBlock := buildMemoryContextBlock(results, e.scope)
	return []agentcore.Message{{Role: agentcore.RoleSystem, Content: contextBlock}}, nil
}

// ---------------------------------------------------------------------------
// TransformContextProvider — 预 LLM 记忆注入
// ---------------------------------------------------------------------------

func (e *MemoryExtension) TransformContext(ctx context.Context, msgs []agentcore.Message) []agentcore.Message {
	if !e.cfg.Enabled || e.manager == nil || len(msgs) == 0 {
		return msgs
	}

	// 找到最后一条用户消息作为查询
	query := lastUserMessage(msgs)
	if query == "" {
		return msgs
	}

	// 检索相关记忆
	filter := MemoryFilter{
		UserID: e.scope.UserID,
		TopK:   e.cfg.TopK,
	}
	results, err := e.manager.Search(ctx, query, filter)
	if err != nil || len(results) == 0 {
		return msgs
	}

	// 在 Token 预算下过滤
	memoriesTok := e.cfg.MaxMemoryTokens
	if memoriesTok > 0 {
		results = filterByBudget(results, memoriesTok)
	}

	if len(results) == 0 {
		return msgs
	}

	// 构建记忆上下文块
	contextBlock := buildMemoryContextBlock(results, e.scope)

	// 注入：在最后一条 system 消息之后、第一条非 system 消息之前
	sysMsg := agentcore.Message{
		Role:    agentcore.RoleSystem,
		Content: contextBlock,
	}
	msgs = injectAfterLastSystem(msgs, sysMsg)

	return msgs
}

// ---------------------------------------------------------------------------
// LifecycleProvider — 自动记忆提取
// ---------------------------------------------------------------------------

func (e *MemoryExtension) LifecycleHook() agentcore.LifecycleHook {
	return &memoryLifecycleHook{ext: e}
}

// memoryLifecycleHook 嵌入 BaseLifecycleHook，只覆写相关方法。
type memoryLifecycleHook struct {
	agentcore.BaseLifecycleHook
	ext *MemoryExtension
}

func (h *memoryLifecycleHook) AfterModelCall(ctx context.Context, arc *agentcore.AgentRunContext, mcc *agentcore.ModelCallContext) {
	if !h.ext.cfg.AutoExtract || h.ext.manager == nil {
		return
	}
	if mcc == nil || mcc.Request == nil || mcc.Response == nil {
		return
	}

	// 找最后一条用户消息和助手响应
	userMsg := lastUserMessage(arc.Messages)
	respContent := ""
	if mcc.Response != nil {
		respContent = mcc.Response.Content
	}
	if userMsg == "" && respContent == "" {
		return
	}

	// 异步提取记忆（不阻塞主流程）
	go func() {
		defer func() { _ = recover() }() // 防止 goroutine panic 导致进程崩溃
		extractCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, _ = h.ext.manager.RememberFromTurn(extractCtx, userMsg, respContent, h.ext.scope)
	}()
}

// ---------------------------------------------------------------------------
// ToolProvider — 记忆工具
// ---------------------------------------------------------------------------

func (e *MemoryExtension) Tools() []*agentcore.Tool {
	if !e.cfg.ExposeTools {
		return nil
	}
	return NewMemoryTools(e.manager, e.scope)
}

// ---------------------------------------------------------------------------
// 内部辅助函数
// ---------------------------------------------------------------------------

// lastUserMessage 从消息列表中提取最后一条用户消息的内容。
func lastUserMessage(msgs []agentcore.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == agentcore.RoleUser {
			return msgs[i].Content
		}
	}
	return ""
}

// injectAfterLastSystem 将消息插入到最后一条 system 消息之后。
func injectAfterLastSystem(msgs []agentcore.Message, inject agentcore.Message) []agentcore.Message {
	insertIdx := 0
	for i, msg := range msgs {
		if msg.Role == agentcore.RoleSystem {
			insertIdx = i + 1
		}
	}
	result := make([]agentcore.Message, 0, len(msgs)+1)
	result = append(result, msgs[:insertIdx]...)
	result = append(result, inject)
	result = append(result, msgs[insertIdx:]...)
	return result
}

// filterByBudget 根据 token 预算过滤记忆结果。
func filterByBudget(results []ScoredMemory, maxTokens int64) []ScoredMemory {
	var filtered []ScoredMemory
	tokensUsed := int64(0)
	for _, sr := range results {
		t := int64(len([]rune(sr.Entry.Content)) / 4)
		if tokensUsed+t > maxTokens {
			break // 已超预算，跳过剩余（已排序）
		}
		tokensUsed += t
		filtered = append(filtered, sr)
	}
	return filtered
}

// buildMemoryContextBlock 构建格式化的记忆上下文块。
func buildMemoryContextBlock(results []ScoredMemory, scope MemoryScope) string {
	var b strings.Builder

	b.WriteString("[记忆上下文 - 参考信息]\n")

	userLabel := scope.UserID
	if userLabel == "" {
		userLabel = "当前用户"
	}
	fmt.Fprintf(&b, "以下是与 %s 相关的历史记录，请参考：\n\n", userLabel)

	for i, sr := range results {
		layer := string(sr.Entry.Layer)
		importance := ""
		if sr.Entry.Importance > 0.7 {
			importance = " [重要]"
		}

		fmt.Fprintf(&b, "--- 记忆 %d (相关度: %.2f%s, 类型: %s) ---\n",
			i+1, sr.Composite, importance, layer)
		b.WriteString(sr.Entry.Content)
		b.WriteString("\n\n")
	}

	b.WriteString("--- 记忆上下文结束 ---\n")
	return b.String()
}
