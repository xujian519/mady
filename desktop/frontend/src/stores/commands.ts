/**
 * CommandPalette 命令注册表。
 *
 * 定义 ⌘K 面板可搜索的全部命令，覆盖导航、模板、技能、
 * 斜杠命令和全局操作。命令的 execute 回调引用 Zustand store
 * action（延迟求值），避免在模块加载时产生循环依赖。
 */

export type CommandCategory = 'navigation' | 'template' | 'skill' | 'command' | 'action'

export interface PaletteCommand {
  id: string
  title: string
  category: CommandCategory
  shortcut?: string
  keywords: string[]
  icon?: string
  /** 执行时的回调工厂；在命令被选中时调用。 */
  execute: () => void
}

// ── 命令构建器 ────────────────────────────────────

/**
 * 创建简单的无参操作命令。
 */
function action(id: string, title: string, icon: string, keywords: string[], fn: () => void, shortcut?: string): PaletteCommand {
  return { id, title, category: 'action', icon, keywords, execute: fn, shortcut }
}

function nav(id: string, title: string, icon: string, keywords: string[], fn: () => void, shortcut?: string): PaletteCommand {
  return { id, title, category: 'navigation', icon, keywords, execute: fn, shortcut }
}

/**
 * 从 SlashCommand 数组构建 PaletteCommand 列表。
 * commands 和 settings 引用由 ChatView 通过回调注入。
 */
export function buildCommands(opts: {
  toggleSettings: () => void
  toggleSidebar: () => void
  setTheme: (mode: 'light' | 'dark') => void
  clearChat: () => void
  exportChat: () => void
  toggleFocusMode: () => void
  openTemplate: (name: string) => void
}): PaletteCommand[] {
  return [
    // ── 导航 ──
    nav('new-session', '新建会话', 'Plus', ['new', 'chat', '对话'], () => {
      // F-I3：新建会话 = 清空当前对话（与 clearChat 同语义）
      opts.clearChat()
    }, 'Cmd+N'),
    nav('toggle-sidebar', '切换侧栏', 'PanelLeft', ['sidebar', 'panel', 'toggle'], opts.toggleSidebar, 'Cmd+B'),
    nav('toggle-focus', '专注模式', 'ScanEye', ['focus', 'mode', 'layout'], opts.toggleFocusMode),

    // ── 模板 ──
    { id: 'template-claims', title: '权利要求书模板', category: 'template', keywords: ['claims', '权利要求', '撰写'], icon: 'FileText', execute: () => opts.openTemplate('claims') },
    { id: 'template-spec', title: '说明书模板', category: 'template', keywords: ['specification', '说明书', '撰写'], icon: 'FileText', execute: () => opts.openTemplate('spec') },
    { id: 'template-oa', title: 'OA 答复函模板', category: 'template', keywords: ['oa', '答复', '审查意见'], icon: 'FileText', execute: () => opts.openTemplate('oa-response') },

    // ── 操作 ──
    action('clear-context', '清除上下文', 'Eraser', ['clear', 'clean', '重置'], opts.clearChat),
    action('export-session', '导出会话', 'Download', ['export', 'download', 'json'], opts.exportChat),
    action('toggle-settings', '打开设置', 'Settings', ['settings', 'preferences', '配置'], opts.toggleSettings),
    action('theme-light', '浅色模式', 'Sun', ['light', 'theme', '浅色'], () => opts.setTheme('light')),
    action('theme-dark', '深色模式', 'Moon', ['dark', 'theme', '深色'], () => opts.setTheme('dark')),

    // ── 斜杠命令（从 SlashCommand 继承，仅列出 local UI 类） ──
    { id: 'cmd-help', title: '帮助信息', category: 'command', keywords: ['help', '帮助', 'usage'], icon: 'HelpCircle', execute: () => {} },
  ]
}
