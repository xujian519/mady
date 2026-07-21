package graph

import (
	"context"
	"fmt"
	"time"
)

// NodePolicy 配置 Pregel 节点的运行时行为。nil policy 表示无重试、无超时、普通模式。
type NodePolicy struct {
	// MaxRetries 设置失败后的最大重试次数。0 表示不重试。
	MaxRetries int

	// RetryDelay 是重试之间的基准等待时间（每次重试翻倍）。
	// 0 时默认 100ms。
	RetryDelay time.Duration

	// Timeout 限制单次节点执行（含所有重试）的总时长。0 表示无超时。
	Timeout time.Duration

	// SideEffect 为 true 时，节点被视为副作用节点：其返回的 state 内容不被 merge 到共享 state。
	// 用于执行 I/O 或外部调用、不需要修改 state 的节点。
	SideEffect bool
}

// RetryPolicy 已废弃，保留向后兼容。新代码使用 NodePolicy。
//
// Deprecated: 使用 NodePolicy。
type RetryPolicy = NodePolicy

const defaultRetryDelay = 100 * time.Millisecond

// safeExecute 执行 PregelNode 并恢复 panic。
func safeExecute(ctx context.Context, nodeName string, nodeFn PregelNode, snapshot PregelState) (out PregelState, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("pregel: node %s panicked: %v", nodeName, r)
		}
	}()
	return nodeFn(ctx, snapshot)
}

// executeWithPolicy 使用配置的策略包装 PregelNode 执行。
// 包括重试（指数退避）、超时（context deadline）、副作用处理、panic 恢复。
func executeWithPolicy(ctx context.Context, nodeName string, nodeFn PregelNode, snapshot PregelState, policy *NodePolicy) (PregelState, error) {
	if policy == nil {
		// 快速路径：无策略，直接执行（保留 panic recovery）。
		return safeExecute(ctx, nodeName, nodeFn, snapshot)
	}

	// 超时控制。
	if policy.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, policy.Timeout)
		defer cancel()
	}

	maxRetries := policy.MaxRetries
	if maxRetries < 0 {
		maxRetries = 0
	}
	delay := policy.RetryDelay
	if delay <= 0 {
		delay = defaultRetryDelay
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// 指数退避：delay * 2^(attempt-1)
			backoff := delay * time.Duration(1<<(attempt-1))
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		out, err := safeExecute(ctx, nodeName, nodeFn, snapshot.Clone())

		if err == nil {
			if policy.SideEffect {
				// 副作用节点：返回空 state，merge 为 no-op。
				return PregelState{}, nil
			}
			return out, nil
		}

		lastErr = err

		// context 取消则不重试。
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}

	return nil, fmt.Errorf("pregel:%s: %w (重试 %d 次后仍失败)", nodeName, lastErr, maxRetries)
}
