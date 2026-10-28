package patenttools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/knowledge/legal"
	ksqlite "github.com/xujian519/mady/knowledge/sqlite"
)

// KnowledgeStatusToolName 是知识库状态与安装引导工具的注册名。
const KnowledgeStatusToolName = "knowledge_status"

// KnowledgeStatusConfig 配置状态工具。
type KnowledgeStatusConfig struct {
	// MadyHome 是数据目录根（$MADY_HOME），用于解析法规库与安装产物路径。
	MadyHome string
}

// KnowledgeStatusReport 是知识库状态报告。
type KnowledgeStatusReport struct {
	Documents       int      `json:"documents"`
	Chunks          int      `json:"chunks"`
	Embeddings      int      `json:"embeddings"`
	EmbeddingDim    int      `json:"embedding_dim"`
	LawsAvailable   bool     `json:"laws_available"`
	LawsCount       int      `json:"laws_count"`
	LawsFTS         bool     `json:"laws_fts"`
	LegalInstalled  bool     `json:"legal_installed"`
	LegalQueryPath  string   `json:"legal_query_path,omitempty"`
	LegalSourcePath string   `json:"legal_source_path,omitempty"`
	Guidance        []string `json:"guidance"`
	Rendered        string   `json:"rendered"`
}

const statusToolDesc = `报告本部署知识库的可用状态，并给出缺什么、怎么装。

- knowledge.db：文档/切片/向量数与向量维度（专利 wiki、法规、判例等入库量）；
- 法规库：是否打开、law 表行数、是否带 FTS5 全文索引；
- 法律知识库快照：查询路径与源库路径，是否已完成 legal_knowledge_install 安装；
- 按调用结果给出安装/刷新指引（放置文件、运行哪个工具）。

只读：不修改任何文件，也不触发索引重建。`

// NewKnowledgeStatusTool 创建 knowledge_status 工具。
func NewKnowledgeStatusTool(store *ksqlite.SQLiteStore, cfg KnowledgeStatusConfig) *agentcore.Tool {
	t := &statusTool{store: store, cfg: cfg}
	return &agentcore.Tool{
		Name:        KnowledgeStatusToolName,
		Description: statusToolDesc,
		Parameters: map[string]any{
			"type":                 "object",
			"properties":           map[string]any{},
			"additionalProperties": false,
		},
		Func:     t.Run,
		ReadOnly: true,
	}
}

type statusTool struct {
	store *ksqlite.SQLiteStore
	cfg   KnowledgeStatusConfig
}

func (t *statusTool) Run(_ context.Context, args json.RawMessage) (any, error) {
	var input struct{}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &input); err != nil {
			return nil, fmt.Errorf("参数无效: %w", err)
		}
	}
	report := t.report()
	return report, nil
}

func (t *statusTool) report() KnowledgeStatusReport {
	report := KnowledgeStatusReport{}
	if t.store != nil {
		stats := t.store.Stats()
		report.Documents = stats.Documents
		report.Chunks = stats.Chunks
		report.Embeddings = stats.Embeddings
		report.EmbeddingDim = stats.Dim
		report.LawsAvailable = t.store.LawsAvailable()
		report.LawsCount = t.store.LawsCount()
		report.LawsFTS = t.store.HasLawFTS()
	}
	if t.cfg.MadyHome != "" {
		queryPath, sourcePath := legal.ResolvePaths(t.cfg.MadyHome)
		report.LegalQueryPath = queryPath
		report.LegalSourcePath = sourcePath
		report.LegalInstalled = legal.DefaultInstallPath(t.cfg.MadyHome) == queryPath && queryPath != ""
	}
	report.Guidance = buildStatusGuidance(report)
	report.Rendered = renderStatus(report)
	return report
}

func buildStatusGuidance(report KnowledgeStatusReport) []string {
	var guidance []string
	if report.Documents == 0 {
		guidance = append(guidance,
			"knowledge.db 尚无文档：把专利 wiki / 法规 / 判例的 Markdown 放入 $MADY_HOME/data/knowledge/（启动时由 knowledgeinit 加载），或用 add_document 逐篇导入。")
	}
	if !report.LawsAvailable || report.LawsCount == 0 {
		guidance = append(guidance,
			"未检测到法规库：把 laws-full-local.db（带 FTS5）或 laws-full.db 放到 $MADY_HOME/knowledge/；已有源库时运行 legal_knowledge_install 生成只读快照。")
	} else if !report.LegalInstalled {
		guidance = append(guidance,
			"法规库当前直接只读使用源文件：运行 legal_knowledge_install 可生成 VACUUM INTO 快照到 $MADY_HOME/legal-knowledge/legal.db，避免源库 WAL 未落盘造成不一致。")
	}
	if !report.LawsFTS && report.LawsAvailable {
		guidance = append(guidance,
			"法规库没有 FTS5 索引：短查询会走 LIKE 降级；换用 laws-full-local.db 或保留 FTS 重新安装可恢复 BM25 检索。")
	}
	if report.Embeddings == 0 {
		guidance = append(guidance,
			"未预载向量：向量检索通道不可用；启动嵌入服务（mady start-embeddings）后重建索引即可启用。")
	}
	if len(guidance) == 0 {
		guidance = append(guidance, "知识库各项均可用；引用前仍应回到官方法源核对原文。")
	}
	return guidance
}

func renderStatus(report KnowledgeStatusReport) string {
	var builder strings.Builder
	builder.WriteString("【知识库状态】\n")
	fmt.Fprintf(&builder, "- knowledge.db：文档 %d、切片 %d、向量 %d（维度 %d）\n",
		report.Documents, report.Chunks, report.Embeddings, report.EmbeddingDim)
	if report.LawsAvailable {
		fmt.Fprintf(&builder, "- 法规库：已打开，law 表 %d 行，FTS5=%t\n", report.LawsCount, report.LawsFTS)
	} else {
		builder.WriteString("- 法规库：未打开\n")
	}
	if report.LegalQueryPath != "" {
		fmt.Fprintf(&builder, "- 法规查询路径：%s（已安装快照=%t）\n", report.LegalQueryPath, report.LegalInstalled)
	}
	if report.LegalSourcePath != "" && report.LegalSourcePath != report.LegalQueryPath {
		fmt.Fprintf(&builder, "- 法规源库：%s\n", report.LegalSourcePath)
	}
	builder.WriteString("\n指引：\n")
	for _, item := range report.Guidance {
		builder.WriteString("- " + item + "\n")
	}
	return strings.TrimRight(builder.String(), "\n")
}
