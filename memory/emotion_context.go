package memory

import (
	"strconv"
	"strings"

	"github.com/xujian519/mady/agentcore"
)

// EmotionContext 是从 psychological 扩展注入的消息中提取的情绪摘要。
// 当上下文中无心理分析块时，所有字段为零值。
type EmotionContext struct {
	// Present 为 true 表示检测到心理分析块。
	Present bool
	// Valence 愉悦度 (-1~1)。负值表示负面情绪。
	Valence float64
	// Arousal 唤醒度 (0~1)。高值表示激动/紧张。
	Arousal float64
	// Dominance 支配度 (0~1)。低值表示失控感。
	Dominance float64
	// DominantEmotion 主导情绪标签（如 "frustration", "joy"）。
	DominantEmotion string
}

// ExtractEmotionContext 从消息列表中提取心理分析块。
// 检测 psychological 扩展注入的 【当前感知的用户心理状态】 块，
// 解析其中的 VAD 三维情绪坐标。
func ExtractEmotionContext(msgs []agentcore.Message) EmotionContext {
	for _, msg := range msgs {
		if msg.Role != agentcore.RoleSystem {
			continue
		}
		if strings.Contains(msg.Content, "【当前感知的用户心理状态】") {
			return parseEmotionBlock(msg.Content)
		}
	}
	return EmotionContext{}
}

// parseEmotionBlock 从 psychological 上下文块文本中解析 VAD 坐标。
// 格式示例: "主导情绪: frustration, VAD(愉悦度=-0.55, 唤醒度=0.55, 支配度=0.35)"
func parseEmotionBlock(block string) EmotionContext {
	ec := EmotionContext{Present: true}

	// 解析 VAD 坐标
	if idx := strings.Index(block, "VAD("); idx != -1 {
		start := idx + 4 // 跳过 "VAD("
		end := strings.Index(block[start:], ")")
		if end != -1 {
			vadPart := block[start : start+end]
			parts := strings.Split(vadPart, ",")
			for _, part := range parts {
				part = strings.TrimSpace(part)
				switch {
				case strings.HasPrefix(part, "愉悦度="):
					ec.Valence = parseFloatField(part, "愉悦度=")
				case strings.HasPrefix(part, "唤醒度="):
					ec.Arousal = parseFloatField(part, "唤醒度=")
				case strings.HasPrefix(part, "支配度="):
					ec.Dominance = parseFloatField(part, "支配度=")
				}
			}
		}
	}

	// 解析主导情绪
	if idx := strings.Index(block, "主导情绪:"); idx != -1 {
		rest := block[idx+len("主导情绪:"):]
		rest = strings.TrimSpace(rest)
		if commaIdx := strings.Index(rest, ","); commaIdx != -1 {
			ec.DominantEmotion = strings.TrimSpace(rest[:commaIdx])
		} else if newlineIdx := strings.Index(rest, "\n"); newlineIdx != -1 {
			ec.DominantEmotion = strings.TrimSpace(rest[:newlineIdx])
		} else {
			ec.DominantEmotion = rest
		}
	}

	return ec
}

// parseFloatField 从 "key=0.5" 格式的字符串中提取浮点值。
func parseFloatField(s, prefix string) float64 {
	val := strings.TrimPrefix(s, prefix)
	f, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return 0
	}
	return f
}

// IsNegative 返回当前情绪是否为负面（愉悦度 < -0.3）。
func (ec EmotionContext) IsNegative() bool {
	return ec.Valence < -0.3
}

// IsPositive 返回当前情绪是否为正面（愉悦度 > 0.3）。
func (ec EmotionContext) IsPositive() bool {
	return ec.Valence > 0.3
}

// IsHighArousal 返回唤醒度是否偏高（> 0.7）。
func (ec EmotionContext) IsHighArousal() bool {
	return ec.Arousal > 0.7
}

// EmotionBoost 返回情绪对记忆重要性的加成系数。
// 情绪化时刻（高唤醒度）的记忆更重要。
func (ec EmotionContext) EmotionBoost() float64 {
	if !ec.Present {
		return 0
	}
	// 高唤醒度 + 极端情绪 → 更高的加成
	boost := ec.Arousal * 0.2 // base: arousal contributes up to 0.2
	if ec.Valence < -0.5 || ec.Valence > 0.5 {
		boost += 0.1 // extreme valence adds 0.1
	}
	if boost > 0.3 {
		boost = 0.3
	}
	return boost
}
