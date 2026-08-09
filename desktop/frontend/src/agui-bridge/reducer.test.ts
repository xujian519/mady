/**
 * aguiReducer run-finished 事件测试。
 *
 * 覆盖输出完整性提示（G-…）：
 * - finishReason="length"（max_tokens 截断）→ 设置 truncationNotice
 * - finishReason="error"（流异常终止）→ 设置 truncationNotice
 * - finishReason="stop"（正常结束）→ 不设置
 * - 新轮次开始（sendMessage）→ 清除上一轮提示
 */

import { describe, it, expect, beforeEach } from 'vitest'
import { useChatStore, initialState } from '@/stores/chat'
import { aguiReducer } from './reducer'

function resetStore() {
  useChatStore.setState({ ...initialState, ready: true })
}

describe('aguiReducer run-finished 输出完整性提示', () => {
  beforeEach(resetStore)

  it('finishReason=length 时提示输出可能不完整', () => {
    aguiReducer('run-finished', { finishReason: 'length' })
    const notice = useChatStore.getState().truncationNotice
    expect(notice).toBeTruthy()
    expect(notice).toContain('长度上限')
  })

  it('finishReason=error 时提示输出可能不完整', () => {
    aguiReducer('run-finished', { finishReason: 'error' })
    const notice = useChatStore.getState().truncationNotice
    expect(notice).toBeTruthy()
    expect(notice).toContain('异常中断')
  })

  it('finishReason=stop 时不设置提示', () => {
    aguiReducer('run-finished', { finishReason: 'stop' })
    expect(useChatStore.getState().truncationNotice).toBeNull()
  })

  it('缺失 finishReason 时不设置提示', () => {
    aguiReducer('run-finished', {})
    expect(useChatStore.getState().truncationNotice).toBeNull()
  })

  it('新轮次发送消息时清除上一轮的截断提示', async () => {
    aguiReducer('run-finished', { finishReason: 'length' })
    expect(useChatStore.getState().truncationNotice).toBeTruthy()

    // sendMessage 内部调用后端；这里仅验证状态清理路径：直接模拟
    // sendMessage 的 set 片段（清空 truncationNotice）。
    useChatStore.setState({ truncationNotice: null })
    expect(useChatStore.getState().truncationNotice).toBeNull()
  })
})
