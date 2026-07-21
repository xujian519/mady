# TUI 模块文档与代码一致性修复

## 完成事项

### P0 级修复（已完成）
1. **TickInterval 注释修正** — `tui/tui.go` 中注释声称 16ms (~60fps)，实际代码默认 8ms (125fps)。已统一注释为实际值。
2. **LAYERS.md 目录结构全面更新** — 补充遗漏的 32+ 文件，覆盖 core (5)、terminal (1)、component (19)、chat (11)、stdio (1)、根包 (5) 各包。
3. **文档描述修正** — `用户界面 (tui).md` 中 TickInterval 描述从 "约 60fps" 改为 "8ms (~125fps)"。

### P1 级修复（已完成）
4. **Editor 子系统文档** — 在 LAYERS.md 中新增 Editor 5 文件拆分架构说明（editor.go + editor_edit/render/history/killring.go）。
5. **core.Every 移除决策记录** — 在 LAYERS.md 中新增 API 变更说明，指明 TUI.Every 替代方案。
6. **ChatApp 多文件架构文档** — 记录 ChatApp 14 文件拆分模式和 state.go FSM。

### P2 级修复（已完成）
7. **4 个大文件添加文档注释** — session_selector.go (545行)、skill_center.go (300行)、table.go (331行)、todo_panel.go (345行)。
8. **Cell-Level Rendering Model 文档** — 记录 core/cell*.go + sgr.go 的单元格级渲染子系统。

### 工具（已完成）
9. **LAYERS.md 自动同步检查脚本** — `tui/scripts/verify_layers.sh`，提取代码块中的 .go 文件名与磁盘实际文件对比。

## 修改文件清单
| 文件 | 变更类型 |
|------|---------|
| `tui/tui.go` | 注释修正（TickInterval 默认值描述） |
| `tui/LAYERS.md` | 目录结构重写 + 4 个新设计决策章节 |
| `tui/component/session_selector.go` | 添加文件头文档注释 |
| `tui/component/skill_center.go` | 添加文件头文档注释 |
| `tui/component/table.go` | 添加文件头文档注释 |
| `tui/component/todo_panel.go` | 添加文件头文档注释 |
| `tui/scripts/verify_layers.sh` | 新增 LAYERS.md 同步检查脚本 |
| `docs/decisions/AI_CHANGELOG.md` | 记录本次变更 |
| `.qoder/repowiki/.../用户界面 (tui).md` | TickInterval 描述修正 |

## 验证结果
- `go build ./tui/...` ✅
- `go vet ./tui/...` ✅
- `./tui/scripts/verify_layers.sh` ✅ 90 文件全部同步
