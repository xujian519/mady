/**
 * ChatView 纯函数特征测试（M54 拆解前护栏）。
 *
 * ChatView.tsx（822 行）依赖 pdf.js 等重组件，jsdom 下直接导入会在
 * pdfjs 的 DOMMatrix 探测处失败，故先 mock 'pdfjs-dist'，再对导出的纯函数
 * （历史消息映射、步骤名/Token 格式化、行数与高度估算、活动内容判定）做断言。
 * 这些函数正是拆分时最先会被抽出的部分，断言即拆分护栏。
 */

import { describe, it, expect, vi } from 'vitest'

vi.mock('pdfjs-dist', () => ({ GlobalWorkerOptions: {}, getDocument: vi.fn() }))

import {
  mapHistoryMessage,
  formatStepName,
  formatTokens,
  estimateLines,
  itemHeight,
  hasActiveContent,
} from '@/components/chat/ChatView'
import type { Message, ToolCall } from '@/stores/chat'

describe('mapHistoryMessage', () => {
  it('保留 Go 侧已有 id 并归一 role', () => {
    const got = mapHistoryMessage({ id: 'm1', role: 'user', content: '你好', timestamp: 123 }, 0)
    expect(got).toMatchObject({ id: 'm1', role: 'user', content: '你好', timestamp: 123 })
    expect(got.thinking).toBeUndefined()
  })

  it('缺 id 时用下标生成稳定 fallback', () => {
    expect(mapHistoryMessage({}, 7).id).toBe('hist-7')
    expect(mapHistoryMessage({ id: '' }, 3).id).toBe('hist-3')
  })

  it('非 user 角色统一归一为 agent', () => {
    expect(mapHistoryMessage({ role: 'assistant' }, 0).role).toBe('agent')
    expect(mapHistoryMessage({ role: 'system' }, 0).role).toBe('agent')
  })

  it('content 非字符串置空、非法 timestamp 归零', () => {
    const got = mapHistoryMessage({ role: 'user', content: 42, timestamp: Number.NaN }, 0)
    expect(got.content).toBe('')
    expect(got.timestamp).toBe(0)
  })

  it('仅在 thinking 为字符串时保留', () => {
    expect(mapHistoryMessage({ thinking: '思考中' }, 0).thinking).toBe('思考中')
    expect(mapHistoryMessage({ thinking: 1 }, 0).thinking).toBeUndefined()
  })
})

describe('formatStepName', () => {
  it('turn_N 归一为统一文案', () => {
    expect(formatStepName('turn_3')).toBe('推理与工具调用进行中')
  })

  it('其他步骤名原样返回', () => {
    expect(formatStepName('retrieval')).toBe('retrieval')
    expect(formatStepName('turn_x')).toBe('turn_x')
  })
})

describe('formatTokens', () => {
  it('千位以下原样输出', () => {
    expect(formatTokens(0)).toBe('0')
    expect(formatTokens(999)).toBe('999')
  })

  it('千位及以上保留一位小数并加 k', () => {
    expect(formatTokens(1000)).toBe('1.0k')
    expect(formatTokens(12500)).toBe('12.5k')
  })
})

describe('estimateLines', () => {
  it('空文本按 1 行计', () => {
    expect(estimateLines('')).toBe(1)
  })

  it('按换行数与 80 字符折行取较大者', () => {
    expect(estimateLines('a\nb\nc')).toBe(3)
    expect(estimateLines('x'.repeat(100))).toBe(2)
    expect(estimateLines('短文本')).toBe(1)
  })
})

function msg(content: string): Message {
  return { id: 'x', role: 'agent', content, timestamp: 0 }
}

describe('itemHeight', () => {
  it('固定高度项', () => {
    expect(itemHeight({ kind: 'round-header', roundId: 'r', startIndex: 0, count: 2, collapsed: false })).toBe(32)
    expect(itemHeight({ kind: 'step-indicator', step: 'turn_1', count: 1 })).toBe(36)
    expect(itemHeight({ kind: 'error', error: 'boom' })).toBe(60)
  })

  it('消息高度随内容行数增长', () => {
    expect(itemHeight({ kind: 'message', message: msg('a'), index: 0 })).toBe(102)
    expect(itemHeight({ kind: 'message', message: msg('a\nb\nc'), index: 0 })).toBe(146)
  })

  it('思考流式内容封顶 200px', () => {
    expect(itemHeight({ kind: 'streaming-thinking', thinking: 'x'.repeat(10000) })).toBe(200)
  })

  it('工具调用按 80px/条叠加', () => {
    const toolCalls = [{}, {}] as ToolCall[]
    expect(itemHeight({ kind: 'tool-calls', toolCalls })).toBe(184)
  })
})

describe('hasActiveContent', () => {
  it('全空时为 false', () => {
    expect(hasActiveContent([], '', '', [], null)).toBe(false)
  })

  it('任一通道有内容即为 true', () => {
    expect(hasActiveContent([msg('hi')], '', '', [], null)).toBe(true)
    expect(hasActiveContent([], '输出', '', [], null)).toBe(true)
    expect(hasActiveContent([], '', '思考', [], null)).toBe(true)
    expect(hasActiveContent([], '', '', [{} as ToolCall], null)).toBe(true)
    expect(hasActiveContent([], '', '', [], '错误')).toBe(true)
  })
})
