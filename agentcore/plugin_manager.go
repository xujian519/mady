package agentcore

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// PluginManager manages the lifecycle and execution of file-based plugins.
// It discovers plugins via ScanPlugins, provides a run_plugin tool for agents,
// and delegates stage execution to the PipelineExecutor.
type PluginManager struct {
	provider  Provider
	retriever Retriever
	plugins   []PluginManifest
	executor  *PipelineExecutor
}

// NewPluginManager creates a PluginManager that discovers plugins from the
// given directories and uses provider for LLM-based stages.
func NewPluginManager(provider Provider, retriever Retriever, pluginDirs ...string) (*PluginManager, error) {
	RegisterBuiltinStageHandlers()

	plugins, err := ScanPlugins(pluginDirs...)
	if err != nil {
		return nil, fmt.Errorf("plugin: scan: %w", err)
	}

	pm := &PluginManager{
		provider:  provider,
		retriever: retriever,
		plugins:   plugins,
		executor:  NewPipelineExecutor(provider),
	}
	return pm, nil
}

// Plugins returns the list of discovered PluginManifests.
func (pm *PluginManager) Plugins() []PluginManifest { return pm.plugins }

// RunPlugin executes a plugin by name with the given input state.
func (pm *PluginManager) RunPlugin(ctx context.Context, name string, input PipelineState) (PipelineState, error) {
	for i := range pm.plugins {
		if pm.plugins[i].Name == name {
			// Inject retriever into state for search stages.
			if pm.retriever != nil {
				input["_retriever"] = pm.retriever
			}
			return pm.executor.Run(ctx, &pm.plugins[i], input)
		}
	}
	return nil, fmt.Errorf("plugin: %q not found (scanned %d plugins)", name, len(pm.plugins))
}

// RunPluginTool returns an agentcore.Tool that allows the Agent to run
// any discovered plugin by name.
//
// The tool accepts:
//   - plugin_name (string, required): name of the plugin to run
//   - input (object, optional): key-value input for the plugin's first stage
//
// The tool returns:
//   - status: "completed" or "interrupted" or "error"
//   - output: the final PipelineState as JSON
//   - interrupted_at: the stage ID where the pipeline was interrupted (if any)
func (pm *PluginManager) RunPluginTool() *Tool {
	// Build descriptions for each registered plugin.
	pluginDescriptions := make([]map[string]any, 0, len(pm.plugins))
	for _, p := range pm.plugins {
		stages := make([]string, 0, len(p.Pipeline.Stages))
		for _, s := range p.Pipeline.Stages {
			stages = append(stages, s.ID)
		}
		pluginDescriptions = append(pluginDescriptions, map[string]any{
			"name":        p.Name,
			"description": p.Description,
			"domain":      p.Domain,
			"stages":      stages,
		})
	}

	return &Tool{
		Name: "run_plugin",
		Description: fmt.Sprintf(
			"运行指定的插件工作流。插件工作流由多个阶段（stage）组成，按顺序执行。"+
				"可用插件：%d 个。详情见 plugin_descriptions。",
			len(pm.plugins),
		),
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"plugin_name": map[string]any{
					"type":        "string",
					"description": "要运行的插件名称。" + buildPluginNameDescription(pm.plugins),
				},
				"input": map[string]any{
					"type":        "object",
					"description": "插件的输入参数（key-value 对）。典型键包括：text（待分析的文本）、query（检索查询）、claim（权利要求文本）等。",
					"properties": map[string]any{
						"text":   map[string]any{"type": "string", "description": "待分析的文本内容"},
						"query":  map[string]any{"type": "string", "description": "检索查询"},
						"claim":  map[string]any{"type": "string", "description": "权利要求/技术方案"},
						"domain": map[string]any{"type": "string", "description": "领域（patent/legal）"},
					},
				},
				"plugin_descriptions": map[string]any{
					"type":        "array",
					"description": "可用插件列表及其阶段描述（供参考，不可修改）",
					"items": map[string]any{
						"type":        "object",
						"description": "插件信息",
						"properties": map[string]any{
							"name":        map[string]any{"type": "string"},
							"description": map[string]any{"type": "string"},
							"domain":      map[string]any{"type": "string"},
							"stages":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						},
					},
				},
			},
			"required": []any{"plugin_name"},
		},
		DynamicParameters: func() map[string]any {
			return map[string]any{
				"plugin_descriptions": map[string]any{
					"type":        "array",
					"description": "可用插件列表",
					"static":      pluginDescriptions,
				},
			}
		},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			var params struct {
				PluginName string         `json:"plugin_name"`
				Input      map[string]any `json:"input,omitempty"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return map[string]any{
					"status": "error",
					"error":  fmt.Sprintf("参数解析失败: %v", err),
				}, nil
			}

			if params.PluginName == "" {
				return map[string]any{
					"status": "error",
					"error":  "plugin_name 不能为空",
				}, nil
			}

			input := PipelineState(params.Input)
			state, err := pm.RunPlugin(ctx, params.PluginName, input)
			if err != nil {
				if IsInterruptStage(err) {
					return map[string]any{
						"status":         "interrupted",
						"interrupted_at": state.GetString("_interrupted_at"),
						"message":        err.Error(),
						"output":         state,
					}, nil
				}
				return map[string]any{
					"status": "error",
					"error":  err.Error(),
				}, nil
			}

			// Clean up internal keys before returning to agent.
			cleanState := make(map[string]any)
			for k, v := range state {
				if k == "_retriever" || k == "_warnings" || k == "_executed_stages" {
					continue
				}
				cleanState[k] = v
			}

			return map[string]any{
				"status":          "completed",
				"output":          cleanState,
				"executed_stages": state["_executed_stages"],
			}, nil
		},
	}
}

func buildPluginNameDescription(plugins []PluginManifest) string {
	if len(plugins) == 0 {
		return "暂无可用插件。"
	}
	var sb strings.Builder
	sb.WriteString("可选值：")
	for i, p := range plugins {
		if i > 0 {
			sb.WriteString("、")
		}
		sb.WriteString(p.Name)
	}
	sb.WriteString("。")
	return sb.String()
}
