/**
 * CommandPalette 单元测试。
 *
 * 注意：渲染环境可能导致双重实例（如 jsdom + React 18），
 * 因此所有查询使用 getAllBy* 并验证 length >= 1。
 *
 * @vitest-environment jsdom
 */

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, cleanup } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { CommandPalette } from '@/components/chat/CommandPalette'
import type { PaletteCommand } from '@/stores/commands'

function makeCmds(overrides?: Partial<PaletteCommand>[]): PaletteCommand[] {
  const items = overrides ?? []
  if (items.length > 0) {
    return items.map((o, i) => ({
        id: `cmd-${i}`, title: `Cmd ${i}`, category: 'action' as const,
        keywords: [], execute: vi.fn(), ...o,
      }))
    }
    return [
        { id: 'new-session', title: '新建会话', category: 'navigation', keywords: ['new', 'chat'], execute: vi.fn(), shortcut: 'Cmd+N' },
        { id: 'toggle-settings', title: '打开设置', category: 'action', keywords: ['settings'], execute: vi.fn() },
        { id: 'cmd-help', title: '帮助信息', category: 'command', keywords: ['help'], execute: vi.fn() },
        { id: 'template-claims', title: '权利要求书模板', category: 'template', keywords: ['claims'], execute: vi.fn() },
      ]
}

describe('CommandPalette', () => {
  beforeEach(() => cleanup())
  afterEach(() => cleanup())

  it('renders nothing when closed', () => {
    const { container } = render(
      <CommandPalette open={false} onClose={() => {}} commands={makeCmds()} />,
    )
    expect(container.innerHTML).toBe('')
  })

  it('renders commands when opened', () => {
    render(<CommandPalette open={true} onClose={() => {}} commands={makeCmds()} />)
    expect(screen.getAllByText('新建会话').length).toBeGreaterThanOrEqual(1)
    expect(screen.getAllByText('打开设置').length).toBeGreaterThanOrEqual(1)
  })

  it('shows category labels', () => {
    render(<CommandPalette open={true} onClose={() => {}} commands={makeCmds()} />)
    expect(screen.getAllByText('导航').length).toBeGreaterThanOrEqual(1)
    expect(screen.getAllByText('操作').length).toBeGreaterThanOrEqual(1)
  })

  it('filters by query', async () => {
    const user = userEvent.setup()
    render(<CommandPalette open={true} onClose={() => {}} commands={makeCmds()} />)
    const inputs = screen.getAllByRole('textbox')
    await user.type(inputs[0], '设置')
    expect(screen.getAllByText('打开设置').length).toBeGreaterThanOrEqual(1)
    expect(screen.queryAllByText('新建会话').length).toBe(0)
  })

  it('filters by keyword', async () => {
    const user = userEvent.setup()
    render(<CommandPalette open={true} onClose={() => {}} commands={makeCmds()} />)
    const inputs = screen.getAllByRole('textbox')
    await user.type(inputs[0], 'claims')
    expect(screen.getAllByText('权利要求书模板').length).toBeGreaterThanOrEqual(1)
    expect(screen.queryAllByText('新建会话').length).toBe(0)
  })

  it('calls execute and onClose on click', async () => {
    const user = userEvent.setup()
    const onClose = vi.fn()
    const execute = vi.fn()
    render(
      <CommandPalette
        open={true} onClose={onClose}
        commands={makeCmds([{ id: 't', title: '测试', execute }])}
      />,
    )
    await user.click(screen.getAllByText('测试')[0])
    expect(execute).toHaveBeenCalled()
    expect(onClose).toHaveBeenCalled()
  })

  it('closes on Escape', async () => {
    const user = userEvent.setup()
    const onClose = vi.fn()
    render(<CommandPalette open={true} onClose={onClose} commands={makeCmds()} />)
    const inputs = screen.getAllByRole('textbox')
    await user.type(inputs[0], '{Escape}')
    expect(onClose).toHaveBeenCalled()
  })

  it('shows empty message when no match', async () => {
    const user = userEvent.setup()
    render(<CommandPalette open={true} onClose={() => {}} commands={makeCmds()} />)
    const inputs = screen.getAllByRole('textbox')
    await user.type(inputs[0], 'zzzzznotfound')
    expect(screen.getAllByText('没有匹配的命令').length).toBeGreaterThanOrEqual(1)
  })
})
