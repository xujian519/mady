/**
 * ToolCard handoff 过滤测试。
 *
 * Invisible Handoff 红线：handoff 工具调用不得渲染进消息流。
 * isHandoffTool 是双层防护：
 *   1. toolCall.invisible === true（Go 端 AGUI 事件携带）
 *   2. 名称前缀黑名单 transfer_to_ / handoff_to_（与 Go handoff.go 同步）
 */

import { describe, it, expect } from 'vitest'
import { isHandoffTool } from '../ToolCard'
import type { ToolCall } from '@/stores/chat'

function tc(name: string, invisible?: boolean): ToolCall {
  return { id: 't1', name, args: '{}', status: 'completed', invisible }
}

describe('isHandoffTool（Invisible Handoff 红线）', () => {
  it('invisible=true 一律过滤（最高优先级）', () => {
    expect(isHandoffTool(tc('read_file', true))).toBe(true)
    expect(isHandoffTool(tc('transfer_to_lawyer', true))).toBe(true)
  })

  it('transfer_to_ 前缀过滤', () => {
    expect(isHandoffTool(tc('transfer_to_claim_drafting'))).toBe(true)
  })

  it('handoff_to_ 前缀过滤', () => {
    expect(isHandoffTool(tc('handoff_to_enablement'))).toBe(true)
  })

  it('普通工具不过滤', () => {
    expect(isHandoffTool(tc('read_file'))).toBe(false)
    expect(isHandoffTool(tc('bash'))).toBe(false)
    expect(isHandoffTool(tc('grep'))).toBe(false)
  })

  it('前缀必须在开头：名称中间含 transfer_to_ 不过滤', () => {
    expect(isHandoffTool(tc('my_transfer_to_helper'))).toBe(false)
  })

  it('invisible=false 且非 handoff 前缀不过滤', () => {
    expect(isHandoffTool(tc('read_file', false))).toBe(false)
  })
})
