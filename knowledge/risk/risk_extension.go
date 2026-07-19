package risk

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/knowledge"
)

// ExtensionName is the unique identifier for the risk extension.
const ExtensionName = "risk"

// Extension adapts the risk scanner as an agentcore.Extension.
// It provides the risk_scan tool and injects risk-related system prompt hints.
type Extension struct {
	scanner *Scanner
	store   *knowledge.Store
}

// NewExtension creates a risk extension with a scanner backed by the store.
func NewExtension(store *knowledge.Store, config ScannerConfig) *Extension {
	searcher := NewStoreCaseSearcher(store)
	scanner := NewScanner(searcher, config)
	return &Extension{scanner: scanner, store: store}
}

// Interface assertions.
var (
	_ agentcore.Extension            = (*Extension)(nil)
	_ agentcore.ToolProvider         = (*Extension)(nil)
	_ agentcore.SystemPromptProvider = (*Extension)(nil)
)

func (e *Extension) Name() string                                     { return ExtensionName }
func (e *Extension) Init(_ context.Context, _ *agentcore.Agent) error { return nil }
func (e *Extension) Dispose() error                                   { return nil }

// SystemPromptSuffix injects a hint about available risk tools.
func (e *Extension) SystemPromptSuffix() string {
	return fmt.Sprintf(
		"\n## 风险扫描工具\n" +
			"使用 risk_scan 工具对技术特征组合进行历史无效宣告风险扫描。\n" +
			"输入特征标签（如 \"功能性限定\", \"参数限定\"），系统会检索历史案例并返回风险评分。\n")
}

// Tools registers the risk_scan tool.
func (e *Extension) Tools() []*agentcore.Tool {
	return []*agentcore.Tool{
		{
			Name:        "risk_scan",
			Description: "对技术特征组合进行历史无效宣告风险扫描。输入特征标签列表，返回风险信号（含严重度、历史案例数、无效率）。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"features": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "技术特征标签列表，如 [\"功能性限定\", \"参数限定\"]",
					},
				},
				"required": []string{"features"},
			},
			Func: e.handleScan,
		},
	}
}

// handleScan implements the risk_scan tool function.
func (e *Extension) handleScan(ctx context.Context, args json.RawMessage) (any, error) {
	var params struct {
		Features []string `json:"features"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return fmt.Errorf("risk_scan: invalid parameters: %w", err), nil
	}
	if len(params.Features) == 0 {
		return "请提供至少一个技术特征标签。", nil
	}
	// Clean and normalize features.
	cleaned := make([]string, 0, len(params.Features))
	for _, f := range params.Features {
		f = strings.TrimSpace(f)
		if f != "" {
			cleaned = append(cleaned, f)
		}
	}
	if len(cleaned) == 0 {
		return "特征标签为空，请提供有效的技术特征。", nil
	}

	result, err := e.scanner.ScanByFeatures(ctx, cleaned)
	if err != nil {
		return fmt.Errorf("risk_scan: scan failed: %w", err), nil
	}
	return result.RenderMarkdown(), nil
}
