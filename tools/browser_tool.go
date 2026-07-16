package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chromedp/chromedp"

	"github.com/xujian519/mady/agentcore"
)

// browserToolInput is the parsed argument structure for browser tool calls.
type browserToolInput struct {
	Action     string         `json:"action"`
	URL        string         `json:"url"`
	Ref        string         `json:"ref"`
	Text       string         `json:"text"`
	Direction  string         `json:"direction"`
	Key        string         `json:"key"`
	FullPage   bool           `json:"full_page"`
	Full       bool           `json:"full"`
	Mode       string         `json:"mode"`
	Expression string         `json:"expression"`
	FrameID    string         `json:"frame_id"`
	DialogID   string         `json:"dialog_id"`
	Accept     bool           `json:"accept"`
	PromptText string         `json:"prompt_text"`
	Question   string         `json:"question"`
	Annotate   bool           `json:"annotate"`
	CDPMethod  string         `json:"cdp_method"`
	CDPParams  map[string]any `json:"cdp_params"`
}

// browserActionHandler is a function that implements a single browser action.
type browserActionHandler func(ctx context.Context, input browserToolInput, cfg *BrowserToolConfig) (any, error)

// browserActionHandlers dispatches each action string to its implementation.
var browserActionHandlers = map[string]browserActionHandler{
	"navigate":   handleNavigate,
	"snapshot":   handleSnapshot,
	"click":      handleClick,
	"type":       handleType,
	"scroll":     handleScroll,
	"back":       handleBack,
	"press":      handlePress,
	"screenshot": handleScreenshot,
	"evaluate":   handleEvaluate,
	"dialog":     handleDialog,
	"vision":     handleVision,
	"console":    handleConsole,
	"cdp":        handleCdp,
	"get_images": handleGetImages,
}

func NewBrowserTool(cfg *BrowserToolConfig) *agentcore.Tool {
	if cfg == nil {
		cfg = &BrowserToolConfig{}
	}
	cfg.defaults()

	// Close previous manager to avoid leaking browser sessions and goroutines.
	if DefaultBrowserManager() != nil {
		DefaultBrowserManager().CloseAll()
	}
	bm := NewBrowserManager(&BrowserConfig{
		Headless:            cfg.Headless,
		AllowPrivate:        cfg.AllowPrivate,
		CommandTimeout:      cfg.CommandTimeout,
		CDPURL:              cfg.CDPURL,
		CamofoxURL:          cfg.CamofoxURL,
		CloudProvider:       cfg.CloudProvider,
		Engine:              cfg.Engine,
		DialogPolicy:        cfg.DialogPolicy,
		DialogTimeout:       cfg.DialogTimeout,
		AutoLocalForPrivate: cfg.AutoLocalForPrivate,
		RecordSessions:      cfg.RecordSessions,
		RecordingDir:        cfg.RecordingDir,
		InactivityTimeout:   cfg.InactivityTimeout,
		UserAgent:           cfg.UserAgent,
		AcceptLanguage:      cfg.AcceptLanguage,
		ProxyURL:            cfg.ProxyURL,
		ViewportWidth:       cfg.ViewportWidth,
		ViewportHeight:      cfg.ViewportHeight,
		AgentBrowserEnabled: cfg.AgentBrowserEnabled,
	})
	SetDefaultBrowserManager(bm)

	return &agentcore.Tool{
		Name:        "browser",
		Description: "控制网页浏览器。用于用户提供的 URL、交互式页面、登录流程、JavaScript 密集型页面，或作为 web_fetch 被屏蔽时的后备方案。简单信息检索请优先使用 web_search（更快、更便宜、无浏览器开销）。操作：navigate（打开 URL）、snapshot（获取页面文本及交互元素）、click（按 ref ID 点击元素）、type（按 ref ID 在元素中输入文字）、scroll（上/下滚动）、back（历史后退）、press（键盘按键）、screenshot（视口截屏）、evaluate（执行 JS）、dialog（处理 alert/confirm/prompt 对话框）、vision（让 AI 分析页面截图）、console（获取控制台日志）。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{
					"type":        "string",
					"description": "要执行的浏览器操作",
					"enum":        []string{"navigate", "snapshot", "click", "type", "scroll", "back", "press", "screenshot", "evaluate", "dialog", "vision", "console", "cdp", "get_images"},
				},
				"url": map[string]any{
					"type":        "string",
					"description": "要导航到的 URL（action=navigate 时必需）",
				},
				"ref": map[string]any{
					"type":        "string",
					"description": "快照中的元素 ref ID（例如 @e5 或 e5，action=click 和 action=type 时必需）",
				},
				"text": map[string]any{
					"type":        "string",
					"description": "要输入到字段中的文本（action=type 时必需）",
				},
				"direction": map[string]any{
					"type":        "string",
					"description": "滚动方向（action=scroll 时必需）",
					"enum":        []string{"up", "down"},
				},
				"key": map[string]any{
					"type":        "string",
					"description": "要按的按键（action=press 时必需）。常用按键：Enter、Tab、Escape、ArrowUp、ArrowDown、ArrowLeft、ArrowRight。",
				},
				"full_page": map[string]any{
					"type":        "boolean",
					"description": "截取完整页面截图（用于 action=screenshot，默认：false，仅截取视口）",
				},
				"full": map[string]any{
					"type":        "boolean",
					"description": "显示完整页面内容（用于 action=snapshot，默认：false，仅显示交互元素）",
				},
				"mode": map[string]any{
					"type":        "string",
					"description": "快照模式（用于 action=snapshot）。\"default\"：基于 JS 的无障碍树，含 XPath 引用。\"aria\"：Chrome 原生 aria 树，角色/名称信息更丰富。（默认：\"default\"）",
					"enum":        []string{"default", "aria"},
				},
				"expression": map[string]any{
					"type":        "string",
					"description": "要执行的 JavaScript 表达式（action=evaluate 时必需）",
				},
				"frame_id": map[string]any{
					"type":        "string",
					"description": "OOPIF 评估的可选 frame ID（来自快照 frame 树，用于 action=evaluate）",
				},
				"dialog_id": map[string]any{
					"type":        "string",
					"description": "待处理对话框列表中的对话框 ID（action=dialog 时必需）",
				},
				"accept": map[string]any{
					"type":        "boolean",
					"description": "是否接受（true）或关闭（false）对话框（action=dialog 时必需）",
				},
				"prompt_text": map[string]any{
					"type":        "string",
					"description": "在 prompt 对话框中输入的文本（用于 action=dialog）",
				},
				"question": map[string]any{
					"type":        "string",
					"description": "关于页面的问题（action=vision 时必需）",
				},
				"annotate": map[string]any{
					"type":        "boolean",
					"description": "在交互元素上叠加编号标签（用于 action=vision，默认：false）",
				},
				"cdp_method": map[string]any{
					"type":        "string",
					"description": "Chrome DevTools Protocol 方法（action=cdp 时必需，例如 Page.captureScreenshot、Runtime.evaluate、DOM.getDocument）",
				},
				"cdp_params": map[string]any{
					"type":        "object",
					"description": "CDP 方法参数，以 JSON 对象形式提供（用于 action=cdp）",
				},
			},
			"required": []any{"action"},
		},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			var input browserToolInput
			if err := json.Unmarshal(args, &input); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}

			// Dispatch via handler map (strategy pattern) instead of
			// a monolithic switch for each action/backend combination.
			handler, ok := browserActionHandlers[input.Action]
			if !ok {
				return nil, fmt.Errorf("unknown browser action: %s (valid: navigate, snapshot, click, type, scroll, back, press, screenshot, evaluate, dialog, vision, console, cdp, get_images)", input.Action)
			}
			return handler(ctx, input, cfg)
		},
	}
}

// normalizeRef adds a "@" prefix to a ref string if one is not present.
func normalizeRef(ref string) string {
	if !strings.HasPrefix(ref, "@") {
		return "@" + ref
	}
	return ref
}

// getXPathFromRef looks up an XPath for the given ref in the session's ref
// mapper. If not found it falls back to the JavaScript ref map stored in the
// page.
func getXPathFromRef(ctx context.Context, session *BrowserSession, ref string) (string, error) {
	xpath, ok := session.refMapper.Get(ref)
	if ok {
		return xpath, nil
	}
	// Fallback: try window.__covoRefMap in the browser.
	var jsXpath string
	js := fmt.Sprintf(`window.__covoRefMap && window.__covoRefMap[%q] || null`, ref)
	if err := chromedp.Run(ctx, chromedp.Evaluate(js, &jsXpath)); err == nil && jsXpath != "" {
		session.refMapper.Set(ref, jsXpath)
		return jsXpath, nil
	}
	if session.refMapper.Count() == 0 {
		return "", fmt.Errorf("ref %s not found. Page state is unknown. Call browser_snapshot first", ref)
	}
	return "", fmt.Errorf("ref %s not found in current page state. Call browser_snapshot to refresh", ref)
}
