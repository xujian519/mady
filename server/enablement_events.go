package server

import (
	"context"
	"log/slog"
	"time"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/agentcore/iface"
	"github.com/xujian519/mady/disclosure"
	"github.com/xujian519/mady/domains/enablement"
	"github.com/xujian519/mady/graph"
)

// enablementExecTimeout 是单个 26.3 评估子图的最大执行时间。
// 作为 EventBus 回调的一部分运行——如果 LLM API 挂起，此超时可防止永久阻塞分发 goroutine。
const enablementExecTimeout = 10 * time.Minute

// =============================================================================
// EnablementTrigger — 26.3 充分公开评估事件触发器
// =============================================================================

// EnablementTrigger 通过 EventBus 监听 disclosure 完成事件，
// 自动触发专利法第26条第3款（充分公开/可实现性）评估的独立 Pregel 子图。
//
// 设计原则（对标 InventivenessTrigger）：
//   - 完全独立：不修改 disclosure 管线代码，只订阅事件
//   - 异步执行：不阻塞 disclosure 管线的主流程
//   - 容错运行：子图失败仅记录日志，不影响上游
type EnablementTrigger struct {
	provider      agentcore.Provider
	bus           iface.EventBus
	logger        *slog.Logger
	cancel        func() // 取消订阅
	ctx           context.Context
	cancelCtx     context.CancelFunc
	resultHandler func(taskID string, result *enablement.EnablementResult) // 可选回调，用于存储结果
}

// EnablementTriggerOption 配置 EnablementTrigger。
type EnablementTriggerOption func(*EnablementTrigger)

// WithEnablementResultHandler 设置结果回调，在 26.3 评估完成后调用。
// 典型用法：注入 s.SetEnablementResult，使结果可通过 API 查询。
func WithEnablementResultHandler(fn func(taskID string, result *enablement.EnablementResult)) EnablementTriggerOption {
	return func(t *EnablementTrigger) { t.resultHandler = fn }
}

// NewEnablementTrigger 创建 26.3 充分公开评估触发器。
// provider 用于运行评估子图的 LLM 调用。
// bus 是事件总线引用。
func NewEnablementTrigger(provider agentcore.Provider, bus iface.EventBus, opts ...EnablementTriggerOption) *EnablementTrigger {
	ctx, cancel := context.WithCancel(context.Background())
	t := &EnablementTrigger{
		provider:  provider,
		bus:       bus,
		logger:    slog.Default().With("component", "enablement_trigger"),
		ctx:       ctx,
		cancelCtx: cancel,
	}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// Start 开始监听 disclosure 完成事件。
// 每次 disclosure 分析完成后自动触发 26.3 充分公开评估。
func (t *EnablementTrigger) Start() {
	t.cancel = t.bus.OnAll(t.onEvent)
}

// Stop 停止监听并取消正在进行的分析，释放订阅资源。
func (t *EnablementTrigger) Stop() {
	if t.cancel != nil {
		t.cancel()
	}
	t.cancelCtx()
}

// onEvent 是 EventBus 回调：筛选 disclosure_completed 事件并执行 26.3 评估。
func (t *EnablementTrigger) onEvent(ev iface.Event) {
	if ev.Type() != iface.EventType(EventDisclosureCompleted) {
		return
	}
	completed, ok := ev.Payload().(DisclosureCompletedEvent)
	if !ok {
		return
	}
	// 报告缺失或执行失败时跳过 26.3 评估。
	if completed.Report == nil || completed.Err != "" {
		return
	}
	report := completed.Report
	if report.Extraction == nil {
		t.logger.Debug("enablement: 跳过（报告无提取数据）", "task_id", completed.TaskID)
		return
	}

	// 检查触发器的 context 是否已被取消（Stop 被调用）。
	if err := t.ctx.Err(); err != nil {
		t.logger.Debug("enablement: 跳过（触发器已停止）", "task_id", completed.TaskID)
		return
	}

	t.logger.Info("enablement: 开始 26.3 充分公开评估",
		"task_id", completed.TaskID,
	)

	input := buildEnablementInput(report, completed.EvidenceCoverage)
	result, err := t.runGraph(t.ctx, input)
	if err != nil {
		t.logger.Error("enablement: 子图执行失败",
			"task_id", completed.TaskID,
			"error", err,
		)
		// Even on error, store partial result so the API can surface
		// whatever structured analysis was completed before the failure.
		if result != nil && t.resultHandler != nil {
			t.resultHandler(completed.TaskID, result)
		}
		return
	}

	t.logger.Info("enablement: 26.3 充分公开评估完成",
		"task_id", completed.TaskID,
		"assessed", result.Assessed,
		"skipped", result.Skipped,
		"is_sufficient", result.IsSufficient,
		"confidence", result.Confidence,
	)

	// 通过回调将结果写回 Server 的 enablementResults map。
	if t.resultHandler != nil {
		t.resultHandler(completed.TaskID, result)
	}
}

// runGraph 构建并执行 26.3 充分公开评估 Pregel 子图。
// 使用 context.WithTimeout 包装传入的 ctx，防止 LLM API 挂起时永久阻塞。
func (t *EnablementTrigger) runGraph(ctx context.Context, input *enablement.EnablementInput) (*enablement.EnablementResult, error) {
	compiled, err := enablement.BuildEnablementGraph(t.provider)
	if err != nil {
		return nil, err
	}

	state := graph.PregelState{}
	state["enablement_input"] = input

	timeoutCtx, cancel := context.WithTimeout(ctx, enablementExecTimeout)
	defer cancel()
	state, runErr := compiled.Run(timeoutCtx, state)

	// 从 state 中提取结果。
	if raw, ok := state["enablement_result"]; ok {
		if result, ok := raw.(*enablement.EnablementResult); ok {
			return result, runErr
		}
	}

	if runErr != nil {
		return nil, runErr
	}
	return &enablement.EnablementResult{
		Assessed:   false,
		Skipped:    true,
		SkipReason: "子图执行后未找到结果（state key 缺失或类型异常）",
	}, nil
}

// buildEnablementInput 从 disclosure 分析报告构建 26.3 评估子图的输入。
// evidenceCoverage 来自 DisclosureCompletedEvent，反映 disclosure 管线中
// retrieve_prior_art 节点的实际证据覆盖状态（"full"/"partial"/"none"）。
func buildEnablementInput(report *disclosure.AnalysisReport, evidenceCoverage string) *enablement.EnablementInput {
	cover := evidenceCoverage
	if cover == "" {
		cover = "partial"
	}
	input := &enablement.EnablementInput{
		EvidenceCoverage: cover,
	}

	if report.Extraction == nil {
		return input
	}

	ext := report.Extraction

	// 1. 转换技术特征。
	for _, f := range ext.Features {
		input.Features = append(input.Features, enablement.TechFeature{
			ID:          f.ID,
			Description: f.Description,
			Category:    string(f.Category),
			Function:    f.Function,
			Importance:  f.Importance,
		})
	}

	// 2. 转换 PFE 三元组（问题-特征-效果因果链）。
	for _, t := range ext.PFETriples {
		input.PFETriples = append(input.PFETriples, enablement.PFETriple{
			ID:         t.ID,
			Problem:    t.Problem,
			FeatureIDs: t.FeatureIDs,
			Effect:     t.Effect,
		})
	}

	// 3. 技术问题和效果列表。
	input.Problems = ext.Problems
	input.Effects = ext.Effects

	// 4. 说明书章节内容（从 DisclosureDoc 获取）。
	if report.Document != nil {
		input.HasDrawings = report.Document.HasDrawings
		input.DocSections = make(map[string]string)
		for section, content := range report.Document.Sections {
			input.DocSections[string(section)] = content
		}
	}

	// 5. 特征非空时提升证据覆盖度。
	if len(input.Features) > 0 {
		input.EvidenceCoverage = "full"
	}

	return input
}
