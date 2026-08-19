package chat

import (
	"fmt"
	"strings"

	"github.com/xujian519/mady/tui/core"
	tuitheme "github.com/xujian519/mady/tui/theme"
)

// renderMessagesRange 从 start 开始渲染连续消息到 out/ranges。
// 快路径（start > 0，拼接）和慢路径（start = 0，全量）共用此函数。
// renderMessageSeparator 的 i > 0 条件对两路径均成立：快路径保证
// firstDirtyIdx > 0，慢路径从 0 开始自然跳过首次无前任消息。
func (h *ChatHistory) renderMessagesRange(
	msgs []ChatMessage, start int,
	theme ChatHistoryTheme, expandedGroups map[int]bool, width int64,
	cache map[string]cachedMessage,
	out []string, ranges []msgRange,
) ([]string, []msgRange, [][]core.LinkSpan) {
	var outLinks [][]core.LinkSpan
	for i := start; i < len(msgs); i++ {
		m := msgs[i]
		if groupEnd, ok := h.detectToolGroup(msgs, i); ok {
			lines, r, links := h.renderToolGroup(msgs, i, groupEnd, expandedGroups[i], theme, width, cache)
			out = append(out, lines...)
			outLinks = append(outLinks, links...)
			// renderToolGroup reports lines relative to itself (startLine 0);
			// rebase onto the absolute output position so hit-testing and
			// scroll-to-match use consistent coordinates.
			r.startLine = len(out) - len(lines)
			r.endLine = len(out)
			ranges = append(ranges, r)
			i = groupEnd
			continue
		}
		if i > 0 {
			roleChanged := msgs[i-1].Role != m.Role
			if roleChanged {
				// 跨角色切换时色带行自身承担分隔，不再叠加空行分隔符，
				// 避免空行+色带产生 2 行间距导致视觉过松。
				band := renderRoleTransitionBand(m.Role, width, theme)
				out = append(out, band...)
				if len(band) > 0 {
					outLinks = append(outLinks, nilLinks(len(band))...)
				}
			} else {
				sep := h.renderMessageSeparator(msgs[i-1], m, width, theme)
				out = append(out, sep...)
				if len(sep) > 0 {
					outLinks = append(outLinks, nilLinks(len(sep))...)
				}
			}
		}
		startLine := len(out)
		msgLines, msgLinks := h.renderMessageCachedWithCache(m, theme, width, cache)
		trimmed, _ := trimBlankEdges(msgLines)
		out = append(out, trimmed...)
		if msgLinks != nil {
			outLinks = append(outLinks, msgLinks...)
		} else if len(trimmed) > 0 {
			outLinks = append(outLinks, nilLinks(len(trimmed))...)
		}
		ranges = append(ranges, msgRange{startLine: startLine, endLine: len(out), msgIndex: i})
	}
	return out, ranges, outLinks
}

// detectToolGroup 检查 msgs[i] 是否为一组连续工具/系统消息的起始。
// 如果是且不在中间轮次（mid-turn，Assistant 仍在 Pending 中），返回
// groupEnd（含）和 ok=true。快速路径和慢速路径共用此检测逻辑。
func (h *ChatHistory) detectToolGroup(msgs []ChatMessage, i int) (groupEnd int, ok bool) {
	if msgs[i].Role != RoleTool && msgs[i].Role != RoleSystem {
		return 0, false
	}
	end := i
	for j := i + 1; j < len(msgs); j++ {
		r := msgs[j].Role
		if r == RoleTool || r == RoleSystem {
			end = j
		} else {
			break
		}
	}
	// 单条工具消息不折叠
	if end == i {
		return 0, false
	}
	// 检查是否为中间轮次（消息在末尾且前一条 Assistant 消息仍在 Pending）
	if end == len(msgs)-1 {
		foundPrev := false
		for j := i - 1; j >= 0; j-- {
			if msgs[j].Role != RoleTool && msgs[j].Role != RoleSystem {
				if msgs[j].Pending {
					return 0, false // mid-turn，不折叠
				}
				foundPrev = true
				break
			}
		}
		// 没有前一条非工具消息（如 i==0 全部为工具消息），
		// 原始逻辑 midTurn 保持 true，不折叠
		if !foundPrev {
			return 0, false
		}
	}
	return end, true
}

// renderToolGroup 渲染一组连续的工具/系统消息为折叠（[+]）或展开（[-]）形式。
// 展开时使用左侧色带（│）把多个工具/系统消息连成一条紧凑时间线，
// 避免原本散落的卡片感。
// 返回渲染行、行区间与链接元数据（展开时成员行加色带前缀，链接列偏移）。
func (h *ChatHistory) renderToolGroup(msgs []ChatMessage, start, end int, expanded bool, theme ChatHistoryTheme, width int64, cache map[string]cachedMessage) ([]string, msgRange, [][]core.LinkSpan) { //nolint:gocognit // 渲染/分发/状态机复杂分支，拆分列入 P3
	toolCount, sysCount := 0, 0
	for j := start; j <= end; j++ {
		if msgs[j].Role == RoleTool {
			toolCount++
		} else {
			sysCount++
		}
	}

	var lines []string
	var links [][]core.LinkSpan
	// 折叠/展开只差一个标记符，统一用 marker 构建。marker 不带尾随空格，
	// 由各分支的格式串/拼接统一补一个空格，避免 "[+]  2 tools" 双空格。
	marker := "[+]"
	if expanded {
		marker = "[-]"
	}
	summary := fmt.Sprintf("%s %d tools · %d msgs", marker, toolCount, sysCount)
	if sysCount == 0 {
		summary = fmt.Sprintf("%s %d tools", marker, toolCount)
	}
	if !expanded {
		for j := start; j <= end; j++ {
			if msgs[j].Meta != "" && msgs[j].Meta != "tool" {
				summary = marker + " " + msgs[j].Meta
				break
			}
		}
	}
	lines = append(lines, theme.DimStyle.Render(summary))
	links = append(links, nil)
	if expanded {
		// 左侧 2 列色带 + 内容，使组内成员连成一体。
		barStyled := theme.DimStyle.Render("│")
		prefix := "  " + barStyled + " "
		prefixW := core.VisibleWidth(prefix)
		innerW := width - prefixW
		if innerW < 1 {
			innerW = 1
		}
		for j := start; j <= end; j++ {
			memberLines, memberLinks := h.renderMessageCachedWithCache(msgs[j], theme, innerW, cache)
			trimmed, _ := trimBlankEdges(memberLines)
			for k, ln := range trimmed {
				lines = append(lines, prefix+ln)
				if memberLinks != nil && k < len(memberLinks) && memberLinks[k] != nil {
					// 成员行加色带前缀：链接列区间整体右移 prefixW。
					shifted := make([]core.LinkSpan, len(memberLinks[k]))
					for si, ls := range memberLinks[k] {
						shifted[si] = core.LinkSpan{StartCol: ls.StartCol + prefixW, EndCol: ls.EndCol + prefixW, URL: ls.URL}
					}
					links = append(links, shifted)
				} else {
					links = append(links, nil)
				}
			}
			if j < end {
				// 组成员之间的细色带连接，比空行更紧凑。
				lines = append(lines, "  "+barStyled)
				links = append(links, nil)
			}
		}
	}

	return lines, msgRange{
		startLine: 0, endLine: len(lines), msgIndex: start,
		toolGroup: true, groupFrom: start, groupTo: end,
	}, links
}

// renderMessageSeparator 在两条连续消息之间插入最小视觉分隔。
// 采用“时间线”密度：角色切换用单空行，同角色连续用极细点线，
// 连续工具调用之间不再插入额外分隔线（由工具组自身色带/卡片承担层次）。
func (h *ChatHistory) renderMessageSeparator(prev, curr ChatMessage, width int64, theme ChatHistoryTheme) []string {
	switch {
	// Tool ↔ Tool 连续工具调用：无额外分隔，由工具组色带/卡片自身承担层次。
	case prev.Role == RoleTool && curr.Role == RoleTool:
		return nil

	// 系统消息之间：单空行。
	case prev.Role == RoleSystem && curr.Role == RoleSystem:
		return []string{""}

	// 其他连续同角色消息（Assistant↔Assistant、User↔User）：八分之一宽点线。
	case prev.Role == curr.Role:
		eighth := int64(int(width) / 8)
		if eighth < 1 {
			eighth = 1
		}
		return []string{theme.DimStyle.Render(strings.Repeat(tuitheme.SymbolRuleEighthDots, int(eighth)))}

	// 其余任意角色切换：单空行。
	default:
		return []string{""}
	}
}

// trimBlankEdges removes leading and trailing blank (whitespace-only) lines
// from a rendered message block. Streamed assistant text often carries stray
// leading/trailing newlines which the markdown renderer turns into padded
// blank lines, inflating the vertical gap between turns. Internal blank lines
// (e.g. inside code blocks) are preserved.
//
// 返回裁剪后的行与起始偏移（供链接元数据同步裁剪）。
func trimBlankEdges(lines []string) ([]string, int) {
	start, end := 0, len(lines)
	for start < end && strings.TrimSpace(core.StripAnsi(lines[start])) == "" {
		start++
	}
	for end > start && strings.TrimSpace(core.StripAnsi(lines[end-1])) == "" {
		end--
	}
	return lines[start:end], start
}
