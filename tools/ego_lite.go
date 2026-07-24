// ego_lite.go — EgoLite Extension: handoff + task_spaces 工具
package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/xujian519/mady/agentcore"
)

const (
	EgoLiteHandoffToolName    = "ego_lite_handoff"
	EgoLiteTaskSpacesToolName = "ego_lite_task_spaces"
)

// EgoLiteConfig 配置 EgoLite Extension。
type EgoLiteConfig struct {
	Enabled  bool
	TaskName string
	Headless bool
}

// EgoLiteExtension 实现 agentcore.Extension，注册 handoff 和 task_spaces 工具。
type EgoLiteExtension struct {
	mgr *EgoLiteManager
	cfg EgoLiteConfig
}

// Compile-time check: EgoLiteExtension satisfies agentcore.Extension.
var _ agentcore.Extension = (*EgoLiteExtension)(nil)

// NewEgoLiteExtension 创建 EgoLite Extension。
func NewEgoLiteExtension(cfg EgoLiteConfig) (*EgoLiteExtension, error) {
	if !cfg.Enabled {
		return nil, fmt.Errorf("egolite: extension disabled (set EgoLiteEnabled=true)")
	}
	mgr, err := NewEgoLiteManager(cfg.TaskName)
	if err != nil {
		return nil, fmt.Errorf("egolite: create manager: %w", err)
	}
	return &EgoLiteExtension{mgr: mgr, cfg: cfg}, nil
}

func (e *EgoLiteExtension) Name() string { return "ego-lite" }
func (e *EgoLiteExtension) Init(_ context.Context, _ *agentcore.Agent) error {
	if e.mgr == nil {
		return fmt.Errorf("egolite: manager not initialized")
	}
	return nil
}
func (e *EgoLiteExtension) Dispose() error           { return e.mgr.Close() }
func (e *EgoLiteExtension) Manager() *EgoLiteManager { return e.mgr }

func (e *EgoLiteExtension) Tools() []*agentcore.Tool {
	return []*agentcore.Tool{
		newEgoLiteHandoffTool(e.mgr),
		newEgoLiteTaskSpacesTool(e.mgr),
	}
}

// ==========================================
// ego_lite_handoff 工具
// ==========================================

func newEgoLiteHandoffTool(mgr *EgoLiteManager) *agentcore.Tool {
	return &agentcore.Tool{
		Name: EgoLiteHandoffToolName,
		Description: `浏览器控制权交接——当需要人工介入时（登录、验证码、表单确认等），将 Ego Lite 浏览器控制权交给用户。
操作：
  handoff — 交出控制权给用户（用户可手动操作浏览器）
  takeover — 取回控制权，自动获取当前页面快照
  status — 查看当前控制权状态`,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{
					"type":        "string",
					"description": "交接操作：handoff=交给用户, takeover=取回控制权, status=查询状态",
					"enum":        []any{"handoff", "takeover", "status"},
				},
				"message": map[string]any{
					"type":        "string",
					"description": "handoff 时向用户展示的操作说明（如'请登录后告诉我继续'）",
				},
			},
			"required": []any{"action"},
		},
		ReadOnly: false,
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Action  string `json:"action"`
				Message string `json:"message"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, fmt.Errorf("egolite handoff: invalid args: %w", err)
			}

			switch in.Action {
			case "handoff":
				result, err := mgr.Send(ctx, "handoffTaskSpace", nil)
				if err != nil {
					return nil, fmt.Errorf("handoff failed: %w", err)
				}
				output := "浏览器控制权已交出。"
				if m, ok := result.(map[string]any); ok {
					if done, _ := m["done"].(bool); !done {
						if skipped, ok := m["skipped"].(string); ok {
							output = fmt.Sprintf("控制权交接被跳过：%s", skipped)
						}
					}
				}
				if in.Message != "" {
					output += "\n\n用户操作说明：" + in.Message
				}
				return output, nil

			case "takeover":
				_, err := mgr.Send(ctx, "takeOverTaskSpace", nil)
				if err != nil {
					return nil, fmt.Errorf("takeover failed: %w", err)
				}
				snapshot, snapErr := mgr.Send(ctx, "snapshotText", nil)
				pageInfo, _ := mgr.Send(ctx, "pageInfo", nil)
				info := ""
				if pi, ok := pageInfo.(map[string]any); ok {
					if u, ok := pi["url"].(string); ok {
						info = fmt.Sprintf("当前页面: %s\n", u)
					}
				}
				if snapErr != nil {
					return fmt.Sprintf("已取回浏览器控制权。\n%s\n（无法获取页面快照：%v）", info, snapErr), nil
				}
				snapStr, _ := snapshot.(string)
				return fmt.Sprintf("已取回浏览器控制权。\n%s\n页面快照:\n%s", info, snapStr), nil

			case "status":
				result, err := mgr.Send(ctx, "listTaskSpaces", nil)
				if err != nil {
					return nil, fmt.Errorf("status query failed: %w", err)
				}
				spaces, _ := result.([]any)
				if len(spaces) == 0 {
					return "当前没有活动任务空间。", nil
				}
				var output string
				for _, s := range spaces {
					if m, ok := s.(map[string]any); ok {
						name, _ := m["name"].(string)
						owner, _ := m["ownership"].(string)
						output += fmt.Sprintf("· %s (ownership: %s)\n", name, owner)
					}
				}
				return output, nil

			default:
				return nil, fmt.Errorf("egolite handoff: unknown action %q (valid: handoff, takeover, status)", in.Action)
			}
		},
	}
}

// ==========================================
// ego_lite_task_spaces 工具
// ==========================================

func newEgoLiteTaskSpacesTool(mgr *EgoLiteManager) *agentcore.Tool {
	return &agentcore.Tool{
		Name: EgoLiteTaskSpacesToolName,
		Description: `管理 Ego Lite 浏览器任务空间——每个任务空间有独立的标签页集合和浏览上下文，但共享浏览器登录态。用于并行处理多个独立浏览任务。
操作：
  list — 列出所有任务空间
  create — 创建或恢复指定名称的任务空间
  switch — 切换到指定任务空间
  close — 关闭任务空间`,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{
					"type":        "string",
					"description": "操作：list=列出所有空间, create=创建或恢复, switch=切换, close=关闭",
					"enum":        []any{"list", "create", "switch", "close"},
				},
				"name": map[string]any{
					"type":        "string",
					"description": "任务空间名称（create/switch 时使用）",
				},
				"keep": map[string]any{
					"type":        "boolean",
					"description": "关闭时是否保留标签页给用户查看（默认 false）",
				},
			},
			"required": []any{"action"},
		},
		ReadOnly: false,
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Action string `json:"action"`
				Name   string `json:"name"`
				Keep   bool   `json:"keep"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, fmt.Errorf("egolite task_spaces: invalid args: %w", err)
			}

			switch in.Action {
			case "list":
				result, err := mgr.Send(ctx, "listTaskSpaces", nil)
				if err != nil {
					return nil, fmt.Errorf("list task spaces: %w", err)
				}
				spaces, _ := result.([]any)
				if len(spaces) == 0 {
					return "当前没有活动任务空间。", nil
				}
				var output string
				for _, s := range spaces {
					m, _ := s.(map[string]any)
					name, _ := m["name"].(string)
					owner, _ := m["ownership"].(string)
					output += fmt.Sprintf("· %s (ownership: %s)\n", name, owner)
				}
				return output, nil

			case "create":
				if in.Name == "" {
					return nil, fmt.Errorf("name is required for create action")
				}
				result, err := mgr.Send(ctx, "initTaskSpace", map[string]any{"name": in.Name})
				if err != nil {
					return nil, fmt.Errorf("create task space: %w", err)
				}
				m, _ := result.(map[string]any)
				id, _ := m["id"].(string)
				return fmt.Sprintf("任务空间已创建: name=%s, id=%s", in.Name, id), nil

			case "switch":
				if in.Name == "" {
					return nil, fmt.Errorf("name is required for switch action")
				}
				result, err := mgr.Send(ctx, "initTaskSpace", map[string]any{"name": in.Name})
				if err != nil {
					return nil, fmt.Errorf("switch task space: %w", err)
				}
				m, _ := result.(map[string]any)
				id, _ := m["id"].(string)
				return fmt.Sprintf("已切换到任务空间: name=%s, id=%s", in.Name, id), nil

			case "close":
				result, err := mgr.Send(ctx, "completeTaskSpace", map[string]any{"keep": in.Keep})
				if err != nil {
					return nil, fmt.Errorf("close task space: %w", err)
				}
				m, _ := result.(map[string]any)
				if done, _ := m["done"].(bool); !done {
					if skipped, ok := m["skipped"].(string); ok {
						return fmt.Sprintf("关闭被跳过：%s", skipped), nil
					}
				}
				return "任务空间已关闭。", nil

			default:
				return nil, fmt.Errorf("egolite task_spaces: unknown action %q (valid: list, create, switch, close)", in.Action)
			}
		},
	}
}
