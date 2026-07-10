package agentcore

import (
	"context"
	"maps"
	"sort"
)

// DefaultContextBuilder 是 ContextBuilder 的默认实现。
//
// 逐层处理：遍历配置的层，用对应的 LayerProvider 生成内容，
// 在 token 预算下执行截断，最终组装为完整的消息列表。
//
// 布局策略（借鉴 Claude Code 的 Progressive Disclosure）：
//
//	[Static - 每会话可缓存]
//	  0: System Prompt（角色定义、安全规则）
//	  1: Tool Definitions
//	[Dynamic - 每轮变化]
//	  2: Knowledge Context（知识增强，按需触发）
//	  3: Memory Context（长期记忆，自动注入）
//	[Conversation - 始终保留]
//	  4: Conversation History（对话轮次）
type DefaultContextBuilder struct {
	cfg ContextBuilderConfig
}

// NewDefaultContextBuilder 创建默认的 ContextBuilder。
func NewDefaultContextBuilder(cfg ContextBuilderConfig) *DefaultContextBuilder {
	return &DefaultContextBuilder{cfg: cfg}
}

// Build 执行上下文组装。
func (b *DefaultContextBuilder) Build(ctx context.Context, input BuildInput) BuildOutput {
	if !b.cfg.Enabled {
		return BuildOutput{
			Messages: input.Messages,
			ToolDefs: input.ToolDefs,
		}
	}

	// 获取各层配置（使用传入配置或默认值）
	layerCfgs := make(map[ContextLayer]LayerConfig, len(ValidContextLayers))
	if input.LayerConfigs != nil {
		maps.Copy(layerCfgs, input.LayerConfigs)
	}
	for _, l := range ValidContextLayers {
		if _, ok := layerCfgs[l]; !ok {
			if d, ok := b.cfg.DefaultLayerConfigs[l]; ok {
				layerCfgs[l] = d
			} else {
				layerCfgs[l] = DefaultLayerConfig(l)
			}
		}
	}

	// 构建 provider 索引
	providerByLayer := make(map[ContextLayer]LayerProvider)
	if b.cfg.Providers != nil {
		for _, p := range b.cfg.Providers {
			providerByLayer[p.Layer()] = p
		}
	}

	// 计算可用 token 预算
	totalBudget := input.ContextWindow - max(input.ReserveTokens, 0)
	if totalBudget <= 0 {
		totalBudget = input.ContextWindow
	}

	// 按层优先级排序（Priority 升序，数字小先处理）
	type layerJob struct {
		layer ContextLayer
		cfg   LayerConfig
	}
	var jobs []layerJob
	for _, l := range ValidContextLayers {
		cfg := layerCfgs[l]
		if !cfg.Enabled {
			continue
		}
		jobs = append(jobs, layerJob{layer: l, cfg: cfg})
	}
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].cfg.Priority < jobs[j].cfg.Priority
	})

	// 逐层处理
	var systemMsgs []Message // 注入到 system 前后的消息
	var historyMsgs []Message
	usage := BuildUsage{
		ByLayer: make(map[ContextLayer]int64),
	}

	// 分离历史中的 system 和 非 system 消息
	for _, msg := range input.Messages {
		if msg.Role == RoleSystem {
			systemMsgs = append(systemMsgs, msg)
		} else {
			historyMsgs = append(historyMsgs, msg)
		}
	}

	var injectedMsgs []Message
	remainingBudget := totalBudget

	for _, job := range jobs {
		if remainingBudget <= 0 {
			break
		}

		maxTokens := job.cfg.MaxTokens
		if maxTokens <= 0 || maxTokens > remainingBudget {
			maxTokens = remainingBudget
		}

		// 找对应的 provider
		provider, hasProvider := providerByLayer[job.layer]

		switch job.layer {
		case LayerSystem:
			// System 层：使用 provider 或默认 system prompt
			if hasProvider {
				msgs, err := provider.Provide(ctx, input, job.cfg)
				if err == nil && len(msgs) > 0 {
					injectedMsgs = append(injectedMsgs, msgs...)
					tok := estimateMessagesTokens(msgs)
					usage.ByLayer[LayerSystem] = tok
					remainingBudget -= tok
				}
			}
			// 同时保留原始 system messages
			// （由 agent 自己管理的 SystemPrompt）

		case LayerTools:
			// Tools 层：可通过 provider 自定义，否则使用 input.ToolDefs
			if hasProvider {
				msgs, err := provider.Provide(ctx, input, job.cfg)
				if err == nil && len(msgs) > 0 {
					injectedMsgs = append(injectedMsgs, msgs...)
					tok := estimateMessagesTokens(msgs)
					usage.ByLayer[LayerTools] = tok
					remainingBudget -= tok
				}
			}
			// 工具定义由 ToolDefs 直接传递，不消耗消息 token 预算

		case LayerKnowledge:
			if !hasProvider {
				continue
			}
			msgs, err := provider.Provide(ctx, input, job.cfg)
			if err != nil || len(msgs) == 0 {
				continue
			}
			tok := estimateMessagesTokens(msgs)
			if tok > maxTokens {
				msgs = truncateMessagesByTokens(msgs, maxTokens)
				tok = maxTokens
			}
			if tok > 0 {
				injectedMsgs = append(injectedMsgs, msgs...)
				usage.ByLayer[LayerKnowledge] = tok
				remainingBudget -= tok
			}

		case LayerMemory:
			if !hasProvider {
				continue
			}
			msgs, err := provider.Provide(ctx, input, job.cfg)
			if err != nil || len(msgs) == 0 {
				continue
			}
			tok := estimateMessagesTokens(msgs)
			if tok > maxTokens {
				msgs = truncateMessagesByTokens(msgs, maxTokens)
				tok = maxTokens
			}
			if tok > 0 {
				injectedMsgs = append(injectedMsgs, msgs...)
				usage.ByLayer[LayerMemory] = tok
				remainingBudget -= tok
			}

		case LayerHistory:
			// History 层：保留原始对话历史（已经由 ContextEngine 压缩）
			tok := estimateMessagesTokens(historyMsgs)
			if tok > remainingBudget {
				historyMsgs = truncateMessagesByTokens(historyMsgs, remainingBudget)
				tok = remainingBudget
			}
			usage.ByLayer[LayerHistory] = tok
			remainingBudget -= tok
		}
	}

	// 计算 Token 消耗
	toolTok := estimateToolDefTokens(input.ToolDefs)
	usage.ToolDefTokens = toolTok
	for _, tok := range usage.ByLayer {
		usage.TotalTokens += tok
	}
	usage.TotalTokens += toolTok

	// 组装最终消息列表：System → injected → History
	var final []Message
	final = append(final, systemMsgs...)
	final = append(final, injectedMsgs...)
	final = append(final, historyMsgs...)

	return BuildOutput{
		Messages: final,
		ToolDefs: input.ToolDefs,
		Usage:    usage,
	}
}

// --- Token 估算辅助 ---

// estimateMessagesTokens delegates to token.go's EstimateMessagesTokens.
func estimateMessagesTokens(msgs []Message) int64 {
	return EstimateMessagesTokens(msgs)
}

// estimateToolDefTokens delegates to token.go's EstimateToolDefinitionsTokens.
func estimateToolDefTokens(defs []ToolDefinition) int64 {
	return EstimateToolDefinitionsTokens(defs)
}

// truncateMessagesByTokens 按 token 预算截断消息列表。
// 从最旧的消息开始丢弃（保留最新的 N 条消息）。
func truncateMessagesByTokens(msgs []Message, maxTokens int64) []Message {
	if maxTokens <= 0 {
		return nil
	}
	var reversed []Message
	tokensUsed := int64(0)
	// 从最新的消息开始收集（反向）
	for i := len(msgs) - 1; i >= 0; i-- {
		tok := int64(len([]rune(msgs[i].Content)) / 4)
		if tokensUsed+tok > maxTokens {
			break
		}
		tokensUsed += tok
		reversed = append(reversed, msgs[i])
	}
	// 反转顺序以恢复时间先后
	result := make([]Message, len(reversed))
	for i, m := range reversed {
		result[len(result)-1-i] = m
	}
	return result
}
