package server

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/agentcore/iface"
	"github.com/xujian519/mady/disclosure"
	"github.com/xujian519/mady/domains/enablement"
	"github.com/xujian519/mady/graph"
	"github.com/xujian519/mady/knowledge"
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
	provider           agentcore.Provider
	bus                iface.EventBus
	logger             *slog.Logger
	cancel             func() // 取消订阅
	ctx                context.Context
	cancelCtx          context.CancelFunc
	resultHandler      func(taskID string, result *enablement.EnablementResult) // 可选回调，用于存储结果
	knowledgeRetriever enablement.KnowledgeRetriever                            // 可选知识检索器，用于填充审查指南和类案
}

// EnablementTriggerOption 配置 EnablementTrigger。
type EnablementTriggerOption func(*EnablementTrigger)

// WithEnablementResultHandler 设置结果回调，在 26.3 评估完成后调用。
// 典型用法：注入 s.SetEnablementResult，使结果可通过 API 查询。
func WithEnablementResultHandler(fn func(taskID string, result *enablement.EnablementResult)) EnablementTriggerOption {
	return func(t *EnablementTrigger) { t.resultHandler = fn }
}

// WithEnablementKnowledgeRetriever 设置知识检索器，用于在评估前自动检索
// 审查指南条款和类案信息，填充到评估输入的 GuidelineRefs 和 SimilarCases 字段。
// 不注入时降级为纯 LLM 知识评估。
func WithEnablementKnowledgeRetriever(r enablement.KnowledgeRetriever) EnablementTriggerOption {
	return func(t *EnablementTrigger) { t.knowledgeRetriever = r }
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
	// 知识检索增强：填充 GuidelineRefs 和 SimilarCases
	enablement.EnrichInput(ctx, input, t.knowledgeRetriever)

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

// =============================================================================
// serverKnowledgeRetriever — KnowledgeRetriever 的 server 层实现
// =============================================================================

// serverKnowledgeRetriever 是 enablement.KnowledgeRetriever 的 server 层实现，
// 组合 LawSearcher（法律/审查指南检索）和 GraphEnhancer（类案图谱检索）。
// 零值可用：当依赖项未注入时，SearchGuidelines 和 SearchSimilarCases 返回空列表。
type serverKnowledgeRetriever struct {
	lawSearcher knowledge.LawSearcher // 法律法规搜索引擎
	graphCtxFn  func() string         // 知识图谱扩展上下文（来自 KnowledgeExtension.GraphContext）
}

// NewServerKnowledgeRetriever 从 agentcore.Extension 创建 KnowledgeRetriever。
// 内部类型断言为 *knowledge.KnowledgeExtension 以获取 LawSearcher 和 GraphContext；
// 当 ext 为 nil 或类型不匹配时返回 nil（降级为纯 LLM 知识评估）。
// 典型用法：
//
//	kr := server.NewServerKnowledgeRetriever(fc.KnowledgeExt)
func NewServerKnowledgeRetriever(ext agentcore.Extension) enablement.KnowledgeRetriever {
	if ext == nil {
		return nil
	}
	kext, ok := ext.(*knowledge.KnowledgeExtension)
	if !ok {
		return nil
	}
	return &serverKnowledgeRetriever{
		lawSearcher: kext.LawSearcher(),
		graphCtxFn:  kext.GraphContext,
	}
}

// SearchGuidelines 根据技术领域和技术问题检索审查指南相关条款。
// 使用 LawSearcher 搜索 laws-full.db，关键词为技术领域标签 + 审查指南 + 主要技术问题。
// lawSearcher 为 nil 时降级返回空。
func (r *serverKnowledgeRetriever) SearchGuidelines(ctx context.Context, domain enablement.TechDomain, problems []string, features []enablement.TechFeature) ([]string, error) {
	if r.lawSearcher == nil {
		return nil, nil
	}

	// 构建搜索查询：技术领域 + 审查指南 + 技术问题
	query := buildGuidelineQuery(domain, problems, features)
	if query == "" {
		return nil, nil
	}

	// 搜索法律法规全文库，topK=5
	results, err := r.lawSearcher(query, 5)
	if err != nil {
		return nil, err
	}

	// 过滤出审查指南相关的结果，并格式化为引用文本
	var refs []string
	for _, rec := range results {
		// 仅保留审查指南或专利法相关的记录
		if isGuidelineRelevant(rec) {
			ref := formatLawRef(rec)
			if ref != "" {
				refs = append(refs, ref)
			}
		}
	}

	return refs, nil
}

// SearchSimilarCases 根据技术领域和技术特征检索类案。
// 通过 graphCtxFn 获取最近一次知识图谱增强结果中的类案信息。
// graphCtxFn 为 nil 或返回空字符串时降级返回空。
func (r *serverKnowledgeRetriever) SearchSimilarCases(ctx context.Context, domain enablement.TechDomain, features []enablement.TechFeature, problems []string) ([]string, error) {
	if r.graphCtxFn == nil {
		return nil, nil
	}

	graphCtx := r.graphCtxFn()
	if graphCtx == "" {
		return nil, nil
	}

	// 图谱上下文可能包含大量信息，取其前 3 个类案片段
	// 按换行分割，取包含案例关键词的前几条
	var cases []string
	lines := strings.Split(graphCtx, "\n")
	caseCount := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		// 保留包含案例标识的段落（案例/判决/无效宣告等）
		if isCaseLine(trimmed) {
			cases = append(cases, trimmed)
			caseCount++
			if caseCount >= 3 {
				break
			}
		}
	}

	// 如果没找到结构化案例行，取前 2 段非空内容作为兜底
	if len(cases) == 0 {
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" {
				cases = append(cases, trimmed)
				if len(cases) >= 2 {
					break
				}
			}
		}
	}

	return cases, nil
}

// buildGuidelineQuery 构建审查指南检索查询。
func buildGuidelineQuery(domain enablement.TechDomain, problems []string, features []enablement.TechFeature) string {
	var parts []string

	// 加入技术领域标签
	domainLabel := enablement.DomainLabel(domain)
	if domainLabel != "通用" {
		parts = append(parts, domainLabel)
	}

	// 加入审查指南前缀
	parts = append(parts, "审查指南")

	// 加入主要技术问题（取前 2 个）
	for i, p := range problems {
		if i >= 2 {
			break
		}
		// 截断过长的问题描述
		runes := []rune(p)
		if len(runes) > 30 {
			p = string(runes[:30])
		}
		parts = append(parts, p)
	}

	return strings.Join(parts, " ")
}

// isGuidelineRelevant 判断法律记录是否与审查指南相关。
func isGuidelineRelevant(rec knowledge.LawRecord) bool {
	name := rec.Name
	category := rec.Category
	level := rec.Level

	// 审查指南本身
	if strings.Contains(name, "审查指南") {
		return true
	}
	// 审查指南下属分类
	if strings.Contains(category, "审查指南") {
		return true
	}
	// 专利法及其实施细则（与充分公开直接相关）
	if strings.Contains(name, "专利法") || strings.Contains(name, "专利法实施细则") {
		return true
	}
	// 级别为部门规章或司法解释的专利相关规定
	if strings.Contains(level, "部门规章") && (strings.Contains(name, "专利") || strings.Contains(category, "专利")) {
		return true
	}
	// 最高法专利相关司法解释
	if strings.Contains(level, "司法解释") && strings.Contains(category, "专利") {
		return true
	}

	return false
}

// formatLawRef 将法律记录格式化为引用文本。
func formatLawRef(rec knowledge.LawRecord) string {
	name := rec.Name
	subtitle := rec.Subtitle
	content := rec.Content

	var b strings.Builder
	b.WriteString(name)
	if subtitle != "" {
		b.WriteString(" - ")
		b.WriteString(subtitle)
	}

	// 添加内容摘要
	if content != "" {
		runes := []rune(content)
		if len(runes) > 300 {
			content = string(runes[:300]) + "…"
		}
		b.WriteString("\n  ")
		b.WriteString(content)
	}

	return b.String()
}

// isCaseLine 判断文本行是否包含案例标识。
func isCaseLine(line string) bool {
	keywords := []string{"案例", "判决", "无效宣告", "复审", "案号", "决定号", "行政判决"}
	for _, kw := range keywords {
		if strings.Contains(line, kw) {
			return true
		}
	}
	return false
}
