package legal

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/xujian519/mady/agentcore"
)

// 工具名。
const (
	SearchToolName  = "legal_knowledge_search"
	InstallToolName = "legal_knowledge_install"
)

const searchToolDesc = `检索本部署的法律知识库，覆盖法规条文、司法解释、裁判案例与指导性案例四类。

- scope 选择类别：regulation（法规条文，默认）、judicial_interpretation（司法解释）、
  case_law（裁判案例）、guiding_case（指导性案例）。
- 法规与司法解释结果带 validity（in-force/expired/not-yet-effective/unverified）；
  expired 与 not-yet-effective 默认不返回，unverified 可用 include_unverified 收紧。
- 指导性案例支持「指导案例第 12 号」这类编号直接检索。
- via 标注命中通道（fts 全文、like 降级匹配、memory 内存记录）；relevance_score 越小越相关。

空结果表示本部署知识库未命中，不证明该规范或案例不存在；引用前仍需回到官方法源核对原文。`

const installToolDesc = `把源 SQLite 法律知识库安装到本机数据目录。

源库以只读方式打开，不会被修改；产物默认写到 <数据目录>/legal-knowledge/legal.db。
no_fts=true 时删除全文索引以减小体积，检索自动降级为 LIKE 匹配。
显式 output 必须位于数据目录内。`

type searchTool struct{ ext *Extension }

func newSearchTool(ext *Extension) *agentcore.Tool {
	t := &searchTool{ext: ext}
	return &agentcore.Tool{
		Name:        SearchToolName,
		Description: searchToolDesc,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "检索关键词、短语或指导案例编号",
				},
				"scope": map[string]any{
					"type":        "string",
					"enum":        scopeNames(),
					"description": "记录类别，默认 regulation",
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": fmt.Sprintf("返回条数上限（1-%d，默认 %d）", MaxLimit, DefaultLimit),
					"minimum":     1,
					"maximum":     MaxLimit,
				},
				"level": map[string]any{
					"type":        "string",
					"enum":        levelNames(),
					"description": "法规层级过滤（仅 regulation；缺省查法律与行政法规）",
				},
				"as_of": map[string]any{
					"type":        "string",
					"description": "效力判定日期 YYYY-MM-DD（仅法规与司法解释），默认今天",
				},
				"include_unverified": map[string]any{
					"type":        "boolean",
					"description": "是否返回效力未核验的记录，默认 true",
				},
			},
			"required":             []string{"query"},
			"additionalProperties": false,
		},
		Func:     t.Run,
		ReadOnly: true,
	}
}

type searchArgs struct {
	Query             string `json:"query"`
	Scope             Scope  `json:"scope"`
	Limit             int    `json:"limit"`
	Level             Level  `json:"level"`
	AsOf              string `json:"as_of"`
	IncludeUnverified *bool  `json:"include_unverified"`
}

type searchOutput struct {
	Scope      string `json:"scope"`
	Query      string `json:"query"`
	Count      int    `json:"count"`
	DataSource string `json:"data_source"`
	Hits       []Hit  `json:"hits"`
	Summary    string `json:"summary"`
}

func (t *searchTool) Run(_ context.Context, args json.RawMessage) (any, error) {
	var p searchArgs
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, fmt.Errorf("参数无效: %w", err)
	}
	query := strings.TrimSpace(p.Query)
	if query == "" {
		return nil, fmt.Errorf("缺少必填字段 'query'")
	}
	scope := p.Scope
	if scope == "" {
		scope = ScopeRegulation
	}
	if !scope.Valid() {
		return nil, fmt.Errorf("未知的检索类别 %q，可选值：%s", scope, strings.Join(scopeNames(), "、"))
	}
	if err := t.ext.searchError(); err != nil {
		return nil, err
	}
	opts := SearchOptions{
		Limit:             p.Limit,
		Level:             p.Level,
		AsOf:              p.AsOf,
		IncludeUnverified: p.IncludeUnverified,
	}
	hits, err := t.ext.searcher.Search(scope, query, opts)
	if err != nil {
		return nil, err
	}
	if hits == nil {
		hits = []Hit{}
	}
	dataSource := t.ext.searcher.DataSource()
	return searchOutput{
		Scope:      string(scope),
		Query:      query,
		Count:      len(hits),
		DataSource: dataSource,
		Hits:       hits,
		Summary:    buildSummary(scope, len(hits), dataSource, t.ext.searcher.GuidingCaseDataAvailable()),
	}, nil
}

// buildSummary 生成结果摘要：空结果与「本部署无数据」分开表述。
func buildSummary(scope Scope, count int, dataSource string, guidingData bool) string {
	if scope == ScopeGuidingCase && !guidingData {
		return "本部署未安装指导性案例数据（guiding_cases 表为空，案例库中也没有带「指导案例」标记的记录），" +
			"因此本次结果为空不表示不存在相应指导案例。可运行 " + InstallToolName + " 安装含指导性案例的数据源。"
	}
	label := dataSourceLabel(dataSource)
	if count == 0 {
		return fmt.Sprintf("未命中%s（数据来源：%s）。空结果表示本部署知识库未命中，不证明该规范或案例不存在；引用前请核对原文。",
			scope.Label(), label)
	}
	return fmt.Sprintf("命中 %d 条%s（数据来源：%s）。via 标注命中通道，relevance_score 越小越相关；引用前请核对条文编号、案号与原文。",
		count, scope.Label(), label)
}

func dataSourceLabel(dataSource string) string {
	if dataSource == "db" {
		return "本地法律知识库"
	}
	return "内存记录（未安装数据库）"
}

type installTool struct{ ext *Extension }

func newInstallTool(ext *Extension) *agentcore.Tool {
	t := &installTool{ext: ext}
	return &agentcore.Tool{
		Name:        InstallToolName,
		Description: installToolDesc,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"source": map[string]any{
					"type":        "string",
					"description": "源库路径；缺省使用数据目录下已发现的 laws-full*.db",
				},
				"output": map[string]any{
					"type":        "string",
					"description": "安装产物路径；缺省为 <数据目录>/legal-knowledge/legal.db，必须在数据目录内",
				},
				"no_fts": map[string]any{
					"type":        "boolean",
					"description": "是否删除全文索引以减小体积，默认 false",
				},
			},
			"additionalProperties": false,
		},
		Func: t.Run,
	}
}

type installArgs struct {
	Source string `json:"source"`
	Output string `json:"output"`
	NoFTS  bool   `json:"no_fts"`
}

type installOutput struct {
	InstallResult
	Note string `json:"note"`
}

func (t *installTool) Run(ctx context.Context, args json.RawMessage) (any, error) {
	var p installArgs
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, fmt.Errorf("参数无效: %w", err)
	}
	home := t.ext.cfg.HomeDir
	if home == "" {
		return nil, fmt.Errorf("未配置数据目录（MADY_HOME），无法安装法律知识库")
	}
	source := strings.TrimSpace(p.Source)
	if source == "" {
		source = t.ext.sourcePath
	}
	if source == "" {
		return nil, fmt.Errorf("未找到源库；请在数据目录的 knowledge/ 下放置 laws-full-local.db 或 laws-full.db，或用 source 指定路径")
	}
	output := strings.TrimSpace(p.Output)
	if output == "" {
		output = DefaultInstallPath(home)
	} else if !withinDir(output, home) {
		return nil, fmt.Errorf("输出路径必须位于数据目录 %s 内", home)
	}

	result, err := Install(ctx, InstallOptions{SourcePath: source, OutputPath: output, NoFTS: p.NoFTS})
	if err != nil {
		return nil, err
	}
	note := "安装完成，后续检索已切换到新库。"
	if err := t.ext.searcher.Reload(result.OutputPath); err != nil {
		note = fmt.Sprintf("安装完成，但新库未能立即生效：%v；重启后生效。", err)
	}
	return installOutput{InstallResult: result, Note: note}, nil
}

// withinDir 报告 path 是否位于 dir 之内（解析符号链接后比较）。
func withinDir(path, dir string) bool {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return false
	}
	if resolved, err := filepath.EvalSymlinks(absDir); err == nil {
		absDir = resolved
	}
	if resolved, err := filepath.EvalSymlinks(absPath); err == nil {
		absPath = resolved
	}
	rel, err := filepath.Rel(absDir, absPath)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func scopeNames() []string {
	names := make([]string, 0, len(AllScopes()))
	for _, scope := range AllScopes() {
		names = append(names, string(scope))
	}
	return names
}

func levelNames() []string {
	names := make([]string, 0, len(AllLevels()))
	for _, level := range AllLevels() {
		names = append(names, string(level))
	}
	return names
}
