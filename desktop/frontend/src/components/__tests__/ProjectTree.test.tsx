/**
 * ProjectTree 特征测试（M54 拆解前护栏）。
 *
 * 目的：在拆分 ProjectTree.tsx（879 行）前固定当前可见行为——根目录排序、
 * 加载/失败/空三种状态、文件夹懒加载。断言只依赖用户可见文本与后端调用，
 * 不依赖内部 reducer 结构。
 */

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, cleanup, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { FileEntry } from '@/lib/backend'

vi.mock('@/lib/backend', () => ({
  listDirectory: vi.fn(),
  createFolder: vi.fn(),
  renameFolder: vi.fn(),
  deleteEntry: vi.fn(),
  writeFile: vi.fn(),
  listProjects: vi.fn(async () => []),
  getCurrentProject: vi.fn(async () => null),
  createProjectFolder: vi.fn(),
  selectProjectFolder: vi.fn(),
  switchProject: vi.fn(),
}))

import { listDirectory } from '@/lib/backend'
import { ProjectTree } from '@/components/ProjectTree'
import { useProjectStore } from '@/stores/project'

const listDir = vi.mocked(listDirectory)

/** 构造一个后端文件项（大小与时间不影响渲染顺序）。 */
function entry(name: string, isDir = false): FileEntry {
  return { name, isDir, size: 0, modTime: 0 }
}

/** 等待根目录加载完成（出现任一文件名）。 */
async function waitForTree(name: string) {
  return screen.findByText(name)
}

beforeEach(() => {
  cleanup()
  listDir.mockReset()
  useProjectStore.setState({ current: null, projects: [], loading: false, error: null })
})

afterEach(() => {
  cleanup()
})

describe('ProjectTree', () => {
  it('根目录按「文件夹优先 + 名称升序」渲染', async () => {
    listDir.mockResolvedValue([
      entry('b.md'),
      entry('alpha', true),
      entry('Beta', true),
      entry('a.txt'),
    ])

    const { container } = render(<ProjectTree />)
    await waitForTree('alpha')

    const text = container.textContent ?? ''
    const order = ['alpha', 'Beta', 'a.txt', 'b.md'].map((n) => text.indexOf(n))
    expect(order.every((i) => i >= 0)).toBe(true)
    expect([...order].sort((x, y) => x - y)).toEqual(order)
  })

  it('根目录加载中显示占位文案', () => {
    listDir.mockReturnValue(new Promise<FileEntry[]>(() => {}))

    render(<ProjectTree />)
    expect(screen.getByText('加载中…')).toBeInTheDocument()
  })

  it('加载失败显示错误文案与重试入口', async () => {
    listDir.mockRejectedValue(new Error('目录不可读'))

    render(<ProjectTree />)
    expect(await screen.findByText(/加载失败: 目录不可读/)).toBeInTheDocument()
    expect(screen.getByText('重试')).toBeInTheDocument()
  })

  it('空目录显示「项目为空」', async () => {
    listDir.mockResolvedValue([])

    render(<ProjectTree />)
    expect(await screen.findByText('项目为空')).toBeInTheDocument()
  })

  it('点击文件夹箭头按路径懒加载子项', async () => {
    listDir.mockImplementation(async (relPath?: string) => {
      if (!relPath) return [entry('alpha', true)]
      if (relPath === 'alpha') return [entry('inner.md')]
      return []
    })

    render(<ProjectTree />)
    const folderLabel = await waitForTree('alpha')
    const row = folderLabel.closest('div')!
    const toggle = row.querySelector('button')!

    await userEvent.click(toggle)

    await waitFor(() => {
      expect(listDir).toHaveBeenCalledWith('alpha')
    })
    expect(await screen.findByText('inner.md')).toBeInTheDocument()
  })
})
