/**
 * A2UI Theme Properties → Tailwind 类名转换。
 *
 * BasicCatalog 定义的三个主题属性：
 *   primaryColor — 主色（如 "#5856D6"），映射到 Tailwind accent / ring / border
 *   iconUrl — 图标 URL（组件级别处理，此模块不处理）
 *   agentDisplayName — Agent 显示名称
 */

/** 解析主题主色，返回 Tailwind 类名映射。 */
export function resolveThemeTheme(theme: Record<string, unknown>): Record<string, string> {
  const classes: Record<string, string> = {}

  const primaryColor = theme.primaryColor as string | undefined
  if (primaryColor) {
    // 设置 CSS 变量，子组件通过 style 引用
    classes['--a2ui-primary'] = primaryColor
  }

  const agentName = theme.agentDisplayName as string | undefined
  if (agentName) {
    classes['--a2ui-agent-name'] = agentName
  }

  return classes
}

/** 将主题属性映射到内联样式。 */
export function themeToStyle(theme: Record<string, unknown>): Record<string, string> {
  const style: Record<string, string> = {}
  const resolved = resolveThemeTheme(theme)
  for (const [key, val] of Object.entries(resolved)) {
    style[key] = val
  }
  return style
}

/** 获取主题中的 agent 显示名称。 */
export function agentDisplayName(theme: Record<string, unknown>): string {
  return (theme.agentDisplayName as string) ?? 'Agent'
}

/** 获取主题中的 icon URL。 */
export function iconUrl(theme: Record<string, unknown>): string | undefined {
  return theme.iconUrl as string | undefined
}
