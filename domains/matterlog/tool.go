package matterlog

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/xujian519/mady/agentcore"
)

// ToolName 是事项日志追加工具的注册名。
const ToolName = "matter_log_append"

const toolDescription = `把本轮作业写进案卷的只追加事件日志 ` + LogFileName + `（案件的事实源），并登记取代关系。

- 每次撰写、修订、交付、门禁都在这里留档：changes 写清改了什么、为什么、是否影响保护点；artifacts 列出本轮产物（须已存在）
- 取代关系显式登记：supersedes 列出本轮产物取代的既有文件；交付物一律另存新文件，禁止覆盖上一轮
- 确定性拒绝：产物不存在、产物路径与取代路径相同（原地覆盖）、supersedes 指向日志中没有的路径、同一旧版被取代两次（取代链分叉）、撰写/修订轮复用已登记的产物路径
- 只追加，既有行不改写；每条同时记本地时间与 UTC
- 动作闭集：建案/检索/分析/撰写/门禁/修订/交付/归档；日志不存在时只允许「建案」初始化`

// ToolConfig 配置事项日志工具。
type ToolConfig struct {
	// WorkingDir 是 case_dir 的沙箱边界（项目目录，缺省工作区）。
	WorkingDir string
}

// ToolResult 是工具输出。
type ToolResult struct {
	AppendResult
	Rendered string `json:"rendered"`
}

type toolArgs struct {
	CaseDir    string   `json:"case_dir"`
	Action     string   `json:"action"`
	Artifacts  []string `json:"artifacts"`
	Changes    string   `json:"changes"`
	Supersedes []string `json:"supersedes"`
	Approver   string   `json:"approver"`
	Note       string   `json:"note"`
}

// NewTool 创建 matter_log_append 工具。
func NewTool(cfg ToolConfig) *agentcore.Tool {
	t := &logTool{cfg: cfg}
	return &agentcore.Tool{
		Name:        ToolName,
		Description: toolDescription,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"case_dir": map[string]any{
					"type":        "string",
					"description": "案件目录（内含 " + LogFileName + "；相对工作目录或工作目录内的绝对路径）",
				},
				"action": map[string]any{
					"type": "string", "enum": Actions, "description": "本轮动作",
				},
				"artifacts": map[string]any{
					"type": "array", "items": map[string]any{"type": "string"},
					"description": "本轮产物文件路径（须已存在）",
				},
				"changes": map[string]any{
					"type": "string", "description": "本轮改了什么、为什么、是否影响保护点（留档段正文）",
				},
				"supersedes": map[string]any{
					"type": "array", "items": map[string]any{"type": "string"},
					"description": "被本轮产物取代的既有产物路径（可选）",
				},
				"approver": map[string]any{"type": "string", "description": "审批人（可选）"},
				"note":     map[string]any{"type": "string", "description": "附加备注（可选）"},
			},
			"required":             []string{"case_dir", "action", "artifacts", "changes"},
			"additionalProperties": false,
		},
		Func: t.Run,
	}
}

type logTool struct{ cfg ToolConfig }

func (t *logTool) Run(_ context.Context, args json.RawMessage) (any, error) {
	var input toolArgs
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, fmt.Errorf("参数无效: %w", err)
	}
	if input.CaseDir == "" {
		return nil, fmt.Errorf("缺少必填字段 'case_dir'")
	}
	if !ValidAction(input.Action) {
		return nil, fmt.Errorf("动作 %q 不在闭集内（可选：%s）", input.Action, strings.Join(Actions, "/"))
	}
	caseDir, err := t.resolveCaseDir(input.CaseDir)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(caseDir)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("案件目录不可用：%s", input.CaseDir)
	}
	logPath := filepath.Join(caseDir, LogFileName)
	existing, readErr := os.ReadFile(logPath) //nolint:gosec // 路径已限定在工作目录内
	if readErr != nil && !os.IsNotExist(readErr) {
		return nil, fmt.Errorf("读取事项日志失败：%w", readErr)
	}
	if os.IsNotExist(readErr) && input.Action != "建案" {
		return nil, fmt.Errorf("%s 不存在：案件先经「建案」动作初始化日志，后续动作才可追加", logPath)
	}

	request := Request{
		Action:     input.Action,
		Artifacts:  t.toCasePaths(caseDir, input.Artifacts),
		Changes:    input.Changes,
		Supersedes: t.toCasePaths(caseDir, input.Supersedes),
		Approver:   input.Approver,
		Note:       input.Note,
	}
	defects := Defects(string(existing), request, func(path string) bool {
		_, statErr := os.Stat(filepath.Join(caseDir, filepath.FromSlash(path)))
		return statErr == nil
	})
	if len(defects) > 0 {
		var lines []string
		for _, defect := range defects {
			lines = append(lines, "- ["+defect.Rule+"] "+defect.Message)
		}
		return nil, fmt.Errorf("留档被拒（条目未写入）：\n%s", strings.Join(lines, "\n"))
	}

	// 并发前提：同一案件目录按单写者顺序调用（与上游一致）。O_APPEND 只保证
	// 单次写入不丢行，不保证「读日志—校验—追加」这段是原子的；多写者并发时
	// 取代链校验可能读到同一份旧日志。需要并发写入时，应在调用方串行化或加锁。
	entry := RenderEntry(request, time.Now())
	header := ""
	if len(existing) > 0 && !strings.HasSuffix(string(existing), "\n") {
		header = "\n"
	}
	file, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644) //nolint:gosec
	if err != nil {
		return nil, fmt.Errorf("打开事项日志失败：%w", err)
	}
	if _, err := file.WriteString(header + entry + "\n"); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("追加事项日志失败：%w", err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("关闭事项日志失败：%w", err)
	}

	// 输出按调用方给出的 case_dir 拼回，保持「相对进、相对出」的一致写法。
	result := AppendResult{
		LogPath: filepath.Join(filepath.Clean(input.CaseDir), LogFileName), Entry: entry,
		PriorEntries: len(ParseEntries(string(existing))),
		Superseded:   request.Supersedes,
	}
	return ToolResult{AppendResult: result, Rendered: renderAppend(result)}, nil
}

// resolveCaseDir 把案件目录限制在工作目录内（含符号链接复核）。
func (t *logTool) resolveCaseDir(caseDir string) (string, error) {
	if t.cfg.WorkingDir == "" {
		return "", fmt.Errorf("未配置工作目录，无法写入事项日志")
	}
	absWork, err := filepath.Abs(t.cfg.WorkingDir)
	if err != nil {
		return "", fmt.Errorf("解析工作目录失败：%w", err)
	}
	resolved := caseDir
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(absWork, resolved)
	}
	resolved = filepath.Clean(resolved)
	rel, err := filepath.Rel(absWork, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("case_dir 必须位于工作目录 %s 内：%s", absWork, caseDir)
	}
	resolvedWork, err := filepath.EvalSymlinks(absWork)
	if err != nil {
		return "", fmt.Errorf("解析工作目录符号链接失败：%w", err)
	}
	resolvedDir, err := filepath.EvalSymlinks(resolved)
	if err != nil {
		return "", fmt.Errorf("解析案件目录失败：%w", err)
	}
	rel, err = filepath.Rel(resolvedWork, resolvedDir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("案件目录经符号链接后越出工作目录：%s", resolvedDir)
	}
	return resolvedDir, nil
}

// toCasePaths 把案件目录内的绝对路径归一为案件相对路径（取代链按路径逐个比对，
// 同一文件必须只有一种写法）。
func (t *logTool) toCasePaths(caseDir string, paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		trimmed := strings.TrimSpace(path)
		if trimmed == "" {
			continue
		}
		if filepath.IsAbs(trimmed) {
			if rel, err := filepath.Rel(caseDir, trimmed); err == nil &&
				rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				trimmed = rel
			}
		}
		out = append(out, filepath.ToSlash(trimmed))
	}
	return out
}

func renderAppend(result AppendResult) string {
	lines := []string{
		"已留档：" + result.Entry,
		fmt.Sprintf("日志 %s，本条之前已有 %d 条结构化条目。", result.LogPath, result.PriorEntries),
	}
	if len(result.Superseded) > 0 {
		lines = append(lines, "取代链："+strings.Join(result.Superseded, "、")+" → 本轮产物。")
	}
	lines = append(lines, "日志只追加：发现记录错误时追加更正行，不得改写既有行。")
	return strings.Join(lines, "\n")
}
