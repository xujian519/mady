package domains

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

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
