/**
 * SettingsPanel 组件特征测试（M59 护栏）。
 *
 * 覆盖：对话框渲染与分区导航、主题模式切换写回 store、分区内容切换、
 * 关于区版本号（来自 health）、关闭回调。MCP/模型/主题画廊等子面板以替身隔离。
 *
 * @vitest-environment jsdom
 */

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, fireEvent, cleanup, waitFor } from '@testing-library/react'

const themeMock = vi.hoisted(() => ({ setMode: vi.fn() }))

vi.mock('@/theme/tokens', () => ({
  useTheme: () => ({ setMode: themeMock.setMode }),
}))
vi.mock('@/lib/backend', () => ({
  getAISettings: vi.fn(async () => ({ provider: 'deepseek', model: 'deepseek-chat' })),
  setAISettings: vi.fn(async () => {}),
  checkUpdate: vi.fn(async () => null),
  health: vi.fn(async () => ({ version: '9.9.9' })),
}))
vi.mock('@/components/McpServersSettings', () => ({
  McpServersSettings: () => <div data-testid="mcp-settings" />,
}))
vi.mock('@/components/ModelSettings', () => ({
  ModelSettings: () => <div data-testid="model-settings" />,
}))
vi.mock('@/components/ThemeGallery', () => ({
  ThemeGallery: () => <div data-testid="theme-gallery" />,
}))

import { SettingsPanel } from '@/components/SettingsPanel'
import { useSettingsStore } from '@/stores/settings'

beforeEach(() => {
  cleanup()
  themeMock.setMode.mockClear()
  useSettingsStore.setState(useSettingsStore.getInitialState(), true)
})

afterEach(() => {
  cleanup()
})

describe('SettingsPanel', () => {
  it('渲染对话框、标题与五个分区导航', () => {
    render(<SettingsPanel onClose={() => {}} />)

    expect(screen.getByRole('dialog', { name: '设置' })).toBeInTheDocument()
    for (const label of ['外观', 'AI 服务', '模型与推理', 'MCP 服务器', '关于']) {
      expect(screen.getByRole('button', { name: label })).toBeInTheDocument()
    }
    // 默认停在外观区
    expect(screen.getByRole('button', { name: '亮色' })).toBeInTheDocument()
    expect(screen.getByTestId('theme-gallery')).toBeInTheDocument()
  })

  it('切换主题模式写回 settings store 并调用 setMode', () => {
    render(<SettingsPanel onClose={() => {}} />)

    fireEvent.click(screen.getByRole('button', { name: '深色' }))

    expect(themeMock.setMode).toHaveBeenCalledWith('dark')
    expect(useSettingsStore.getState().themeMode).toBe('dark')
  })

  it('点击分区导航切换到对应内容', () => {
    render(<SettingsPanel onClose={() => {}} />)

    fireEvent.click(screen.getByRole('button', { name: 'MCP 服务器' }))
    expect(screen.getByTestId('mcp-settings')).toBeInTheDocument()
    expect(screen.queryByTestId('theme-gallery')).toBeNull()

    fireEvent.click(screen.getByRole('button', { name: '模型与推理' }))
    expect(screen.getByTestId('model-settings')).toBeInTheDocument()
  })

  it('关于区展示后端 health 返回的版本号', async () => {
    render(<SettingsPanel onClose={() => {}} />)

    fireEvent.click(screen.getByRole('button', { name: '关于' }))

    await waitFor(() => {
      expect(screen.getByText('9.9.9')).toBeInTheDocument()
    })
  })

  it('关闭按钮触发 onClose', () => {
    const onClose = vi.fn()
    render(<SettingsPanel onClose={onClose} />)

    fireEvent.click(screen.getByRole('button', { name: '关闭设置' }))
    expect(onClose).toHaveBeenCalledTimes(1)
  })
})
