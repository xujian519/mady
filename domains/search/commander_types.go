package search

import (
	"github.com/xujian519/mady/retrieval/domain"
)

// Scene 标识检索场景，决定采用哪套策略模板。
type Scene string

const (
	// SceneAuto 自动识别场景（默认）。
	SceneAuto Scene = "auto"
	// SceneOA OA 答复 — 现有技术调查：宽搜 → IPC 引证过滤 → 二次验证。
	SceneOA Scene = "oa"
	// SceneInvalidation 无效宣告 — 证据收集：宽搜 → 引证回溯 → 穷举覆盖。
	SceneInvalidation Scene = "invalidation"
	// SceneInfringement 侵权分析：特征分解 → 同族扩展 → 国际化。
	SceneInfringement Scene = "infringement"
	// SceneFTO 自由实施（FTO）：多分类号并行 → 引用网络 → 交叉验证。
	SceneFTO Scene = "fto"
	// SceneAcademic 学术+专利混合调研：学术宽搜 → 专利窄搜 → 交叉验证。
	SceneAcademic Scene = "academic"
)

// RoundPhase 描述一轮检索的阶段语义。
type RoundPhase string

const (
	// PhaseBroad 宽语义检索（第一轮，多源并行）。
	PhaseBroad RoundPhase = "宽语义检索"
	// PhaseIPC IPC/引证过滤（追加分类号与申请人约束）。
	PhaseIPC RoundPhase = "IPC/引证过滤"
	// PhaseVerify 基于已读文本的二次检索（新术语扩展）。
	PhaseVerify RoundPhase = "二次验证"
	// PhaseExhaust 穷举覆盖（多申请人/多关键词并行）。
	PhaseExhaust RoundPhase = "穷举覆盖"
	// PhaseFamily 同族/国际化扩展。
	PhaseFamily RoundPhase = "同族/国际化扩展"
	// PhaseCross 交叉验证（专利与学术互验）。
	PhaseCross RoundPhase = "交叉验证"
)

// QueryPlan 描述一轮检索的查询构造策略。
type QueryPlan struct {
	// Phase 阶段名。
	Phase RoundPhase
	// UseIPC 是否在查询中追加 IPC 约束。
	UseIPC bool
	// UseApplicant 是否按上轮提取的申请人过滤。
	UseApplicant bool
	// ExpandKeywords 是否用上轮新术语扩充关键词。
	ExpandKeywords bool
}

// Strategy 一个场景的多轮策略模板。
type Strategy struct {
	// Name 策略名称。
	Name string
	// Plans 各轮查询计划（按序执行）。
	Plans []QueryPlan
}

// strategies 五个场景 + 默认兜底的策略模板。
var strategies = map[Scene]Strategy{
	SceneOA: {
		Name: "现有技术调查（OA 答复）",
		Plans: []QueryPlan{
			{Phase: PhaseBroad},
			{Phase: PhaseIPC, UseIPC: true, UseApplicant: true},
			{Phase: PhaseVerify, ExpandKeywords: true},
		},
	},
	SceneInvalidation: {
		Name: "证据收集（无效宣告）",
		Plans: []QueryPlan{
			{Phase: PhaseBroad},
			{Phase: PhaseIPC, UseIPC: true},
			{Phase: PhaseExhaust, UseApplicant: true, ExpandKeywords: true},
		},
	},
	SceneInfringement: {
		Name: "侵权风险排查",
		Plans: []QueryPlan{
			{Phase: PhaseBroad},
			{Phase: PhaseIPC, UseIPC: true},
			{Phase: PhaseFamily, ExpandKeywords: true},
		},
	},
	SceneFTO: {
		Name: "自由实施（FTO）",
		Plans: []QueryPlan{
			{Phase: PhaseBroad},
			{Phase: PhaseIPC, UseIPC: true, UseApplicant: true},
			{Phase: PhaseCross, ExpandKeywords: true},
		},
	},
	SceneAcademic: {
		Name: "学术+专利混合调研",
		Plans: []QueryPlan{
			{Phase: PhaseBroad},
			{Phase: PhaseCross, ExpandKeywords: true},
			{Phase: PhaseVerify, UseIPC: true},
		},
	},
}

// defaultStrategy 兜底策略（场景未识别时使用）。
var defaultStrategy = Strategy{
	Name: "通用渐进式检索",
	Plans: []QueryPlan{
		{Phase: PhaseBroad},
		{Phase: PhaseIPC, UseIPC: true},
		{Phase: PhaseVerify, ExpandKeywords: true},
	},
}

// Request 一次检索编排的输入。
type Request struct {
	// Query 检索主题（必填），如 "骨髓腔输液装置的现有技术"。
	Query string
	// Scene 场景；SceneAuto 时自动识别。
	Scene Scene
	// Country 国家过滤（可选）："cn" 中国 / "global" 全球。空表示不限定。
	Country string
	// IPCs 已知 IPC 约束（可选），如 "G06F 17/30"。
	IPCs []string
	// Filters 附加结构化过滤（可选），如 {"country": "cn", "applicant": "华为"}。
	// 直接透传给检索器的 DomainQuery.Filters。
	Filters map[string]string
	// MaxRounds 最大轮次，默认 4（对齐 SKILL 规则 4）。
	MaxRounds int
	// PerRound 每轮返回条数，默认 10。
	PerRound int
}

// RoundRecord 单轮检索的执行记录（对齐 search-round-N.md）。
type RoundRecord struct {
	// Number 轮次序号（从 1 开始）。
	Number int
	// Phase 阶段名。
	Phase RoundPhase
	// Query 本轮实际查询串。
	Query string
	// Reason 本轮查询构造理由。
	Reason string
	// TotalHits 本轮总命中数。
	TotalHits int
	// TopDocs 本轮高相关结果（最多 PerRound 条）。
	TopDocs []domain.DomainDocument
	// NewTerms 本轮新发现的术语。
	NewTerms []string
	// NewApplicants 本轮新发现的申请人。
	NewApplicants []string
	// Stop 是否因收敛而停止后续轮次。
	Stop bool
	// Note 反思结论（命中量评估等）。
	Note string
}

// CompareDoc 对比文件总表中的一条记录。
type CompareDoc struct {
	// Number 文献号（公开号）。
	Number string
	// Title 标题。
	Title string
	// Source 来源（数据源名）。
	Source string
	// Round 命中轮次。
	Round int
	// URL 原文链接。
	URL string
	// Assignee 申请人（可空）。
	Assignee string
}

// Report 编排器最终报告（对齐 search-commander-report.md）。
type Report struct {
	// Target 检索目标。
	Target string
	// Strategy 采用的策略名称。
	Strategy string
	// Rounds 各轮记录。
	Rounds []RoundRecord
	// Table 对比文件总表（按文献号去重、去劣）。
	Table []CompareDoc
	// Gaps 遗漏分析（未覆盖的维度）。
	Gaps []string
	// Conclusion 结论与建议。
	Conclusion string
}

// Commander 检索编排器：确定性多轮渐进式检索。
type Commander struct {
	retriever domain.DomainRetriever
	maxRounds int
	perRound  int
}

// Option 是 Commander 的可选配置。
type Option func(*Commander)

// WithMaxRounds 覆盖默认最大轮次（默认 4）。
func WithMaxRounds(n int) Option {
	return func(c *Commander) {
		if n > 0 {
			c.maxRounds = n
		}
	}
}

// WithPerRound 覆盖每轮条数（默认 10）。
func WithPerRound(n int) Option {
	return func(c *Commander) {
		if n > 0 {
			c.perRound = n
		}
	}
}

// NewCommander 构造编排器。retriever 为 nil 时返回 nil（调用方降级处理）。
func NewCommander(retriever domain.DomainRetriever, opts ...Option) *Commander {
	if retriever == nil {
		return nil
	}
	c := &Commander{
		retriever: retriever,
		maxRounds: 4,
		perRound:  10,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}
