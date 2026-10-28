package teams

import (
	"context"
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/session"
)

// AgentRunnerConfig 配置由真实 Agent 执行的团队成员。
type AgentRunnerConfig struct {
	// Store 是会话存储，用于成员跨任务保存/恢复上下文。
	Store *session.AgentStore
	// Config 是成员 Agent 的配置模板；Name 与 Store 会被覆盖。
	// Tools/Extensions 中的共享对象必须并发安全（不同成员并发执行）。
	Config agentcore.Config
	// KeyPrefix 是成员会话 key 前缀，默认 "team-"；与成员 ID 拼成单段 key。
	KeyPrefix string
	// Prompt 生成任务输入，默认使用 task.Subject。
	Prompt func(member Member, task *agentcore.Task) string
}

// AgentRunner 为每个团队成员运行独立 Agent，并在任务之间经 AgentStore
// 续聊（ADR 0008 前提：快照不含工具注册表，因此必须用同一份配置模板重建）。
//
// 调度器保证同一成员的任务串行执行；不同成员各自持有 Agent 实例，可并发。
// 注意：任务运行失败的半轮消息不会保存（失败重试从上次成功的上下文继续），
// 需要诊断失败回合时应由调用方在 Runner 外层记录输出。
type AgentRunner struct {
	store    *session.AgentStore
	template agentcore.Config
	prefix   string
	prompt   func(member Member, task *agentcore.Task) string
}

// NewAgentRunner 创建 Agent 执行器。
func NewAgentRunner(cfg AgentRunnerConfig) (*AgentRunner, error) {
	if cfg.Store == nil {
		return nil, fmt.Errorf("AgentRunner 需要会话存储")
	}
	if cfg.Config.Provider == nil {
		return nil, fmt.Errorf("AgentRunner 需要模型 Provider")
	}
	prefix := cfg.KeyPrefix
	if prefix == "" {
		prefix = "team-"
	}
	if strings.ContainsAny(prefix, `/\`) || prefix == "." || prefix == ".." {
		return nil, fmt.Errorf("会话 key 前缀必须是单段路径：%q", prefix)
	}
	return &AgentRunner{
		store: cfg.Store, template: cfg.Config, prefix: prefix, prompt: cfg.Prompt,
	}, nil
}

// RunTask 执行一个任务：恢复成员上下文 → 运行 → 保存上下文。
func (r *AgentRunner) RunTask(ctx context.Context, member Member, task *agentcore.Task) (string, error) {
	key := r.prefix + member.ID
	config := r.template
	config.Name = member.ID
	config.Store = r.store

	agent := agentcore.New(config)
	defer agent.Close()

	exists, err := r.store.Has(ctx, key)
	if err != nil {
		return "", fmt.Errorf("检查成员 %s 会话失败：%w", member.ID, err)
	}
	if exists {
		if err := agent.LoadState(ctx, key); err != nil {
			return "", fmt.Errorf("恢复成员 %s 上下文失败：%w", member.ID, err)
		}
	}

	input := task.Subject
	if r.prompt != nil {
		input = r.prompt(member, task)
	}
	output, err := agent.Run(ctx, input)
	if err != nil {
		return "", fmt.Errorf("成员 %s 执行任务 %s 失败：%w", member.ID, task.ID, err)
	}
	if err := agent.SaveState(ctx, key); err != nil {
		return "", fmt.Errorf("保存成员 %s 上下文失败：%w", member.ID, err)
	}
	return output, nil
}
