package tools

import (
	"fmt"
	"os"
	"time"
)

// BrowserToolConfig 统一浏览器工具的配置，供 NewBrowserTool 及浏览器子模块共享使用。
type BrowserToolConfig struct {
	Headless            bool
	AllowPrivate        bool
	CommandTimeout      time.Duration
	CDPURL              string
	CamofoxURL          string
	CloudProvider       string
	Engine              string
	DialogPolicy        DialogPolicy
	DialogTimeout       time.Duration
	AutoLocalForPrivate bool
	RecordSessions      bool
	RecordingDir        string
	VisionModel         string
	// Vision 配置 browser vision action / browser_vision 工具的视觉分析能力。
	// nil 时 defaults() 按 MADY_VISION_* 环境变量构造；仍未配置则 vision
	// 分析返回明确的“未配置”错误，绝不返回伪造的占位结果。
	// 通常由 Extension.WithVision 或 BuildTools 自动共享 vision_analyze 的配置。
	Vision              *VisionToolConfig
	MaxImageSize        int
	InactivityTimeout   time.Duration
	UserAgent           string
	AcceptLanguage      string
	ProxyURL            string
	ViewportWidth       int
	ViewportHeight      int
	AgentBrowserEnabled bool
	EgoLiteEnabled      bool
	EgoLiteTaskName     string
}

// Validate 校验浏览器配置中的明显错误（负超时/非法尺寸）。
// 零值合法——defaults() 会填充默认值；负值通常是配置错误信号，提前暴露。
func (c *BrowserToolConfig) Validate() error {
	if c.CommandTimeout < 0 {
		return fmt.Errorf("tools: Browser.CommandTimeout 不能为负（%v）", c.CommandTimeout)
	}
	if c.DialogTimeout < 0 {
		return fmt.Errorf("tools: Browser.DialogTimeout 不能为负（%v）", c.DialogTimeout)
	}
	if c.InactivityTimeout < 0 {
		return fmt.Errorf("tools: Browser.InactivityTimeout 不能为负（%v）", c.InactivityTimeout)
	}
	if c.ViewportWidth < 0 || c.ViewportHeight < 0 {
		return fmt.Errorf("tools: Browser.Viewport 尺寸不能为负（%dx%d）", c.ViewportWidth, c.ViewportHeight)
	}
	if c.MaxImageSize < 0 {
		return fmt.Errorf("tools: Browser.MaxImageSize 不能为负（%d）", c.MaxImageSize)
	}
	return nil
}

func (c *BrowserToolConfig) defaults() {
	if c.CommandTimeout <= 0 {
		c.CommandTimeout = 30 * time.Second
	}
	if c.DialogTimeout <= 0 {
		c.DialogTimeout = 300 * time.Second
	}
	if c.InactivityTimeout <= 0 {
		c.InactivityTimeout = 5 * time.Minute
	}
	if c.DialogPolicy == "" {
		c.DialogPolicy = DialogMustRespond
	}
	if os.Getenv("AGENT_BROWSER_ENABLED") == "true" || os.Getenv("AGENT_BROWSER_PATH") != "" {
		c.AgentBrowserEnabled = true
	}
	// 解析视觉分析配置：显式注入的优先，否则按环境变量/未配置错误兜底。
	if c.Vision == nil {
		c.Vision = &VisionToolConfig{}
	}
	c.Vision.defaults()
}
