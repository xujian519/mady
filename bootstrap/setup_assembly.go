package bootstrap

import (
	"log/slog"
	"os"
	"path/filepath"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/agentcore/permission"
	domainEvidence "github.com/xujian519/mady/domains/evidence"
	"github.com/xujian519/mady/guardrails"
	"github.com/xujian519/mady/knowledge/loader"
	ksqlite "github.com/xujian519/mady/knowledge/sqlite"
	"github.com/xujian519/mady/pkg/lawcite"
	"github.com/xujian519/mady/retrieval"
	"github.com/xujian519/mady/tools"
)

func BuildCitationSource(wikiRoot string) guardrails.CitationSource {
	s1 := guardrails.DefaultCitationSource()

	if wikiRoot == "" {
		return s1
	}
	legalDir := filepath.Join(wikiRoot, "legal")
	idx, err := loader.BuildLawArticleIndex(legalDir)
	if err != nil {
		slog.Error("构建 S2 法条索引失败，降级为仅 S1 静态表", "error", err)
		return s1
	}
	slog.Info("S2 法条索引已加载", "articles", idx.ArticleCount())

	s2 := guardrails.CitationSourceFuncs{
		TopicsFunc: func(s lawcite.Statute, article int) ([]string, bool) {
			if s != lawcite.StatutePatentLaw {
				return nil, false
			}
			return idx.Topics(article)
		},
		MaxArticleFunc: func(s lawcite.Statute) int {
			if s != lawcite.StatutePatentLaw {
				return 0
			}
			return idx.MaxArticle()
		},
	}
	return guardrails.CompositeCitationSource(s1, s2)
}

// ============================================================
// 装配辅助函数
// ============================================================

// LoadWikiStore initializes the knowledge retrieval system.
func openWritableStore(madyHome string, embedder retrieval.Embedder, knowledgeDBPath string) *ksqlite.WritableStore {
	if embedder == nil {
		return nil
	}
	userDBPath := os.Getenv("USER_DB_PATH")
	if userDBPath == "" {
		if madyHome == "" {
			return nil
		}
		userDBPath = filepath.Join(madyHome, "knowledge", "user.db")
	}
	if dir := filepath.Dir(userDBPath); dir != "" {
		dir = filepath.Clean(dir)
		if err := os.MkdirAll(dir, 0o750); err != nil { // #nosec G703 -- cleaned above
			slog.Error("knowledge: user.db dir create failed", "error", err)
			return nil
		}
	}
	ws, err := ksqlite.OpenWritable(userDBPath, embedder, knowledgeDBPath)
	if err != nil {
		slog.Error("knowledge: user.db open failed", "error", err)
		return nil
	}
	slog.Info("knowledge: user.db writable store active", "path", userDBPath)
	return ws
}

// DenyDangerousToolsExtension 返回一个拒绝危险工具的权限扩展。
// 适用于无交互模式（ACP/Server/Desktop），默认拒绝 bash/process/
// execute_code/browser/computer_use 等可能造成破坏的工具。
func DenyDangerousToolsExtension() agentcore.Extension {
	return permission.NewExtension(permission.Policy{
		Mode: permission.DecisionAllow,
		Deny: []permission.Rule{
			{Tool: tools.ToolBash},
			{Tool: tools.ToolProcess},
			{Tool: tools.ToolExecuteCode},
			{Tool: tools.ToolBrowser},
			{Tool: tools.ToolComputerUse},
		},
	}, permission.AlwaysDenyApprover{})
}

// newEvidenceRuleIndex 从嵌入的 evidence-rules.yaml 加载证据规则，返回预填充的 RuleIndex。
// 如果嵌入数据为空或解析失败，返回空索引（不阻断启动流程）。
func newEvidenceRuleIndex() *domainEvidence.RuleIndex {
	idx := domainEvidence.NewRuleIndex()
	data := domainEvidence.EvidenceRulesYAML()
	if len(data) == 0 {
		slog.Warn("证据规则 YAML 为空，使用空索引")
		return idx
	}
	if err := idx.LoadBytes(data); err != nil {
		slog.Warn("加载证据规则 YAML 失败", "error", err)
		return idx
	}
	slog.Info("已加载证据规则 YAML", "count", idx.Count())
	return idx
}
