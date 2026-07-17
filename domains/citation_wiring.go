package domains

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/guardrails"
)

// 本文件是引用核验 Gate 的域装配注入点（docs/design/citation-verification-gate.md
// §9 P2b）。
//
// PatentAgentConfig / LegalAgentConfig / BuildProjectAgent 经 router.go 的
// domainFactoryMap 函数值被 Router 调用，签名不可动（否则 manifest 装配路径
// 连锁修改）；因此 S2 知识源与 disclosure 留痕 store 由装配侧
// （cmd/mady setupFrameworkContext）在启动期一次性注入，域工厂构建
// Agent 配置时读取。注入必须先于任何 Agent 运行（启动期单线程），
// atomic.Value 保证读取端并发安全。

// CitationWiring 是引用核验 Gate 的装配侧注入。
type CitationWiring struct {
	// Source 是 S1+S2 复合知识源；nil 时 Gate 退回 S1 静态表（P1b 行为）。
	Source guardrails.CitationSource
	// Store 是 disclosure 留痕后端（approvals.db）；
	// nil 时命中疑点仅追加提示，不写留痕（跑批/冒烟默认）。
	Store ApprovalStore
}

// citationWiring 持有当前装配（atomic.Value 存 CitationWiring）。
var citationWiring atomic.Value

// SetupCitationWiring 在启动期一次性注入引用核验装配（cmd/mady 调用）。
// 可重复调用（后值覆盖前值），但必须在任何 Agent 运行前完成。
func SetupCitationWiring(w CitationWiring) {
	citationWiring.Store(w)
}

// currentCitationWiring 读取当前装配；未注入时返回零值
// （S1 静态表 + 不留痕），保证 scripts/ 等不走装配侧的调用方行为不变。
func currentCitationWiring() CitationWiring {
	if v := citationWiring.Load(); v != nil {
		return v.(CitationWiring)
	}
	return CitationWiring{}
}

// newCitationGate 按当前装配构建域级引用核验 Gate（P2b 起 Strict 档）：
// 命中疑点 → 追加存疑提示 + citation_verify 留痕 + SuppressPersist
// （未人工复核的输出不写入会话存储）。
// sessionID/caseID 是留痕归属：静态域传域名（DomainPatent/DomainLegal），
// 案件 Agent 传 Agent 名与案件号（ListByCase 可按案件查待审）。
func newCitationGate(sessionID, caseID string) agentcore.LifecycleHook {
	w := currentCitationWiring()
	return guardrails.NewCitationGate(
		guardrails.WithCitationGateLevel(guardrails.LevelStrict),
		guardrails.WithCitationSource(w.Source),
		guardrails.WithCitationRecorder(citationRecorder(w.Store, sessionID, caseID)),
	)
}

// citationRecorder 构建域级留痕闭包：命中疑点写 ApprovalRecord
// （trigger_keyword="citation_verify"）。
//
// Decision 留空（StateNone）表达「已记录、待人工复核」——citation 留痕是
// disclosure 性质，不走交互式审批状态机；待审列表可按
// trigger_keyword='citation_verify' AND decision=” 过滤。
// store 为 nil 时返回 nil（Gate 仅标注）；写库失败不阻断 Agent，
// 仅 stderr 记录（留痕是审计增强，不是主流程）。
func citationRecorder(store ApprovalStore, sessionID, caseID string) func(guardrails.CitationReport, string) {
	if store == nil {
		return nil
	}
	return func(report guardrails.CitationReport, content string) {
		feedback := fmt.Sprintf("引用核验命中 %d 条疑点（Suspect %d / Invalid %d），原始输出已抑制持久化，待人工复核",
			len(report.Flagged), report.Suspect, report.Invalid)
		if err := RecordApprovalDecision(
			context.Background(), store,
			sessionID, caseID, "citation_verify", content,
			"", "", feedback,
		); err != nil {
			fmt.Fprintf(os.Stderr, "citation: 留痕写入失败: %v\n", err)
		}
	}
}
