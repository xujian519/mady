package knowledge

import (
	"context"
	"log/slog"

	"github.com/xujian519/mady/agentcore"
)

// EvalConsumer 监听 eval_result 事件并持久化到 EvalStore。
// 同时检查 Faithfulness 阈值，低于阈值的评估标记警告。
type EvalConsumer struct {
	store  *EvalStore
	logger *slog.Logger

	// AlertThreshold 忠实度阈值。
	AlertThreshold float64
}

// EvalConsumerOption 配置 EvalConsumer。
type EvalConsumerOption func(*EvalConsumer)

// WithEvalLogger 设置日志记录器。
func WithEvalLogger(logger *slog.Logger) EvalConsumerOption {
	return func(c *EvalConsumer) { c.logger = logger }
}

// WithAlertThreshold 设置忠实度警告阈值。
func WithAlertThreshold(t float64) EvalConsumerOption {
	return func(c *EvalConsumer) { c.AlertThreshold = t }
}

// NewEvalConsumer 创建 EvalConsumer。
func NewEvalConsumer(store *EvalStore, opts ...EvalConsumerOption) *EvalConsumer {
	c := &EvalConsumer{
		store:          store,
		logger:         slog.Default(),
		AlertThreshold: 0.6,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// OnEvent 实现 agentcore.EventHandler，可直接注册到 EventBus.OnAll。
func (c *EvalConsumer) OnEvent(ev agentcore.Event) {
	if ev.EventKind() != EventTypeEvalResult {
		return
	}

	result, ok := extractEvalResult(ev)
	if !ok {
		return
	}

	// 1. 持久化到 SQLite。
	if err := c.store.Save(context.Background(), result); err != nil {
		c.logger.Error("eval: 持久化失败", "error", err)
		return
	}

	// 2. 阈值检查。
	if result.Faithfulness > 0 && result.Faithfulness < c.AlertThreshold {
		c.logger.Warn("eval: 低忠实度",
			slog.Float64("faithfulness", result.Faithfulness),
			slog.Int64("turn", result.Turn),
			slog.String("question", truncate(result.Question, 80)),
		)
	}

	// 3. 极低忠实度（< 0.4）告警。
	if result.Faithfulness > 0 && result.Faithfulness < 0.4 {
		c.logger.Error("eval: 极低忠实度 — 答案可能脱离检索上下文",
			slog.Float64("faithfulness", result.Faithfulness),
			slog.String("question", truncate(result.Question, 60)),
		)
	}
}

// extractEvalResult 从 Event 中提取 EvalResult。
func extractEvalResult(ev agentcore.Event) (EvalResult, bool) {
	evalEv, ok := ev.(evalResultEvent)
	if !ok {
		return EvalResult{}, false
	}
	return evalEv.result, true
}
