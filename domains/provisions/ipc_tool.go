package provisions

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
)

// NewResolveDomainWorkersTool 创建 resolve_domain_workers 工具。
// 该工具根据传入的 IPC 提示列表，解析出可用的 domain-* 领域专家 worker 名称。
// 注册到 PatentAgent 后，编排器和条款智能体可用它来发现可用的 IPC 领域专家。
func NewResolveDomainWorkersTool(mapPath string) *agentcore.Tool {
	return &agentcore.Tool{
		Name:        "resolve_domain_workers",
		Description: "根据 IPC 分类号提示列表，返回可用的领域专家列表。IPC 提示由 technical-analyzer 输出或用户指定。示例：ipc_hints=[\"A61\",\"G06\"] → [\"domain-A61-novelty\", \"domain-G06-inventiveness\", ...]",
		ReadOnly:    true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"ipc_hints": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "IPC 分类号提示列表，如 [\"A61\", \"G06\", \"H04\"]",
				},
			},
			"required": []string{"ipc_hints"},
		},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			var p struct {
				IpcHints []string `json:"ipc_hints"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return fmt.Sprintf("参数解析错误: %v", err), nil
			}
			if len(p.IpcHints) == 0 {
				return "请提供至少一个 IPC 分类号提示。", nil
			}

			names := ListDomainWorkerNames(p.IpcHints, mapPath)
			if len(names) == 0 {
				return fmt.Sprintf("未找到 IPC %s 对应的领域专家。已知 IPC 段：A61/G06/H04/C07/C12/G01/B60/F16/H01/E04", strings.Join(p.IpcHints, ", ")), nil
			}

			var b strings.Builder
			fmt.Fprintf(&b, "IPC %s 对应的领域专家（%d 个）：\n\n", strings.Join(p.IpcHints, ", "), len(names))
			for _, name := range names {
				fmt.Fprintf(&b, "- transfer_to_%s\n", name)
			}
			b.WriteString("\n使用 transfer_to_domain-* 工具将领域特定问题委派给对应的 IPC 领域专家。")
			return b.String(), nil
		},
	}
}
