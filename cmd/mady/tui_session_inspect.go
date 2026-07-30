package main

// tui_session_inspect.go 集中实现"只读查看后端已注入能力"的 slash 命令处理器。
//
// 背景：evidence ledger / filecheckpoint / memory 三个子系统在 framework.go
// 启动时已注入 BaseConfig.Extensions，但 TUI 此前没有任何查看入口。
// 本文件通过 fc 上保存的扩展引用，让用户能在对话中直接查看：
//   - /ledger     — 当前轮工具调用证据（Receipt 列表，BeforeTurn 自动重置）
//   - /snapshots  — 已记录的文件快照（每轮写入工具前的文件状态）
//   - /undo [turn]— 回退到指定轮的文件状态（RestoreAndTrim，原子操作）
//   - /memory     — 跨三层（User/Session/LongTerm）查看长期记忆条目
//
// 设计原则：
//   - 只读命令（ledger/snapshots/memory）无副作用，可直接调用
//   - /undo 涉及文件写入，但属于用户显式触发，执行后打印影响范围
//   - 所有命令在扩展不可用时降级为系统提示，不返回错误
//   - 参数解析统一走 parseSlashSubcommand / parseSlashRest（与 /plan /review 一致）

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/xujian519/mady/memory"
)

// maxMemoryEntries 限制 /memory 单次显示的总条数，防止记忆库膨胀时刷屏。
const maxMemoryEntries = 20

// maxSnapshotPreview 限制 /snapshots 显示的历史轮数，防止长会话刷屏。
const maxSnapshotPreview = 20

// truncateRunes 按 rune 数（而非字节）截断字符串，避免在多字节字符中间切断产生无效 UTF-8。
// maxRunes 是返回字符串（含省略号）的最大 rune 数。
func truncateRunes(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	// 预留 1 个 rune 给省略号
	if maxRunes <= 1 {
		return "…"
	}
	return string(r[:maxRunes-1]) + "…"
}

// handleLedgerCommand 实现 /ledger：展示当前轮的工具调用证据账本。
//
// 数据源：evidence.Extension.Ledger().Snapshot()。Ledger 在每次 BeforeTurn 时
// 被 Reset，因此 /ledger 反映的是"本轮已执行的工具调用"。若本轮尚无工具调用，
// 提示用户先发起一轮对话。
func (s *tuiSession) handleLedgerCommand() { //nolint:unused // kept for test coverage; replaced by EvidenceOverlay
	if s.fc == nil || s.fc.EvidenceExt == nil {
		s.app.PrintSystem("⚠️ 证据账本未启用（EvidenceExt 未注入）")
		return
	}
	ledger := s.fc.EvidenceExt.Ledger()
	if ledger == nil || ledger.Len() == 0 {
		s.app.PrintSystem("📋 本轮暂无工具调用证据。\n\n" +
			"提示：证据账本在每轮对话开始时重置。发起一轮包含工具调用的对话后再查看。")
		return
	}
	receipts := ledger.Snapshot()
	var b strings.Builder
	fmt.Fprintf(&b, "📋 本轮工具调用证据（共 %d 条）\n", len(receipts))
	for i, r := range receipts {
		status := "✓"
		if !r.Success {
			status = "✗"
		}
		fmt.Fprintf(&b, "\n  %d. %s %s", i+1, status, r.ToolName)
		if r.DurationMs > 0 {
			fmt.Fprintf(&b, "  · %dms", r.DurationMs)
		}
		if r.Write {
			b.WriteString("  · [write]")
		} else if r.Read {
			b.WriteString("  · [read]")
		}
		if len(r.Paths) > 0 {
			fmt.Fprintf(&b, "\n     paths: %s", strings.Join(r.Paths, ", "))
		}
		if r.Command != "" {
			fmt.Fprintf(&b, "\n     cmd: %s", truncateRunes(r.Command, 80))
		}
	}
	s.app.PrintSystem(b.String())
}

// handleSnapshotsCommand 实现 /snapshots：列出已记录的文件快照历史。
//
// 数据源：filecheckpoint.Store.List()，返回每轮的用户提示、时间戳、触及文件列表。
// 用于让用户在 /undo 前预览可回退的目标轮。
func (s *tuiSession) handleSnapshotsCommand() {
	if s.fc == nil || s.fc.FileCheckpointExt == nil {
		s.app.PrintSystem("⚠️ 文件快照未启用（FileCheckpointExt 未注入）")
		return
	}
	store := s.fc.FileCheckpointExt.Store()
	metas := store.List()
	if len(metas) == 0 {
		s.app.PrintSystem("📸 暂无文件快照。\n\n" +
			"提示：快照在写入工具（edit/write_file/patch/delete/move）执行前自动记录。\n" +
			"让 Agent 修改文件后再查看，然后用 /undo <turn> 可回退。")
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "📸 文件快照历史（共 %d 轮）\n", len(metas))
	// 仅展示最近 N 轮，防止长会话刷屏
	start := 0
	if len(metas) > maxSnapshotPreview {
		start = len(metas) - maxSnapshotPreview
		fmt.Fprintf(&b, "（仅显示最近 %d 轮，更早的已省略）\n", maxSnapshotPreview)
	}
	for i := start; i < len(metas); i++ {
		m := metas[i]
		prompt := truncateRunes(m.Prompt, 60)
		if prompt == "" {
			prompt = "(无提示)"
		}
		fmt.Fprintf(&b, "\n  轮 %d  · %s", m.Turn, m.Time.Format("2006-01-02 15:04:05"))
		fmt.Fprintf(&b, "\n     %s", prompt)
		if len(m.Paths) > 0 {
			fmt.Fprintf(&b, "\n     文件: %s", strings.Join(m.Paths, ", "))
		} else {
			b.WriteString("\n     文件: (本轮无写入)")
		}
	}
	b.WriteString("\n\n用法：/undo <轮号> — 回退到指定轮的文件状态")
	s.app.PrintSystem(b.String())
}

// handleUndoCommand 实现 /undo <turn>：回退到指定轮的文件状态。
//
// 参数 arg 由注册处通过 parseSlashSubcommand 解析后传入（与其他 slash 命令一致）。
// 空字符串表示无参数，显示用法提示。
//
// 数据源：filecheckpoint.Store.RestoreAndTrim(turn)。该操作原子执行：
//   - 把目标轮记录的文件内容写回磁盘（新建文件会被删除）
//   - 删除目标轮之后的所有快照记录
//
// 安全说明：
//   - 仅回退文件checkpoint 追踪的文件（edit/write_file/patch/delete/move 触及的）
//   - bash 副作用不追踪（无法回退）
//   - 操作不可逆（快照已被裁剪），但新对话仍可继续
func (s *tuiSession) handleUndoCommand(arg string) {
	if s.fc == nil || s.fc.FileCheckpointExt == nil {
		s.app.PrintSystem("⚠️ 文件快照未启用（FileCheckpointExt 未注入）")
		return
	}
	if arg == "" {
		s.app.PrintSystem("↩️ 文件回退用法：\n\n" +
			"  /undo <轮号>   — 回退到指定轮的文件状态\n" +
			"  /snapshots     — 先查看可回退的轮号\n\n" +
			"说明：仅回退 edit/write_file/patch/delete/move 触及的文件；\n" +
			"bash 副作用不追踪。操作不可逆（会裁剪目标轮之后的快照）。")
		return
	}
	turn, err := strconv.ParseInt(arg, 10, 64)
	if err != nil {
		s.app.PrintSystem(fmt.Sprintf("❌ 无效的轮号 %q：请输入数字（参考 /snapshots 输出）", arg))
		return
	}
	// RestoreAndTrim 原子执行 restore + trim，并返回被回退轮的 Meta（供 UX 提示）。
	// 内部持锁全程，避免 Restore 单独释放锁期间并发 BeginTurn 的 TOCTOU 问题。
	store := s.fc.FileCheckpointExt.Store()
	meta, err := store.RestoreAndTrim(turn)
	if err != nil {
		s.app.PrintSystem(fmt.Sprintf("❌ 回退失败：%v", err))
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "↩️ 已回退到轮 %d 的文件状态\n", turn)
	if meta.Prompt != "" {
		fmt.Fprintf(&b, "  原提示：%s\n", truncateRunes(meta.Prompt, 60))
	}
	if len(meta.Paths) > 0 {
		b.WriteString("  涉及文件：\n")
		for _, p := range meta.Paths {
			fmt.Fprintf(&b, "    · %s\n", p)
		}
	}
	b.WriteString("\n后续轮的快照已被裁剪。可继续对话或 /snapshots 查看剩余历史。")
	s.app.PrintSystem(b.String())
}

// handleMemoryCommand 实现 /memory [query]：查看跨三层的长期记忆条目。
//
// 参数 query 由注册处通过 parseSlashRest 解析后传入（保留多词参数）。
// 空字符串表示浏览模式，非空表示语义检索模式。
//
// 数据源：memory.Manager.Store().List()，跨 LayerUser / LayerSession / LayerLongTerm 三层。
// 通过 ListOptions.UserID 在存储层做 scope 过滤，避免跨用户泄漏。
func (s *tuiSession) handleMemoryCommand(query string) {
	if s.fc == nil || s.fc.MemoryManager == nil {
		s.app.PrintSystem("⚠️ 长期记忆未启用（MemoryManager 未初始化）")
		return
	}

	mgr := s.fc.MemoryManager
	var b strings.Builder

	if query != "" {
		// 语义检索模式：SearchAllLayers 已内置混合检索（语义+关键词+RRF）
		results, err := mgr.SearchAllLayers(s.ctx, query, maxMemoryEntries)
		if err != nil {
			s.app.PrintSystem(fmt.Sprintf("❌ 记忆检索失败：%v", err))
			return
		}
		fmt.Fprintf(&b, "🧠 记忆检索：%q（命中 %d 条）\n", query, len(results))
		for i, r := range results {
			if i >= maxMemoryEntries {
				break
			}
			renderMemoryEntry(&b, i+1, r.Entry, r.Composite)
		}
	} else {
		// 全量浏览模式：跨三层列出最近条目，按当前用户过滤（存储层完成）
		store := mgr.Store()
		userID := stableUserID()
		total := 0
		fmt.Fprintf(&b, "🧠 长期记忆（跨三层，最近 %d 条）\n", maxMemoryEntries)
		for _, layer := range memory.ValidLayers() {
			entries, err := store.List(s.ctx, layer, memory.ListOptions{
				Limit:  maxMemoryEntries,
				UserID: userID, // 存储层过滤，避免跨用户泄漏
			})
			if err != nil || len(entries) == 0 {
				continue
			}
			fmt.Fprintf(&b, "\n— 层：%s（%d 条）—\n", layerDisplayName(layer), len(entries))
			for _, e := range entries {
				total++
				if total > maxMemoryEntries {
					break
				}
				renderMemoryEntry(&b, total, e, 0)
			}
			if total >= maxMemoryEntries {
				break
			}
		}
		if total == 0 {
			b.WriteString("\n（暂无记忆。Agent 会在对话中通过 remember 工具或自动提取记录偏好与事实。）")
		}
	}
	s.app.PrintSystem(b.String())
}

// renderMemoryEntry 格式化一条记忆到 builder。
// score 为 0 时表示浏览模式（不显示评分），>0 时表示检索模式。
func renderMemoryEntry(b *strings.Builder, idx int, e memory.MemoryEntry, score float64) {
	content := truncateRunes(e.Content, 120)
	fmt.Fprintf(b, "\n  %d. [%s] %s", idx, layerDisplayName(e.Layer), content)
	if score > 0 {
		fmt.Fprintf(b, "  · score=%.2f", score)
	}
	if e.Importance > 0 {
		fmt.Fprintf(b, "  · 重要度=%.2f", e.Importance)
	}
	fmt.Fprintf(b, "\n     更新：%s", e.UpdatedAt.Format("2006-01-02 15:04"))
	if e.Scope.ProjectID != "" {
		fmt.Fprintf(b, "  · 案件：%s", e.Scope.ProjectID)
	}
}

// layerDisplayName 返回记忆层的中文显示名。
func layerDisplayName(layer memory.MemoryLayer) string {
	switch layer {
	case memory.LayerUser:
		return "用户偏好"
	case memory.LayerSession:
		return "会话"
	case memory.LayerLongTerm:
		return "长期"
	default:
		return string(layer)
	}
}
