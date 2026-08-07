package worker

import (
	"context"
	"log/slog"
	"os"

	"github.com/xujian519/mady/agentcore"
)

// WorkerExecutorFactory creates an Executor from a Worker Definition.
type WorkerExecutorFactory func(def *Definition) (*Executor, error)

// NewLLMWorkerFactory returns a WorkerExecutorFactory that creates LLM executors.
// llmFn 接收 (ctx, prompt) 返回 LLM 分析结果。
func NewLLMWorkerFactory(llmFn func(ctx context.Context, prompt string) (string, error)) WorkerExecutorFactory {
	return func(def *Definition) (*Executor, error) {
		return NewLLMExecutor(def, llmFn), nil
	}
}

// RegisterWorkersAsToolsExtensionWithEnv 同 RegisterWorkersAsToolsExtension，
// 但仅在环境变量 MADY_WORKER_ENABLED=1 时生效。
func RegisterWorkersAsToolsExtensionWithEnv(catalog *Catalog, factory WorkerExecutorFactory, tiers ...WorkerTier) agentcore.Extension {
	return &workerToolExtension{
		catalog: catalog,
		factory: factory,
		tiers:   tiers,
		envGate: "MADY_WORKER_ENABLED",
	}
}

// workerToolExtension 将 Worker 定义注册为 Agent 工具的 Extension。
type workerToolExtension struct {
	catalog *Catalog
	factory WorkerExecutorFactory
	tiers   []WorkerTier
	envGate string // 非空时检查环境变量
}

func (e *workerToolExtension) Name() string {
	return "worker-tools"
}

func (e *workerToolExtension) Init(_ context.Context, agent *agentcore.Agent) error {
	// 环境变量门控
	if e.envGate != "" && os.Getenv(e.envGate) != "1" {
		return nil // 静默跳过
	}

	all := e.catalog.List()
	if len(all) == 0 {
		return nil
	}

	// 构建 tier 查找集合
	tierSet := make(map[WorkerTier]bool, len(e.tiers))
	for _, t := range e.tiers {
		tierSet[t] = true
	}

	var tools []*agentcore.Tool
	for _, def := range all {
		// Tier 过滤
		if len(tierSet) > 0 && !tierSet[def.Tier] {
			continue
		}

		exec, err := e.factory(&def)
		if err != nil {
			slog.Warn("worker: factory failed, skipping", "name", def.Name, "err", err)
			continue // 单个 Worker 失败不阻塞整体注册
		}
		tool := AsTool(exec)
		tools = append(tools, tool)
	}

	if len(tools) > 0 {
		agent.RegisterTools(tools...)
	}
	return nil
}

func (e *workerToolExtension) Dispose() error {
	return nil
}

// =============================================================================
// 便利函数
// =============================================================================

// NewCatalogFromDefault 创建一个包含 DefaultWorkers 的 Catalog。
func NewCatalogFromDefault() *Catalog {
	c := NewCatalog()
	for _, d := range DefaultWorkers() {
		_ = c.Register(d)
	}
	return c
}

// RegisterDefaultLLMWorkers 将所有默认 Worker 注册为 LLM 驱动的工具。
// 由环境变量 MADY_WORKER_ENABLED 门控。
// llmFn 接收 (ctx, prompt) 返回 LLM 分析结果。
func RegisterDefaultLLMWorkers(llmFn func(ctx context.Context, prompt string) (string, error)) agentcore.Extension {
	catalog := NewCatalogFromDefault()
	factory := NewLLMWorkerFactory(llmFn)
	return RegisterWorkersAsToolsExtensionWithEnv(catalog, factory)
}

// =============================================================================
// 保留的旧 API — 用于 worker.Registry（非 Extension 路径）
// =============================================================================

// RegisterDefaultWorkers registers all default Workers into the given Registry.
// Pre-registered Workers are registered immediately; lazy Workers are skipped
// and can be activated later via LazyActivate.
func RegisterDefaultWorkers(registry *Registry, catalog *Catalog) []string {
	var skipped []string
	for _, d := range catalog.List() {
		if d.IsPreRegistered() {
			_ = registry.Register(d)
		} else {
			skipped = append(skipped, d.Name)
		}
	}
	return skipped
}

// EnsureWorker is a convenience function that activates a Worker by name if it
// exists in the catalog but is not yet registered. Returns true if newly activated.
func EnsureWorker(registry *Registry, catalog *Catalog, name string) bool {
	d := catalog.Get(name)
	if d == nil {
		return false
	}
	if registry.Get(name) != nil {
		return false
	}
	return registry.LazyActivate(*d)
}

// Ensure compile-time interface compliance.
var _ agentcore.Extension = (*workerToolExtension)(nil)
