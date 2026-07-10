package agentcore

import (
	"context"
	"fmt"
	"time"
)

// --- context engine ---

// ContextEngine returns the active context engine (nil if compaction is disabled).
func (a *Agent) ContextEngine() ContextEngine {
	return a.contextEngine
}

// RegisterContextEngine registers a custom context engine factory.
func (a *Agent) RegisterContextEngine(name string, factory ContextEngineFactory) {
	a.engineReg.Register(name, factory)
}

// SetContextEngine replaces the active context engine at runtime.
func (a *Agent) SetContextEngine(engine ContextEngine) {
	a.contextEngine = engine
}

// ResetContextEngine clears per-session state for the active engine.
func (a *Agent) ResetContextEngine() {
	if a.contextEngine != nil {
		a.contextEngine.OnSessionReset()
	}
}

// ContextEngineStats returns diagnostics from the active engine.
func (a *Agent) ContextEngineStats() map[string]any {
	if a.contextEngine == nil {
		return nil
	}
	stats := map[string]any{
		"name":              a.contextEngine.Name(),
		"context_length":    a.contextEngine.ContextLength(),
		"threshold_tokens":  a.contextEngine.ThresholdTokens(),
		"compression_count": a.contextEngine.CompressionCount(),
		"last_savings_pct":  a.contextEngine.LastSavingsPct(),
	}
	if ce, ok := a.contextEngine.(*CompressorEngine); ok {
		stats["details"] = ce.SummaryStats()
	}
	return stats
}

// Close releases all resources held by the agent, including extensions and the
// event bus. Call this when the agent is no longer needed. It is safe to call
// multiple times. After Close, the agent should not be used for further Run
// calls — create a new Agent instead.
func (a *Agent) Close() {
	if a.contextEngine != nil {
		a.contextEngine.OnSessionEnd()
	}
	_ = a.extensions.Dispose()
	if a.ownsEventBus {
		a.eventBus.Close()
	}
}

// --- persistence ---

func (a *Agent) SaveState(ctx context.Context, key string) error {
	if a.config.Store == nil {
		return fmt.Errorf("未配置持久化存储")
	}
	return a.config.Store.Save(ctx, key, a.state.Snapshot())
}

func (a *Agent) LoadState(ctx context.Context, key string) error {
	if a.config.Store == nil {
		return fmt.Errorf("未配置持久化存储")
	}
	snap, err := a.config.Store.Load(ctx, key)
	if err != nil {
		return err
	}
	a.state.Restore(snap)
	return nil
}

func (a *Agent) checkpointThreadID() string {
	if a.config.Checkpoint == nil || a.config.Checkpoint.ThreadID == "" {
		return "default"
	}
	return a.config.Checkpoint.ThreadID
}

// SaveCheckpoint persists the current StateSnapshot to the configured CheckpointSaver.
func (a *Agent) SaveCheckpoint(ctx context.Context) (int64, error) {
	if a.config.Checkpoint == nil || a.config.Checkpoint.Saver == nil {
		return 0, fmt.Errorf("检查点: 未配置持久化器")
	}
	return a.config.Checkpoint.Saver.Append(ctx, a.checkpointThreadID(), a.state.Snapshot())
}

// RestoreLatestCheckpoint loads the latest snapshot for threadID into this agent.
func (a *Agent) RestoreLatestCheckpoint(ctx context.Context, threadID string) error {
	if a.config.Checkpoint == nil || a.config.Checkpoint.Saver == nil {
		return fmt.Errorf("检查点: 未配置持久化器")
	}
	tid := threadID
	if tid == "" {
		tid = a.checkpointThreadID()
	}
	snap, _, err := a.config.Checkpoint.Saver.Latest(ctx, tid)
	if err != nil {
		return err
	}
	a.state.Restore(snap)
	a.interrupted = a.state.GetInterruptReason()
	return nil
}

func (a *Agent) appendCheckpoint(ctx context.Context) error {
	if a.config.Checkpoint == nil || a.config.Checkpoint.Saver == nil {
		return nil
	}
	_, err := a.config.Checkpoint.Saver.Append(ctx, a.checkpointThreadID(), a.state.Snapshot())
	return err
}

// --- context compaction ---

func (a *Agent) maybeCompact(ctx context.Context) error {
	if a.contextEngine == nil {
		return nil
	}
	msgs := a.state.Messages()
	toolDefs := a.registry.Definitions()
	if !a.contextEngine.ShouldCompact(msgs, toolDefs, a.config.ContextWindow) {
		return nil
	}
	return a.ForceCompact(ctx)
}

func (a *Agent) ForceCompact(ctx context.Context) error {
	return a.ForceCompactWithTopic(ctx, "")
}

func (a *Agent) ForceCompactWithTopic(ctx context.Context, focusTopic string) error {
	if a.contextEngine == nil {
		return nil
	}
	msgs := a.state.Messages()
	toolDefs := a.registry.Definitions()
	tokensBefore := EstimateMessagesTokens(msgs) + EstimateToolDefinitionsTokens(toolDefs)

	a.emit(&CompactionStartEvent{
		baseEvent:     newBase(EventCompactionStart),
		TokensBefore:  tokensBefore,
		ContextWindow: a.config.ContextWindow,
	})

	start := time.Now()
	newMsgs, messagesCut, err := a.contextEngine.Compress(ctx, msgs, focusTopic)
	if err != nil {
		return err
	}
	if messagesCut > 0 {
		a.state.ReplaceMessages(newMsgs)
	}

	tokensAfter := EstimateMessagesTokens(a.state.Messages()) + EstimateToolDefinitionsTokens(toolDefs)

	a.emit(&CompactionEndEvent{
		baseEvent:    newBase(EventCompactionEnd),
		TokensBefore: tokensBefore,
		TokensAfter:  tokensAfter,
		MessagesCut:  messagesCut,
		Duration:     time.Since(start),
	})
	return nil
}
