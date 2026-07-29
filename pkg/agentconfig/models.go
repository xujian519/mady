// Package agentconfig assembles the agent provider/model/thinking configuration
// from environment variables.
package agentconfig

import "os"

// ProviderInfo 描述一个可用的 LLM Provider。
type ProviderInfo struct {
	// Name 是 provider 标识符（deepseek / zhipu / kimi / generic）。
	Name string
	// Label 是面向用户的中文名称。
	Label string
	// Models 是该 Provider 支持的模型列表。
	Models []ModelInfo
}

// ModelInfo 描述一个可用的模型。
type ModelInfo struct {
	// Name 是模型标识符（如 "deepseek-v4-flash"）。
	Name string
	// Label 是面向用户的模型名称（如 "DeepSeek V4 Flash"）。
	Label string
	// Group 是分组标识（"recommended" 或 "all"），用于模型选择器中的分组展示。
	Group string
	// Description 是简短描述（如 "128K 上下文, 快速推理"）。
	Description string
}

// ProviderCatalog 返回所有支持的 Provider 及其模型列表。
// 供 TUI 设置面板和命令中心使用。
func ProviderCatalog() []ProviderInfo {
	return []ProviderInfo{
		{
			Name: "deepseek", Label: "DeepSeek",
			Models: []ModelInfo{
				{Name: "deepseek-v4-flash", Label: "DeepSeek V4 Flash", Group: "recommended", Description: "1M 上下文, 快速推理"},
				{Name: "deepseek-v4-pro", Label: "DeepSeek V4 Pro", Group: "recommended", Description: "1M 上下文, 深度推理"},
			},
		},
		{
			Name: "zhipu", Label: "智谱 GLM",
			Models: []ModelInfo{
				{Name: "glm-5.2", Label: "GLM-5.2", Group: "recommended", Description: "1M 上下文, 高性价比"},
				{Name: "glm-5v-turbo", Label: "GLM-5V Turbo", Group: "all", Description: "1M 上下文, 多模态"},
			},
		},
		{
			Name: "kimi", Label: "Kimi 月之暗面",
			Models: []ModelInfo{
				{Name: "kimi-k2.6", Label: "Kimi K2.6", Group: "recommended", Description: "1M 上下文, 多模态"},
				{Name: "kimi-k2.7-code", Label: "Kimi K2.7 Code", Group: "all", Description: "1M 上下文, 代码优化"},
			},
		},
		{
			Name: "generic", Label: "通用 OpenAI 兼容",
			Models: nil, // 动态加载，见 ModelsForProvider
		},
	}
}

// ModelsForProvider 返回指定 Provider 的模型列表。
// 对 generic（通用 OpenAI 兼容）模型，从 MODEL 环境变量动态生成条目。
func ModelsForProvider(providerName string) []ModelInfo {
	for _, p := range ProviderCatalog() {
		if p.Name == providerName {
			// generic 模型需要从环境变量动态加载
			if providerName == "generic" {
				models := make([]ModelInfo, 0)
				if m := os.Getenv("MODEL"); m != "" {
					models = append(models, ModelInfo{
						Name: m, Label: m, Group: "recommended",
						Description: "通过 MODEL 环境变量指定",
					})
				} else {
					models = append(models, ModelInfo{
						Name: "(未配置)", Label: "(未配置)", Group: "all",
						Description: "请设置 MODEL 环境变量",
					})
				}
				return models
			}
			return p.Models
		}
	}
	return nil
}

// DefaultModelForProvider 返回指定 Provider 的默认模型名。
// 对齐 DefaultModel() 和 BuildProvider()。
func DefaultModelForProvider(providerName string) string {
	switch providerName {
	case "deepseek":
		return "deepseek-v4-flash"
	case "zhipu":
		return "glm-5.2"
	case "kimi":
		return "kimi-k2.6"
	default:
		return os.Getenv("MODEL")
	}
}

// ProviderNameLabel 返回 providerName → 中文名称的映射，用于 TUI 设置面板。
func ProviderNameLabel(name string) string {
	for _, p := range ProviderCatalog() {
		if p.Name == name {
			return p.Label
		}
	}
	return name
}

// ProviderNameList 返回所有支持的 provider 名称列表。
func ProviderNameList() []string {
	providers := ProviderCatalog()
	names := make([]string, len(providers))
	for i, p := range providers {
		names[i] = p.Name
	}
	return names
}
