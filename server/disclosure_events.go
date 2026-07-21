package server

import (
	"context"
	"log/slog"
	"time"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/agentcore/iface"
	"github.com/xujian519/mady/disclosure"
	"github.com/xujian519/mady/domains/inventiveness"
	"github.com/xujian519/mady/graph"
)

// =============================================================================
// DisclosureCompletedEvent
// =============================================================================

// EventDisclosureCompleted 是 disclosure 分析管线完成/终止时的事件类型。
// 由 disclosureTaskManager.executeTask 在锁内自动发射，供下游消费者
// （如 InventivenessTrigger）通过 EventBus.OnAll 接收。
const EventDisclosureCompleted agentcore.EventType = "disclosure_completed"

// DisclosureCompletedEvent 承载 disclosure 管线完成后的报告与元数据。
type DisclosureCompletedEvent struct {
	at               time.Time
	TaskID           string
	Report           *disclosure.AnalysisReport
	Err              string // 空字符串表示成功
	EvidenceChunks   []disclosure.EvidenceChunk
	EvidenceCoverage string
}

// EventKind 实现 agentcore.Event 接口。
func (e DisclosureCompletedEvent) EventKind() agentcore.EventType { return EventDisclosureCompleted }

// EventTime 实现 agentcore.Event 接口。
func (e DisclosureCompletedEvent) EventTime() time.Time { return e.at }

// =============================================================================
// InventivenessTrigger
// =============================================================================

// InventivenessTrigger 通过 EventBus 监听 disclosure 完成事件，
// 自动触发创造性分析（三步法）独立 Pregel 子图。
//
// 设计原则：
//   - 完全独立：不修改 disclosure 管线代码，只订阅事件
//   - 异步执行：不阻塞 disclosure 管线的主流程
//   - 容错运行：子图失败仅记录日志，不影响上游
type InventivenessTrigger struct {
	provider      agentcore.Provider
	bus           iface.EventBus
	logger        *slog.Logger
	cancel        func() // 取消订阅
	ctx           context.Context
	cancelCtx     context.CancelFunc
	resultHandler func(taskID string, result *inventiveness.InventivenessResult) // 可选回调，用于存储结果
}

// InventivenessTriggerOption 配置 InventivenessTrigger。
type InventivenessTriggerOption func(*InventivenessTrigger)

// WithInventivenessResultHandler 设置结果回调，在创造性分析完成后调用。
// 典型用法：注入 s.SetInventivenessResult，使结果可通过 API 查询。
func WithInventivenessResultHandler(fn func(taskID string, result *inventiveness.InventivenessResult)) InventivenessTriggerOption {
	return func(t *InventivenessTrigger) { t.resultHandler = fn }
}

// NewInventivenessTrigger 创建创造性分析触发器。
// provider 用于运行创造性分析子图的 LLM 调用。
// bus 是事件总线引用。
func NewInventivenessTrigger(provider agentcore.Provider, bus iface.EventBus, opts ...InventivenessTriggerOption) *InventivenessTrigger {
	ctx, cancel := context.WithCancel(context.Background())
	t := &InventivenessTrigger{
		provider:  provider,
		bus:       bus,
		logger:    slog.Default().With("component", "inventiveness_trigger"),
		ctx:       ctx,
		cancelCtx: cancel,
	}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// Start 开始监听 disclosure 完成事件。
// 每次 disclosure 分析完成后自动触发创造性三步法评估。
func (t *InventivenessTrigger) Start() {
	t.cancel = t.bus.OnAll(t.onEvent)
}

// Stop 停止监听并取消正在进行的分析，释放订阅资源。
func (t *InventivenessTrigger) Stop() {
	if t.cancel != nil {
		t.cancel()
	}
	t.cancelCtx()
}

// onEvent 是 EventBus 回调：筛选 disclosure_completed 事件并执行创造性分析。
func (t *InventivenessTrigger) onEvent(ev iface.Event) {
	if ev.Type() != iface.EventType(EventDisclosureCompleted) {
		return
	}
	completed, ok := ev.Payload().(DisclosureCompletedEvent)
	if !ok {
		return
	}
	// 报告缺失或执行失败时跳过创造性分析。
	if completed.Report == nil || completed.Err != "" {
		return
	}
	report := completed.Report
	if report.Extraction == nil && report.Novelty == nil {
		t.logger.Debug("inventiveness: 跳过（报告无提取数据）", "task_id", completed.TaskID)
		return
	}

	// 检查触发器的 context 是否已被取消（Stop 被调用）。
	if err := t.ctx.Err(); err != nil {
		t.logger.Debug("inventiveness: 跳过（触发器已停止）", "task_id", completed.TaskID)
		return
	}

	t.logger.Info("inventiveness: 开始三步法创造性评估",
		"task_id", completed.TaskID,
	)

	input := buildInventivenessInput(report, completed.EvidenceChunks, completed.EvidenceCoverage)
	result, err := t.runGraph(t.ctx, input)
	if err != nil {
		t.logger.Error("inventiveness: 子图执行失败",
			"task_id", completed.TaskID,
			"error", err,
		)
		return
	}

	t.logger.Info("inventiveness: 三步法评估完成",
		"task_id", completed.TaskID,
		"assessed", result.Assessed,
		"skipped", result.Skipped,
		"confidence", result.Confidence,
	)

	// 通过回调将结果写回 Server 的 inventivenessResults map。
	if t.resultHandler != nil {
		t.resultHandler(completed.TaskID, result)
	}
}

// runGraph 构建并执行创造性分析 Pregel 子图。
func (t *InventivenessTrigger) runGraph(ctx context.Context, input *inventiveness.InventivenessInput) (*inventiveness.InventivenessResult, error) {
	compiled, err := inventiveness.BuildInventivenessGraph(t.provider)
	if err != nil {
		return nil, err
	}

	state := graph.PregelState{}
	// 用 inventiveness 包约定的 key 写入输入数据。
	state["inventiveness_input"] = input

	state, runErr := compiled.Run(ctx, state)

	// 从 state 中提取结果。
	if raw, ok := state["inventiveness_result"]; ok {
		if result, ok := raw.(*inventiveness.InventivenessResult); ok {
			return result, runErr
		}
	}

	// 结果缺失：可能有中断但评估仍在进行中。
	if runErr != nil {
		return nil, runErr
	}
	// 不返回 nil result：保证调用方不会对 nil 解引用。
	return &inventiveness.InventivenessResult{
		Assessed:   false,
		Skipped:    true,
		SkipReason: "子图执行后未找到结果（state key 缺失或类型异常）",
	}, nil
}

// buildInventivenessInput 从 disclosure 分析报告构建创造性分析子图的输入。
func buildInventivenessInput(report *disclosure.AnalysisReport, evidence []disclosure.EvidenceChunk, coverage string) *inventiveness.InventivenessInput {
	input := &inventiveness.InventivenessInput{
		EvidenceCoverage: coverage,
	}
	if input.EvidenceCoverage == "" {
		input.EvidenceCoverage = "partial"
	}

	// 1. 转换现有技术证据片段。
	for _, c := range evidence {
		input.PriorArtChunks = append(input.PriorArtChunks, inventiveness.EvidenceChunk{
			DocID:   c.DocID,
			Title:   c.Title,
			Snippet: c.Snippet,
			Score:   c.Score,
		})
	}

	// 2. 转换技术特征。
	if report.Extraction != nil {
		for _, f := range report.Extraction.Features {
			input.Features = append(input.Features, inventiveness.TechFeature{
				ID:          f.ID,
				Description: f.Description,
				Category:    string(f.Category),
				Function:    f.Function,
				Importance:  f.Importance,
			})
		}

		// 3. 转换 PFE 三元组（问题-特征-效果）。
		for _, t := range report.Extraction.PFETriples {
			input.PFETriples = append(input.PFETriples, inventiveness.PFETriple{
				ID:      t.ID,
				Problem: t.Problem,
				Effect:  t.Effect,
			})
		}
	}

	// 4. 特征非空时覆盖度提升为 full（与 disclosure 管线对齐）。
	if len(input.Features) > 0 && input.EvidenceCoverage == "partial" {
		input.EvidenceCoverage = "full"
	}

	// 5. 新颖性初判结论（作为三步法第 1 步的辅助参考）。
	if report.Novelty != nil {
		input.NoveltyConclusion = report.Novelty.Conclusion
	}

	return input
}
