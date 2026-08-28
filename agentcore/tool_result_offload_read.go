// tool_result_offload_read.go 实现 offload_read 工具：按句柄回读已落盘的
// 工具结果全文。它是 ToolResultBudget 摘要的配套回读通道——预算摘要只保留
// 头尾片段，模型判断需要完整内容时通过本工具取回。
//
// 安全边界：只允许读取 rootDir 下的文件（路径穿越与目录外绝对路径一律
// 拒绝）；文件大小设防御上限，避免回读本身再次撑爆上下文。走独立工具而
// 非通用文件读取，是为了不依赖 WorkingDir 沙箱配置——offload 目录在
// $MADY_HOME 下，通常不在案件工作区内。

package agentcore

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// offloadReadMaxBytes 是单次回读的字节上限（防御性——正常落盘内容已被
// 工具层 50KB 截断约束，上限仅兜底异常文件）。
const offloadReadMaxBytes = 256 * 1024

// NewOffloadReadTool 创建绑定指定 offload 根目录的回读工具。
// rootDir 必须与对应 ToolResultBudget 的 RootDir 一致。
func NewOffloadReadTool(rootDir string) *Tool {
	return &Tool{
		Name: "offload_read",
		Description: "回读已落盘的工具结果全文。当工具结果摘要标注了 handle（完整结果已落盘）" +
			"且需要中段完整内容时使用；handle 取自摘要中的 handle 字段。只能读取落盘目录内的内容。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"handle": map[string]any{
					"type":        "string",
					"description": "落盘句柄（工具结果摘要中的 handle 字段）",
				},
			},
			"required": []string{"handle"},
		},
		ReadOnly: true,
		Func:     makeOffloadReadFunc(rootDir),
	}
}

func makeOffloadReadFunc(rootDir string) ToolFunc {
	return func(_ context.Context, args json.RawMessage) (any, error) {
		var p struct {
			Handle string `json:"handle"`
		}
		if err := json.Unmarshal(args, &p); err != nil {
			return NewFailureResult("参数解析失败", "offload_read 参数格式错误"), nil
		}
		if p.Handle == "" {
			return NewFailureResult("缺少参数", "handle 不能为空"), nil
		}

		path, err := resolveOffloadPath(rootDir, p.Handle)
		if err != nil {
			return NewFailureResult("句柄无效", err.Error()), nil
		}
		data, err := os.ReadFile(path) //nolint:gosec // 路径已经 resolveOffloadPath 校验在 rootDir 内
		if err != nil {
			return NewFailureResult("回读失败", "落盘内容不存在或已清理: "+p.Handle), nil
		}
		if len(data) > offloadReadMaxBytes {
			data = data[:offloadReadMaxBytes]
		}
		return string(data), nil
	}
}

// resolveOffloadPath 校验句柄并解析为 rootDir 内的绝对路径。
// 拒绝：目录外绝对路径、含 .. 的穿越路径、根目录本身。
func resolveOffloadPath(rootDir, handle string) (string, error) {
	if rootDir == "" {
		return "", fmt.Errorf("offload 根目录未配置")
	}
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return "", fmt.Errorf("解析根目录失败: %w", err)
	}
	abs, err := filepath.Abs(handle)
	if err != nil {
		return "", fmt.Errorf("解析句柄失败: %w", err)
	}
	abs = filepath.Clean(abs)
	if abs == absRoot {
		return "", fmt.Errorf("句柄不能指向落盘根目录本身")
	}
	rel, err := filepath.Rel(absRoot, abs)
	if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return "", fmt.Errorf("句柄指向落盘目录之外，已拒绝")
	}
	return abs, nil
}
