/**
 * Composer 组件特征测试（M58 护栏）。
 *
 * 覆盖输入区四条主路径：空输入禁用、Enter/点击发送、Shift+Enter 换行、
 * 运行中禁用；以及本地 UI 命令拦截（不落到 Agent）。斜杠命令菜单与导出
 * 下载等重依赖替换为替身。
 *
 * @vitest-environment jsdom
 */

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, fireEvent, cleanup } from '@testing-library/react'

vi.mock('@/components/chat/SlashCommandMenu', () => ({
  SlashCommandMenu: () => null,
}))
vi.mock('@/lib/sessionExport', () => ({
  exportSession: vi.fn(() => ''),
  downloadSession: vi.fn(),
  generateExportFilename: vi.fn(() => 'export.md'),
}))
vi.mock('@/lib/backend', () => ({
  savePastedImage: vi.fn(async () => ''),
}))

import { Composer } from '@/components/chat/Composer'
import { useChatStore } from '@/stores/chat'
import { useSettingsStore } from '@/stores/settings'

const sendMessage = vi.fn()

/** 输入框（唯一 textarea）。 */
function input(): HTMLTextAreaElement {
  return screen.getByRole('textbox') as HTMLTextAreaElement
}

/** 发送按钮（title 固定为「发送」）。 */
function sendBtn(): HTMLButtonElement {
  return screen.getByTitle('发送') as HTMLButtonElement
}

beforeEach(() => {
  cleanup()
  localStorage.clear()
  sendMessage.mockClear()
  useChatStore.setState({ ...useChatStore.getInitialState(), sendMessage })
})

afterEach(() => {
  cleanup()
  localStorage.clear()
})

describe('Composer', () => {
  it('空输入时发送按钮禁用', () => {
    render(<Composer />)
    expect(sendBtn()).toBeDisabled()
  })

  it('输入后点击发送并清空输入', () => {
    render(<Composer />)
    fireEvent.change(input(), { target: { value: '帮我分析新颖性' } })
    expect(sendBtn()).toBeEnabled()

    fireEvent.click(sendBtn())

    expect(sendMessage).toHaveBeenCalledTimes(1)
    expect(sendMessage).toHaveBeenCalledWith('帮我分析新颖性')
    expect(input().value).toBe('')
  })

  it('Enter 发送、Shift+Enter 不发送', () => {
    render(<Composer />)
    fireEvent.change(input(), { target: { value: '第一行' } })

    fireEvent.keyDown(input(), { key: 'Enter', shiftKey: true })
    expect(sendMessage).not.toHaveBeenCalled()
    expect(input().value).toBe('第一行')

    fireEvent.keyDown(input(), { key: 'Enter' })
    expect(sendMessage).toHaveBeenCalledWith('第一行')
  })

  it('运行中禁用输入并显示等待文案', () => {
    useChatStore.setState({ running: true })

    render(<Composer />)
    expect(input()).toBeDisabled()
    expect(input().placeholder).toBe('等待回复…')
    expect(sendBtn()).toBeDisabled()
  })

  it('本地 UI 命令前端拦截，不发送到 Agent', () => {
    render(<Composer />)
    fireEvent.change(input(), { target: { value: '/layout focus' } })
    fireEvent.keyDown(input(), { key: 'Enter' })

    expect(sendMessage).not.toHaveBeenCalled()
    expect(useSettingsStore.getState().layout).toBe('focus')
    expect(input().value).toBe('')
  })

  it('空白输入不发送', () => {
    render(<Composer />)
    fireEvent.change(input(), { target: { value: '   ' } })
    fireEvent.keyDown(input(), { key: 'Enter' })
    expect(sendMessage).not.toHaveBeenCalled()
  })
})
