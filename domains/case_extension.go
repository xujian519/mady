package domains

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/pkg/util"
)

// FileContentReader reads text content from a file path.
// Implemented outside domains (in cmd/mady) to avoid infrastructure-layer coupling.
type FileContentReader interface {
	ReadText(path string) string
}

// defaultFileReader is the fallback FileContentReader using os.ReadFile.
type defaultFileReader struct{}

func (defaultFileReader) ReadText(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

// CaseExtension 是一个 agentcore.Extension，提供 AI 内部使用的案件管理工具。
// 用户不直接调用这些工具；AI 根据对话上下文自动触发。
type CaseExtension struct {
	index  *CaseIndex
	reader FileContentReader // 文件内容读取器，避免 domains→knowledge 层级耦合
	cwd    string            // 当前工作目录（构造后不可变）
}

// NewCaseExtension 创建案件管理扩展。reader 为 nil 时使用 os.ReadFile 回退。
func NewCaseExtension(index *CaseIndex, cwd string, reader FileContentReader) *CaseExtension {
	if reader == nil {
		reader = defaultFileReader{}
	}
	return &CaseExtension{index: index, reader: reader, cwd: cwd}
}

func (e *CaseExtension) Name() string                                     { return "case-manager" }
func (e *CaseExtension) Init(_ context.Context, _ *agentcore.Agent) error { return nil }
func (e *CaseExtension) Dispose() error                                   { return nil }

// Tools 返回 AI 内部工具集。
func (e *CaseExtension) Tools() []*agentcore.Tool {
	return []*agentcore.Tool{
		{
			Name:        "list_cases",
			Description: "列出所有已注册的案件。可按客户名、申请号、年份、专利类型等过滤。返回案件列表（含标识阶段、申请号、客户名、专利名称、状态）。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"client_name": map[string]any{
						"type":        "string",
						"description": "按客户名过滤（模糊匹配）",
					},
					"year": map[string]any{
						"type":        "integer",
						"description": "按年份过滤",
					},
					"patent_type": map[string]any{
						"type":        "string",
						"description": "按专利类型过滤（发明专利/实用新型/外观设计）",
					},
					"status": map[string]any{
						"type":        "string",
						"description": "按状态过滤（active/archived/granted/rejected）",
					},
				},
			},
			Func:     e.handleListCases,
			ReadOnly: true,
		},
		{
			Name:        "search_cases",
			Description: "全文搜索案件。输入任意关键词（客户名、专利名称、申请号片段等），返回匹配的案件列表。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "搜索关键词",
					},
				},
				"required": []string{"query"},
			},
			Func:     e.handleSearchCases,
			ReadOnly: true,
		},
		{
			Name:        "sync_case",
			Description: "扫描指定目录，自动识别案件文档（申请确认书、申请文件、官文等），提取案件信息并更新案件索引。首次扫描会创建新案件记录；后续扫描检测文档变更并更新案件状态。用户在新的工作目录启动时应调用此工具。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"directory": map[string]any{
						"type":        "string",
						"description": "要扫描的目录路径（绝对或相对于当前工作目录）。留空则扫描当前工作目录。",
					},
				},
			},
			Func: e.handleSyncCase,
		},
		{
			Name:        "focus_case",
			Description: "切换当前案件上下文。多案并行时用于明确当前在处理哪个案件。按 case_id 或申请号定位案件。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"case_id": map[string]any{
						"type":        "string",
						"description": "案件 ID 或申请号",
					},
				},
				"required": []string{"case_id"},
			},
			Func: e.handleFocusCase,
		},
		{
			Name:        "register_case",
			Description: "手动创建案件记录。当 sync_case 无法自动识别文档，或用户口头提供案件信息时使用。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"client_name": map[string]any{
						"type":        "string",
						"description": "客户/申请人名称",
					},
					"patent_title": map[string]any{
						"type":        "string",
						"description": "专利名称",
					},
					"patent_type": map[string]any{
						"type":        "string",
						"description": "专利类型（发明专利/实用新型/外观设计）",
					},
					"filing_number": map[string]any{
						"type":        "string",
						"description": "申请号（有则填，撰写期留空）",
					},
					"path": map[string]any{
						"type":        "string",
						"description": "案件文件所在目录路径",
					},
				},
				"required": []string{"client_name", "patent_title", "patent_type"},
			},
			Func: e.handleRegisterCase,
		},
		{
			Name:        "upgrade_case_identity",
			Description: "升级案件标识阶段。当收到受理通知书（获得申请号）或公开公告（获得公开号）时调用。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"case_id": map[string]any{
						"type":        "string",
						"description": "案件 ID",
					},
					"filing_number": map[string]any{
						"type":        "string",
						"description": "申请号（受理通知书上的申请号）",
					},
					"publication_number": map[string]any{
						"type":        "string",
						"description": "公开号（公开公告上的公开号）",
					},
				},
				"required": []string{"case_id"},
			},
			Func: e.handleUpgradeCase,
		},
	}
}

// --- Tool handlers ---

type listCasesInput struct {
	ClientName string `json:"client_name,omitempty"`
	Year       int    `json:"year,omitempty"`
	PatentType string `json:"patent_type,omitempty"`
	Status     string `json:"status,omitempty"`
}

func (e *CaseExtension) handleListCases(ctx context.Context, args json.RawMessage) (any, error) {
	var input listCasesInput
	_ = json.Unmarshal(args, &input)

	cases, err := e.index.SearchCases(ctx, CaseSearchQuery{
		ClientName: input.ClientName,
		Year:       input.Year,
		PatentType: input.PatentType,
		Status:     input.Status,
	})
	if err != nil {
		return errorResult("list_cases", err), nil
	}
	return caseListResult(cases), nil
}

type searchCasesInput struct {
	Query string `json:"query"`
}

func (e *CaseExtension) handleSearchCases(ctx context.Context, args json.RawMessage) (any, error) {
	var input searchCasesInput
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("search_cases: %w", err)
	}

	cases, err := e.index.SearchCases(ctx, CaseSearchQuery{Text: input.Query})
	if err != nil {
		return errorResult("search_cases", err), nil
	}
	return caseListResult(cases), nil
}

type syncCaseInput struct {
	Directory string `json:"directory,omitempty"`
}

func (e *CaseExtension) handleSyncCase(ctx context.Context, args json.RawMessage) (any, error) {
	var input syncCaseInput
	_ = json.Unmarshal(args, &input)

	dir := input.Directory
	if dir == "" {
		dir = e.cwd
	}
	if dir == "" {
		dir, _ = os.Getwd()
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return errorResult("sync_case", err), nil
	}

	if _, err := os.Stat(absDir); os.IsNotExist(err) {
		return syncResult{
			Directory: absDir,
			Message:   "目录不存在",
		}, nil
	}

	// 1. 检查此目录是否已关联案件
	existing, _ := e.index.FindByPath(ctx, absDir)
	if len(existing) > 0 {
		// 已关联，增量扫描更新
		return e.incrementalScan(ctx, &existing[0], absDir)
	}

	// 2. 首次扫描：遍历文件，分类并提取信息
	docs := e.scanDirectory(absDir)
	if len(docs) == 0 {
		return syncResult{
			Directory: absDir,
			Message:   "未在目录中找到可识别的案件文档（申请确认书/申请文件/官文）。可使用 register_case 手动创建案件。",
		}, nil
	}

	// 3. 从文档提取案件信息
	merged := ExtractedCaseInfo{}
	for _, doc := range docs {
		text := e.readFileText(doc.Path)
		if text == "" {
			continue
		}
		info := ExtractFromText(doc.Type, text)
		merged = MergeExtractions(merged, info)
	}

	// 4. 去重检查：按复合标识查找已有案件
	if merged.ClientName != "" && merged.PatentTitle != "" {
		matches, _ := e.index.FindByDraftingIdentity(ctx,
			merged.ClientName, merged.PatentTitle, merged.PatentType, merged.Year)
		if len(matches) > 0 {
			// 关联到已有案件
			rec := matches[0]
			_ = e.index.AddPath(ctx, rec.CaseID, absDir, "扫描发现")
			return e.applyDocUpdates(ctx, &rec, docs, merged, absDir)
		}
	}

	// 5. 申请号去重
	if merged.FilingNumber != "" {
		existing, err := e.index.FindByFilingNumber(ctx, merged.FilingNumber)
		if err == nil && existing != nil {
			_ = e.index.AddPath(ctx, existing.CaseID, absDir, "扫描发现")
			return e.applyDocUpdates(ctx, existing, docs, merged, absDir)
		}
	}

	// 6. 创建新案件
	stage := stageForInfo(merged)

	rec := CaseRecord{
		CaseID:        uuid.New().String(),
		IdentityStage: stage,
		FilingNumber:  merged.FilingNumber,
		ClientName:    merged.ClientName,
		PatentTitle:   merged.PatentTitle,
		PatentType:    merged.PatentType,
		Year:          merged.Year,
		Domain:        DomainPatent,
		Status:        CaseStatusActive,
		PrimaryPath:   absDir,
	}
	if err := e.index.CreateCase(ctx, rec); err != nil {
		return errorResult("sync_case", err), nil
	}

	// 7. 记录文档 + 确保工作区
	for _, doc := range docs {
		_ = e.index.RecordDocument(ctx, CaseDocument{
			CaseID:  rec.CaseID,
			DocType: doc.Type,
			DocPath: doc.Path,
		})
	}
	_, _ = EnsureCaseWorkspace(absDir)

	return syncResult{
		Directory: absDir,
		CaseID:    rec.CaseID,
		Identity:  rec.PrimaryIdentity(),
		Stage:     stage,
		DocCount:  len(docs),
		IsNew:     true,
		Message:   fmt.Sprintf("已创建新案件: %s（阶段: %s，文档: %d 个）", rec.DisplayLabel(), stageLabel(stage), len(docs)),
	}, nil
}

type focusCaseInput struct {
	CaseID string `json:"case_id"`
}

func (e *CaseExtension) handleFocusCase(ctx context.Context, args json.RawMessage) (any, error) {
	var input focusCaseInput
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("focus_case: %w", err)
	}

	// 先按 CaseID 查，再按申请号查
	rec, err := e.index.GetCase(ctx, input.CaseID)
	if err != nil {
		rec, err = e.index.FindByFilingNumber(ctx, input.CaseID)
		if err != nil {
			return focusResult{Found: false, Message: fmt.Sprintf("未找到案件: %s", input.CaseID)}, nil
		}
	}

	paths, _ := e.index.GetPaths(ctx, rec.CaseID)
	docs, _ := e.index.GetDocuments(ctx, rec.CaseID)

	return focusResult{
		Found:    true,
		CaseID:   rec.CaseID,
		Identity: rec.PrimaryIdentity(),
		Label:    rec.DisplayLabel(),
		Stage:    rec.IdentityStage,
		Status:   rec.Status,
		Paths:    pathStrings(paths),
		DocCount: len(docs),
		Message:  fmt.Sprintf("已聚焦案件: %s", rec.DisplayLabel()),
	}, nil
}

type registerCaseInput struct {
	ClientName   string `json:"client_name"`
	PatentTitle  string `json:"patent_title"`
	PatentType   string `json:"patent_type"`
	FilingNumber string `json:"filing_number,omitempty"`
	Path         string `json:"path,omitempty"`
}

func (e *CaseExtension) handleRegisterCase(ctx context.Context, args json.RawMessage) (any, error) {
	var input registerCaseInput
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("register_case: %w", err)
	}

	stage := stageForInfo(ExtractedCaseInfo{FilingNumber: input.FilingNumber})

	primaryPath := input.Path
	if primaryPath == "" {
		primaryPath = e.cwd
	}

	rec := CaseRecord{
		CaseID:        uuid.New().String(),
		IdentityStage: stage,
		FilingNumber:  input.FilingNumber,
		ClientName:    input.ClientName,
		PatentTitle:   input.PatentTitle,
		PatentType:    input.PatentType,
		Year:          time.Now().Year(),
		Domain:        DomainPatent,
		Status:        CaseStatusActive,
		PrimaryPath:   primaryPath,
	}
	if err := e.index.CreateCase(ctx, rec); err != nil {
		return errorResult("register_case", err), nil
	}

	if primaryPath != "" {
		_, _ = EnsureCaseWorkspace(primaryPath)
	}

	return map[string]any{
		"case_id":  rec.CaseID,
		"identity": rec.PrimaryIdentity(),
		"stage":    rec.IdentityStage,
		"message":  fmt.Sprintf("已创建案件: %s", rec.DisplayLabel()),
	}, nil
}

type upgradeCaseInput struct {
	CaseID            string `json:"case_id"`
	FilingNumber      string `json:"filing_number,omitempty"`
	PublicationNumber string `json:"publication_number,omitempty"`
}

func (e *CaseExtension) handleUpgradeCase(ctx context.Context, args json.RawMessage) (any, error) {
	var input upgradeCaseInput
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("upgrade_case_identity: %w", err)
	}

	if input.FilingNumber != "" {
		if err := e.index.UpgradeToFiled(ctx, input.CaseID, input.FilingNumber); err != nil {
			return errorResult("upgrade_case_identity", err), nil
		}
	}
	if input.PublicationNumber != "" {
		if err := e.index.UpgradeToPublished(ctx, input.CaseID, input.PublicationNumber); err != nil {
			return errorResult("upgrade_case_identity", err), nil
		}
	}

	rec, _ := e.index.GetCase(ctx, input.CaseID)
	stage := ""
	label := ""
	if rec != nil {
		stage = rec.IdentityStage
		label = rec.DisplayLabel()
	}

	return map[string]any{
		"case_id": input.CaseID,
		"stage":   stage,
		"label":   label,
		"message": fmt.Sprintf("案件标识已升级: %s（阶段: %s）", label, stageLabel(stage)),
	}, nil
}

// --- 内部方法 ---

type scannedDoc struct {
	Path string
	Type string
}

func (e *CaseExtension) scanDirectory(dir string) []scannedDoc {
	var docs []scannedDoc

	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		// 跳过 .mady 工作区
		if strings.Contains(path, util.AppDirName+string(filepath.Separator)) {
			return nil
		}
		name := d.Name()
		// 只处理文档文件
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".pdf" && ext != ".docx" && ext != ".doc" && ext != ".txt" && ext != ".md" {
			return nil
		}
		// 读取前 2000 字符用于分类
		preview := e.readFilePreview(path, 2000)
		docType := ClassifyDocument(name, preview)
		if docType == DocOther {
			return nil
		}
		docs = append(docs, scannedDoc{Path: path, Type: docType})
		return nil
	})

	return docs
}

func (e *CaseExtension) readFileText(path string) string {
	return e.reader.ReadText(path)
}

func (e *CaseExtension) readFilePreview(path string, maxLen int) string {
	text := e.readFileText(path)
	if len(text) > maxLen {
		return text[:maxLen]
	}
	return text
}

func (e *CaseExtension) incrementalScan(ctx context.Context, rec *CaseRecord, dir string) (any, error) {
	docs := e.scanDirectory(dir)
	if len(docs) == 0 {
		return syncResult{
			Directory: dir,
			CaseID:    rec.CaseID,
			IsNew:     false,
			Message:   "未发现新文档",
		}, nil
	}

	merged := ExtractedCaseInfo{}
	newDocCount := 0
	for _, doc := range docs {
		// 检查是否已记录
		existing, _ := e.index.GetDocument(ctx, rec.CaseID, doc.Type)
		if existing != nil && existing.DocPath == doc.Path {
			continue
		}
		newDocCount++
		text := e.readFileText(doc.Path)
		if text == "" {
			continue
		}
		info := ExtractFromText(doc.Type, text)
		merged = MergeExtractions(merged, info)
		_ = e.index.RecordDocument(ctx, CaseDocument{
			CaseID:  rec.CaseID,
			DocType: doc.Type,
			DocPath: doc.Path,
		})
	}

	// 检查是否需要升级标识
	e.applyIdentityUpgrade(ctx, rec, merged)

	updated, _ := e.index.GetCase(ctx, rec.CaseID)
	stage := rec.IdentityStage
	if updated != nil {
		stage = updated.IdentityStage
	}

	return syncResult{
		Directory: dir,
		CaseID:    rec.CaseID,
		Identity:  rec.PrimaryIdentity(),
		Stage:     stage,
		DocCount:  newDocCount,
		IsNew:     false,
		Message:   fmt.Sprintf("已更新案件: %s（新增文档: %d）", rec.DisplayLabel(), newDocCount),
	}, nil
}

func (e *CaseExtension) applyDocUpdates(ctx context.Context, rec *CaseRecord, docs []scannedDoc, merged ExtractedCaseInfo, dir string) (any, error) {
	for _, doc := range docs {
		_ = e.index.RecordDocument(ctx, CaseDocument{
			CaseID:  rec.CaseID,
			DocType: doc.Type,
			DocPath: doc.Path,
		})
	}

	// 标识升级
	e.applyIdentityUpgrade(ctx, rec, merged)

	_, _ = EnsureCaseWorkspace(dir)

	updated, _ := e.index.GetCase(ctx, rec.CaseID)
	stage := rec.IdentityStage
	if updated != nil {
		stage = updated.IdentityStage
	}

	return syncResult{
		Directory: dir,
		CaseID:    rec.CaseID,
		Identity:  rec.PrimaryIdentity(),
		Stage:     stage,
		DocCount:  len(docs),
		IsNew:     false,
		Message:   fmt.Sprintf("已关联到已有案件: %s（文档: %d）", rec.DisplayLabel(), len(docs)),
	}, nil
}

// applyIdentityUpgrade upgrades case identity based on extracted info.
// Logs warnings on failure instead of silently swallowing errors.
func (e *CaseExtension) applyIdentityUpgrade(ctx context.Context, rec *CaseRecord, merged ExtractedCaseInfo) {
	if merged.FilingNumber != "" && rec.IdentityStage == StageDrafting {
		if err := e.index.UpgradeToFiled(ctx, rec.CaseID, merged.FilingNumber); err != nil {
			slog.Warn("case_extension: identity upgrade to filed failed",
				"case_id", rec.CaseID, "err", err)
		}
	}
	if merged.PublicationNumber != "" && rec.IdentityStage != StagePublished {
		if err := e.index.UpgradeToPublished(ctx, rec.CaseID, merged.PublicationNumber); err != nil {
			slog.Warn("case_extension: identity upgrade to published failed",
				"case_id", rec.CaseID, "err", err)
		}
	}
}

// stageForInfo determines the identity stage based on extracted info.
func stageForInfo(info ExtractedCaseInfo) string {
	if info.PublicationNumber != "" {
		return StagePublished
	}
	if info.FilingNumber != "" {
		return StageFiled
	}
	return StageDrafting
}

// --- 响应类型 ---

type caseListItem struct {
	CaseID       string `json:"case_id"`
	Identity     string `json:"identity"`
	Label        string `json:"label"`
	Stage        string `json:"stage"`
	FilingNumber string `json:"filing_number,omitempty"`
	ClientName   string `json:"client_name"`
	PatentTitle  string `json:"patent_title"`
	PatentType   string `json:"patent_type"`
	Year         int    `json:"year"`
	Status       string `json:"status"`
}

type caseListResponse struct {
	Total int            `json:"total"`
	Cases []caseListItem `json:"cases"`
}

func caseListResult(cases []CaseRecord) caseListResponse {
	items := make([]caseListItem, 0, len(cases))
	for _, c := range cases {
		items = append(items, caseListItem{
			CaseID:       c.CaseID,
			Identity:     c.PrimaryIdentity(),
			Label:        c.DisplayLabel(),
			Stage:        c.IdentityStage,
			FilingNumber: c.FilingNumber,
			ClientName:   c.ClientName,
			PatentTitle:  c.PatentTitle,
			PatentType:   c.PatentType,
			Year:         c.Year,
			Status:       c.Status,
		})
	}
	return caseListResponse{Total: len(items), Cases: items}
}

type syncResult struct {
	Directory string `json:"directory"`
	CaseID    string `json:"case_id,omitempty"`
	Identity  string `json:"identity,omitempty"`
	Stage     string `json:"stage,omitempty"`
	DocCount  int    `json:"doc_count"`
	IsNew     bool   `json:"is_new"`
	Message   string `json:"message"`
}

type focusResult struct {
	Found    bool     `json:"found"`
	CaseID   string   `json:"case_id,omitempty"`
	Identity string   `json:"identity,omitempty"`
	Label    string   `json:"label,omitempty"`
	Stage    string   `json:"stage,omitempty"`
	Status   string   `json:"status,omitempty"`
	Paths    []string `json:"paths,omitempty"`
	DocCount int      `json:"doc_count"`
	Message  string   `json:"message"`
}

func errorResult(tool string, err error) map[string]any {
	return map[string]any{"error": err.Error(), "message": fmt.Sprintf("%s 失败: %s", tool, err.Error())}
}

func stageLabel(stage string) string {
	switch stage {
	case StageDrafting:
		return "撰写期"
	case StageFiled:
		return "已申请"
	case StagePublished:
		return "已公开"
	default:
		return stage
	}
}

func pathStrings(paths []CasePath) []string {
	result := make([]string, 0, len(paths))
	for _, p := range paths {
		result = append(result, p.Path)
	}
	return result
}
