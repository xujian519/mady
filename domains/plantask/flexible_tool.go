package plantask

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/pkg/util"
)

// FlexiblePlanStage is a single stage in a flexible plan.
type FlexiblePlanStage struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Goal     string `json:"goal"`
	Strategy string `json:"strategy"`
	Status   string `json:"status"`
}

// FlexiblePlan is the persisted plan state.
type FlexiblePlan struct {
	CaseID       string              `json:"caseId"`
	CaseType     string              `json:"caseType"`
	InputText    string              `json:"inputText"`
	Status       string              `json:"status"`
	Stages       []FlexiblePlanStage `json:"stages"`
	CurrentStage string              `json:"currentStage,omitempty"`
	CreatedAt    string              `json:"createdAt"`
	UpdatedAt    string              `json:"updatedAt"`
}

// FlexiblePlanInput is the tool input shape.
type FlexiblePlanInput struct {
	Action      string              `json:"action"`
	CaseID      string              `json:"case_id"`
	CaseType    string              `json:"case_type"`
	InputText   string              `json:"input_text"`
	Stages      []FlexiblePlanStage `json:"stages"`
	Stage       *FlexiblePlanStage  `json:"stage"`
	StageID     string              `json:"stage_id"`
	StageIds    []string            `json:"stage_ids"`
	Reason      string              `json:"reason"`
	AutoConfirm bool                `json:"autoConfirm"`
}

// FlexiblePlanStore persists plans to disk.
type FlexiblePlanStore struct {
	baseDir string
	mu      sync.Mutex
}

// NewFlexiblePlanStore creates a store backed by the given directory.
func NewFlexiblePlanStore(baseDir string) *FlexiblePlanStore {
	return &FlexiblePlanStore{baseDir: baseDir}
}

// DefaultFlexiblePlanStore returns a store under ~/.mady/flexible-plans.
func DefaultFlexiblePlanStore() *FlexiblePlanStore {
	home, err := util.MadyHome()
	if err != nil {
		home = ""
	}
	return NewFlexiblePlanStore(filepath.Join(home, "flexible-plans"))
}

// isSafeCaseID 判定 caseID 是否是纯 base-name 标识符（不含路径分隔符、非 "." / ".."）。
// case_id 完全由 LLM（或外部调用者）控制，若不净化可借 "../" 逃逸 flexible-plans
// 目录，读写任意 .json 文件。与 inventiveness.CaseFeedbackDir 的净化规则保持一致。
// 注意：显式拒绝 \ 与 /（Windows 上 \ 也是分隔符；POSIX 上虽不是，但作为逻辑标识符
// 出现反斜杠亦属异常，统一拒绝避免平台差异带来的逃逸）。
func isSafeCaseID(caseID string) bool {
	if caseID == "" {
		return false
	}
	if strings.ContainsAny(caseID, `/\`) {
		return false
	}
	if caseID == "." || caseID == ".." {
		return false
	}
	if filepath.Base(caseID) != caseID {
		return false
	}
	return true
}

// path 返回 caseID 对应的计划文件路径；caseID 不合法时返回错误（拒绝而非落入
// baseDir 之外）。baseDir 为受信任前缀，caseID 经 isSafeCaseID 净化后不可能逃逸。
func (s *FlexiblePlanStore) path(caseID string) (string, error) {
	if !isSafeCaseID(caseID) {
		return "", fmt.Errorf("非法 case_id %q（仅允许不含路径分隔符的标识符）", caseID)
	}
	return filepath.Join(s.baseDir, caseID+".json"), nil
}

// Load reads a plan by case ID.
func (s *FlexiblePlanStore) Load(caseID string) (*FlexiblePlan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.path(caseID)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p) //nolint:gosec // caseID 已净化为 base name，baseDir 为可信前缀
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("计划 %q 不存在", caseID)
		}
		return nil, err
	}
	var plan FlexiblePlan
	if err := json.Unmarshal(data, &plan); err != nil {
		return nil, err
	}
	return &plan, nil
}

// Save writes a plan to disk.
func (s *FlexiblePlanStore) Save(plan *FlexiblePlan) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.baseDir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return err
	}
	p, err := s.path(plan.CaseID)
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0600)
}

// NewFlexiblePlanTool creates the patent_flexible_plan tool.
func NewFlexiblePlanTool(store *FlexiblePlanStore) *agentcore.Tool {
	if store == nil {
		store = DefaultFlexiblePlanStore()
	}
	return &agentcore.Tool{
		Name:        "patent_flexible_plan",
		Description: "灵活计划工具（阶段级 HITL）：创建计划、逐阶段确认/回退、运行时增删改阶段、执行未确认阶段。计划按 caseId 持久化。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action":      map[string]any{"type": "string", "enum": []string{"create", "get", "run", "confirm", "rollback", "add", "remove", "complete", "abandon"}, "description": "操作类型"},
				"case_id":     map[string]any{"type": "string", "description": "计划主键（全部操作必需）"},
				"case_type":   map[string]any{"type": "string", "description": "案件类型（create 必需）"},
				"input_text":  map[string]any{"type": "string", "description": "案件原始输入文本（create/run 用）"},
				"stages":      map[string]any{"type": "array", "description": "阶段定义（create/add 用）"},
				"stage":       map[string]any{"type": "object", "description": "单个阶段定义（add 用）"},
				"stage_id":    map[string]any{"type": "string", "description": "目标阶段 id（confirm/rollback/remove 用）"},
				"reason":      map[string]any{"type": "string", "description": "放弃原因（abandon 必需）"},
				"autoConfirm": map[string]any{"type": "boolean", "description": "run 结束自动确认成功阶段"},
			},
			"required": []string{"action", "case_id"},
		},
		Func: func(_ context.Context, args json.RawMessage) (any, error) {
			return handleFlexiblePlan(store, args)
		},
	}
}

func handleFlexiblePlan(store *FlexiblePlanStore, args json.RawMessage) (any, error) {
	var p FlexiblePlanInput
	if err := json.Unmarshal(args, &p); err != nil {
		return agentcore.NewFailureResult("参数解析失败", "灵活计划参数格式错误"), nil
	}
	if p.CaseID == "" {
		return agentcore.NewFailureResult("缺少 case_id", "case_id 不能为空"), nil
	}
	if !isSafeCaseID(p.CaseID) {
		return agentcore.NewFailureResult("非法 case_id", "case_id 仅允许不含路径分隔符的标识符"), nil
	}

	switch p.Action {
	case "create":
		return handleCreatePlan(store, p)
	case "get":
		return handleGetPlan(store, p)
	case "run":
		return handleRunPlan(store, p)
	case "confirm":
		return mutateStage(store, p.CaseID, p.StageID, "confirmed", "已确认阶段")
	case "rollback":
		return mutateStage(store, p.CaseID, p.StageID, "rolled_back", "已回退阶段")
	case "add":
		return handleAddPlan(store, p)
	case "remove":
		return handleRemovePlan(store, p)
	case "complete":
		return handleCompletePlan(store, p)
	case "abandon":
		return handleAbandonPlan(store, p)
	default:
		return fmt.Sprintf("patent_flexible_plan: 未知操作 %q", p.Action), nil
	}
}

func handleCreatePlan(store *FlexiblePlanStore, p FlexiblePlanInput) (any, error) {
	if p.CaseType == "" {
		return agentcore.NewFailureResult("缺少 case_type", "create 需要 case_type"), nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	plan := &FlexiblePlan{
		CaseID:    p.CaseID,
		CaseType:  p.CaseType,
		InputText: p.InputText,
		Status:    "active",
		Stages:    normalizeStages(p.Stages),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := store.Save(plan); err != nil {
		return agentcore.NewFailureResult("保存失败", err.Error()), nil
	}
	return renderFlexiblePlan(plan, "已创建并持久化"), nil
}

func handleGetPlan(store *FlexiblePlanStore, p FlexiblePlanInput) (any, error) {
	plan, err := store.Load(p.CaseID)
	if err != nil {
		return agentcore.NewFailureResult("加载失败", err.Error()), nil
	}
	return renderFlexiblePlan(plan, ""), nil
}

func handleRunPlan(store *FlexiblePlanStore, p FlexiblePlanInput) (any, error) {
	plan, err := store.Load(p.CaseID)
	if err != nil {
		return agentcore.NewFailureResult("加载失败", err.Error()), nil
	}
	if p.InputText != "" {
		plan.InputText = p.InputText
	}
	pendingCount := 0
	for i, s := range plan.Stages {
		if s.Status == "pending" || s.Status == "rolled_back" {
			pendingCount++
			plan.Stages[i].Status = "running"
			if plan.CurrentStage == "" {
				plan.CurrentStage = s.ID
			}
		}
	}
	if pendingCount == 0 {
		return renderFlexiblePlan(plan, "无待执行阶段"), nil
	}
	if p.AutoConfirm {
		for i := range plan.Stages {
			if plan.Stages[i].Status == "running" {
				plan.Stages[i].Status = "confirmed"
			}
		}
	}
	plan.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := store.Save(plan); err != nil {
		return agentcore.NewFailureResult("保存失败", err.Error()), nil
	}
	return renderFlexiblePlan(plan, fmt.Sprintf("执行 %d 个未确认阶段", pendingCount)), nil
}

func handleAddPlan(store *FlexiblePlanStore, p FlexiblePlanInput) (any, error) {
	if p.Stage == nil {
		return agentcore.NewFailureResult("缺少 stage", "add 需要 stage"), nil
	}
	plan, err := store.Load(p.CaseID)
	if err != nil {
		return agentcore.NewFailureResult("加载失败", err.Error()), nil
	}
	stage := *p.Stage
	if stage.Status == "" {
		stage.Status = "pending"
	}
	plan.Stages = append(plan.Stages, stage)
	plan.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := store.Save(plan); err != nil {
		return agentcore.NewFailureResult("保存失败", err.Error()), nil
	}
	return renderFlexiblePlan(plan, fmt.Sprintf("已追加阶段 %s", stage.ID)), nil
}

func handleRemovePlan(store *FlexiblePlanStore, p FlexiblePlanInput) (any, error) {
	if p.StageID == "" {
		return agentcore.NewFailureResult("缺少 stage_id", "remove 需要 stage_id"), nil
	}
	plan, err := store.Load(p.CaseID)
	if err != nil {
		return agentcore.NewFailureResult("加载失败", err.Error()), nil
	}
	filtered := make([]FlexiblePlanStage, 0, len(plan.Stages))
	for _, s := range plan.Stages {
		if s.ID != p.StageID {
			filtered = append(filtered, s)
		}
	}
	plan.Stages = filtered
	plan.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := store.Save(plan); err != nil {
		return agentcore.NewFailureResult("保存失败", err.Error()), nil
	}
	return renderFlexiblePlan(plan, fmt.Sprintf("已删除阶段 %s", p.StageID)), nil
}

func handleCompletePlan(store *FlexiblePlanStore, p FlexiblePlanInput) (any, error) {
	plan, err := store.Load(p.CaseID)
	if err != nil {
		return agentcore.NewFailureResult("加载失败", err.Error()), nil
	}
	plan.Status = "completed"
	plan.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := store.Save(plan); err != nil {
		return agentcore.NewFailureResult("保存失败", err.Error()), nil
	}
	return renderFlexiblePlan(plan, "计划已完成"), nil
}

func handleAbandonPlan(store *FlexiblePlanStore, p FlexiblePlanInput) (any, error) {
	if strings.TrimSpace(p.Reason) == "" {
		return agentcore.NewFailureResult("缺少 reason", "abandon 需要 reason（审计留痕）"), nil
	}
	plan, err := store.Load(p.CaseID)
	if err != nil {
		return agentcore.NewFailureResult("加载失败", err.Error()), nil
	}
	plan.Status = "abandoned"
	plan.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := store.Save(plan); err != nil {
		return agentcore.NewFailureResult("保存失败", err.Error()), nil
	}
	return renderFlexiblePlan(plan, fmt.Sprintf("计划已放弃（原因：%s）", p.Reason)), nil
}

func mutateStage(store *FlexiblePlanStore, caseID, stageID, status, msg string) (any, error) {
	if stageID == "" {
		return agentcore.NewFailureResult("缺少 stage_id", "需要 stage_id"), nil
	}
	plan, err := store.Load(caseID)
	if err != nil {
		return agentcore.NewFailureResult("加载失败", err.Error()), nil
	}
	found := false
	for i := range plan.Stages {
		if plan.Stages[i].ID == stageID {
			plan.Stages[i].Status = status
			found = true
			break
		}
	}
	if !found {
		return agentcore.NewFailureResult("阶段不存在", fmt.Sprintf("阶段 %q 不存在", stageID)), nil
	}
	plan.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := store.Save(plan); err != nil {
		return agentcore.NewFailureResult("保存失败", err.Error()), nil
	}
	return renderFlexiblePlan(plan, fmt.Sprintf("%s %s", msg, stageID)), nil
}

func normalizeStages(stages []FlexiblePlanStage) []FlexiblePlanStage {
	for i := range stages {
		if stages[i].Status == "" {
			stages[i].Status = "pending"
		}
	}
	return stages
}

func renderFlexiblePlan(plan *FlexiblePlan, note string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "flexible_plan(caseId=%s, caseType=%s, status=%s)\n", plan.CaseID, plan.CaseType, plan.Status)
	fmt.Fprintf(&b, "当前阶段: %s\n", plan.CurrentStage)
	for _, s := range plan.Stages {
		flag := "⏳"
		switch s.Status {
		case "confirmed":
			flag = "✅"
		case "rolled_back":
			flag = "↩️"
		case "running":
			flag = "🏃"
		}
		fmt.Fprintf(&b, "- %s %s (%s): %s\n", flag, s.ID, s.Strategy, s.Goal)
	}
	if note != "" {
		fmt.Fprintf(&b, "%s\n", note)
	}
	return b.String()
}
