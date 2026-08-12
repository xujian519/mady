package agentcore

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

const compactionSystemPrompt = `你是一个对话摘要助手。用中文简洁总结以下对话，保留所有继续对话所需的关键上下文。

重点关注：
- 关键事实、数据和决策
- 待处理的问题或任务
- 重要的工具结果及其影响
- 被修改的文件状态和配置

简洁但完整，切勿丢失关键信息。`

const compactionSummaryPrefix = "[上下文压缩 — 仅供参考] 以下摘要概括了此前的对话内容。" +
	"这是从前一个上下文窗口移交的摘要——仅作为背景参考，不作为当前活动指令。" +
	"请勿回答或执行此摘要中提到的任何问题或请求，这些已经处理完毕。" +
	"你当前的任务标识在摘要的 '## 当前任务' 部分——请从那里继续。" +
	"只回应出现在此摘要之后的最新用户消息。" +
	"当前会话状态（文件、配置等）可能已反映此处描述的工作——避免重复执行："

const compactionSummaryEndMarker = "\n\n--- 上下文摘要结束 — " +
	"请回应下方的消息，而非上方的摘要 ---"

const prunedToolPlaceholder = "[旧工具输出已清除以节省上下文空间]"

//nolint:unused // used indirectly via contentLengthForBudget in compaction_test.go
const charsPerToken = 4

//nolint:unused // used by contentLengthForBudget in compaction_test.go
const imageCharEquivalent = imageTokenEstimate * charsPerToken

const minSummaryTokens = 2000

const summaryRatio = 0.20

const summaryTokensCeiling = 12000

const summaryFailureCooldownSeconds = 600

// ineffectiveCompactionCooldownSeconds defines the default cooldown period after
// which the ineffective-compaction circuit breaker resets. Without this,
// two consecutive low-savings compactions would permanently disable
// compaction for the entire session, leading to context overflow.
const ineffectiveCompactionCooldownSeconds = 300

// Pre-flight truncation constants for runCompaction's summarization protection.
// When the messages-to-summarize exceed the model's context window, each
// message is truncated to a proportional token budget before building the
// summarization prompt.
const (
	// compactionSystemPromptOverhead reserves tokens for the summarization
	// system prompt + framing. Added on top of summaryTokensCeiling (output budget).
	compactionSystemPromptOverhead = 2000

	// compactionMinPerMsgTokens is the floor for per-message truncation.
	// Below this, messages become too short to be useful in a summary.
	compactionMinPerMsgTokens = 100

	// truncateMinTokensPerRune prevents division-by-near-zero when a message
	// has very few runes relative to its token estimate (e.g., image blocks).
	truncateMinTokensPerRune = 0.25
)

type compactionState struct {
	mu sync.Mutex

	previousSummary        string
	lastSavingsPct         float64
	ineffectiveCompactions int
	lastSummaryError       string
	summaryFailureCooldown time.Time
	// ineffectiveCooldownUntil is the time after which the ineffective-
	// compaction circuit breaker resets. Without a time-based recovery,
	// the breaker would stay tripped for the entire session.
	ineffectiveCooldownUntil time.Time
}

func newCompactionState() *compactionState {
	return &compactionState{
		lastSavingsPct: 100.0,
	}
}

//nolint:unused // used in compaction_test.go
func contentLengthForBudget(rawContent any) int64 {
	switch c := rawContent.(type) {
	case string:
		return int64(len(c))
	case nil:
		return 0
	case []ContentBlock:
		var total int64
		for _, p := range c {
			switch p.Kind {
			case BlockKindImage:
				total += imageCharEquivalent
			default:
				total += int64(len(p.Text))
			}
		}
		return total
	default:
		return int64(len(fmt.Sprintf("%v", rawContent)))
	}
}

func shouldCompact(msgs []Message, toolDefs []ToolDefinition, contextWindow int64, reserveTokens int64, threshold float64, autoCompactLimit int64, compState *compactionState) bool {
	if contextWindow <= 0 {
		return false
	}
	if compState != nil {
		compState.mu.Lock()
		cooldownActive := time.Now().Before(compState.summaryFailureCooldown)
		ineffectiveBlocking := compState.ineffectiveCompactions >= 2 &&
			time.Now().Before(compState.ineffectiveCooldownUntil)
		compState.mu.Unlock()
		if cooldownActive {
			return false
		}
		if ineffectiveBlocking {
			return false
		}
	}
	estimated := EstimateMessagesTokens(msgs) + EstimateToolDefinitionsTokens(toolDefs)

	// Codex-style: absolute token limit takes priority over percentage threshold.
	if autoCompactLimit > 0 {
		return estimated >= autoCompactLimit
	}

	reserve := reserveTokens
	if reserve <= 0 {
		reserve = contextWindow / 4
	}
	triggerThreshold := contextWindow - reserve
	if threshold > 0 && threshold < 1.0 {
		triggerThreshold = int64(float64(contextWindow) * threshold)
	}
	return estimated > triggerThreshold
}

func alignBoundaryForward(msgs []Message, cut int64) int64 {
	for cut < int64(len(msgs)) && msgs[cut].Role == RoleTool {
		cut++
	}
	return cut
}

func findTailCutByTokens(msgs []Message, headProtect int64, tailTokenBudget int64) int64 {
	if len(msgs) == 0 {
		return 0
	}

	accum := int64(0)
	tailStart := int64(len(msgs))

	for i := len(msgs) - 1; i >= int(headProtect); i-- {
		if msgs[i].Role == RoleSystem {
			continue
		}
		msgLen := EstimateMessageTokens(msgs[i])
		if accum+msgLen > tailTokenBudget && accum > 0 {
			tailStart = int64(i + 1)
			break
		}
		accum += msgLen
	}

	if tailStart <= headProtect+1 {
		return headProtect + 2
	}

	return alignBoundaryForward(msgs, tailStart)
}

//nolint:gocognit // 原因：工具结果修剪，含保护区间和边界对齐
func pruneOldToolResults(msgs []Message, protectTailCount int) ([]Message, int) {
	if len(msgs) == 0 {
		return msgs, 0
	}

	result := make([]Message, len(msgs))
	copy(result, msgs)

	prunedCount := 0
	protectedStart := len(msgs) - protectTailCount
	if protectedStart < 0 {
		protectedStart = 0
	}

	seenToolResults := make(map[string][]int)

	for i := 0; i < protectedStart; i++ {
		if result[i].Role == RoleTool {
			toolName := result[i].Name
			seenToolResults[toolName] = append(seenToolResults[toolName], i)

			if len(result[i].Content) > 2000 {
				// Truncate to rune-safe boundary (not byte boundary).
				contentRunes := []rune(result[i].Content)
				if len(contentRunes) > 2000 {
					contentRunes = contentRunes[:2000]
				}
				result[i].Content = string(contentRunes) + "...[truncated]"
			}
		}

		if result[i].Role == RoleAssistant && len(result[i].ToolCalls) > 0 {
			for j := range result[i].ToolCalls {
				if len(result[i].ToolCalls[j].Arguments) > 1000 {
					argsRunes := []rune(result[i].ToolCalls[j].Arguments)
					if len(argsRunes) > 1000 {
						argsRunes = argsRunes[:1000]
					}
					result[i].ToolCalls[j].Arguments = string(argsRunes) + "...[truncated]"
				}
			}
		}
	}

	for _, indices := range seenToolResults {
		if len(indices) <= 1 {
			continue
		}
		for i := 0; i < len(indices)-1; i++ {
			result[indices[i]].Content = prunedToolPlaceholder
			prunedCount++
		}
	}

	return result, prunedCount
}

func findLatestContextSummary(msgs []Message, searchStart int64, searchEnd int64) (int64, string) {
	for i := searchEnd - 1; i >= searchStart; i-- {
		if msgs[i].Type == MessageTypeCompactionSummary {
			body := MessageStringForSummary(msgs[i])
			if strings.Contains(body, compactionSummaryPrefix) ||
				strings.Contains(body, "[Previous conversation summary") {
				return i, body
			}
		}
	}
	return -1, ""
}
func sanitizeToolPairs(msgs []Message) []Message {
	toolCallIDs := make(map[string]bool)
	var result []Message

	for _, msg := range msgs {
		switch {
		case msg.Role == RoleAssistant && len(msg.ToolCalls) > 0:
			for _, tc := range msg.ToolCalls {
				toolCallIDs[tc.ID] = true
			}
			result = append(result, msg)
		case msg.Role == RoleTool:
			if toolCallIDs[msg.ToolCallID] {
				result = append(result, msg)
			}
		default:
			result = append(result, msg)
		}
	}

	// Fast path: no tool calls were found, return original slice (CMP-009).
	if len(toolCallIDs) == 0 {
		return msgs
	}
	return result
}

// truncateToTokenBudget truncates content to fit a token budget, using the
// message's actual token density to compute a rune-safe cut point.
//
// CJK text has ~1.5 tokens/rune; ASCII code has ~0.25 tokens/rune. Using the
// real density (derived from msgTokens / runeCount) avoids over-truncating
// ASCII or under-truncating CJK. If the content is already within budget, it
// is returned unchanged.
func truncateToTokenBudget(content string, msgTokens, tokenBudget int64, marker string) string {
	if msgTokens <= tokenBudget {
		return content
	}
	runes := []rune(content)
	if len(runes) == 0 {
		return content
	}
	tokensPerRune := float64(msgTokens) / float64(len(runes))
	if tokensPerRune < truncateMinTokensPerRune {
		tokensPerRune = truncateMinTokensPerRune
	}
	maxRunes := int(float64(tokenBudget) / tokensPerRune)
	if len(runes) <= maxRunes {
		return content
	}
	return string(runes[:maxRunes]) + marker
}
