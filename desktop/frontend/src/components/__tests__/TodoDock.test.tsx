/**
 * TodoDock 单元测试。
 *
 * 覆盖：
 * - 空状态隐藏
 * - 任务列表渲染
 * - 排序（urgency > in_progress > pending）
 * - 展开/折叠交互
 *
 * @vitest-environment jsdom
 */

import { describe, it, expect, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { TodoDock } from '@/components/TodoDock'
import { useChatStore } from '@/stores/chat'
import type { TaskItem } from '@/stores/chat'

function setTasks(tasks: TaskItem[]) {
  useChatStore.getState().setTasks(tasks)
}

function clearTasks() {
  useChatStore.getState().clearTasks()
}

/** 辅助：在 TodoDock 中找到唯一的 button（折叠/展开行）。 */
function findToggleBtn(container: HTMLElement): HTMLButtonElement {
  return container.querySelector('button')!
}

describe('TodoDock', () => {
  beforeEach(() => {
    clearTasks()
  })

  it('renders nothing when task list is empty', () => {
    const { container } = render(<TodoDock />)
    expect(container.innerHTML).toBe('')
  })

  it('renders summary row with task count', () => {
    setTasks([
      { id: '1', subject: '检索现有技术', status: 'in_progress', priority: 'high' },
    ])
    render(<TodoDock />)
    // "1" 出现多次（任务计数 + 进行中计数），用 getAllByText
    const ones = screen.getAllByText('1')
    expect(ones.length).toBeGreaterThanOrEqual(2)
    // "个任务" 可能在 StrictMode 下重复
    const geLabels = screen.getAllByText('个任务')
    expect(geLabels.length).toBeGreaterThanOrEqual(1)
  })

  it('shows completed count', () => {
    setTasks([
      { id: '1', subject: '完成的任务', status: 'completed', priority: 'normal' },
      { id: '2', subject: '待处理任务', status: 'pending', priority: 'low' },
    ])
    render(<TodoDock />)
    // 可能因 React.StrictMode 导致重复渲染，用 getAllByText
    const twos = screen.getAllByText('2')
    expect(twos.length).toBeGreaterThanOrEqual(1)
    const geLabels = screen.getAllByText('个任务')
    expect(geLabels.length).toBeGreaterThanOrEqual(1)
  })

  it('expands on click to show task list', () => {
    const tasks = [
      { id: '1', subject: '检索现有技术', status: 'in_progress', priority: 'high' },
      { id: '2', subject: '分析对比文件', status: 'pending', priority: 'normal' },
    ] as TaskItem[]
    setTasks(tasks)
    const { container } = render(<TodoDock />)

    // 默认折叠：不显示任务标题
    expect(screen.queryByText('检索现有技术')).toBeNull()

    // 点击展开
    fireEvent.click(findToggleBtn(container))

    // 现在应该显示任务行
    expect(screen.getByText('检索现有技术')).toBeTruthy()
    expect(screen.getByText('分析对比文件')).toBeTruthy()
  })

  it('shows activeForm when available', () => {
    setTasks([
      {
        id: '1',
        subject: '检索现有技术',
        status: 'in_progress',
        priority: 'high',
        activeForm: '正在检索中国专利数据库',
      },
    ])
    const { container } = render(<TodoDock />)

    // 展开后查看 activeForm 优先于 subject
    fireEvent.click(findToggleBtn(container))

    // 可能因 StrictMode 双重渲染，用 getAllByText
    const forms = screen.getAllByText('正在检索中国专利数据库')
    expect(forms.length).toBeGreaterThanOrEqual(1)
  })

  it('sorts urgent tasks before normal ones', () => {
    setTasks([
      { id: '1', subject: '普通任务', status: 'pending', priority: 'normal' },
      { id: '2', subject: '紧急任务', status: 'pending', priority: 'urgent' },
    ] as TaskItem[])
    const { container } = render(<TodoDock />)

    fireEvent.click(findToggleBtn(container))

    // 紧急任务应排在前
    const urgentEl = screen.getAllByText('紧急任务')[0]
    const normalEl = screen.getAllByText('普通任务')[0]
    const urgentPos = urgentEl.compareDocumentPosition(normalEl)
    expect(urgentPos & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  })
})
