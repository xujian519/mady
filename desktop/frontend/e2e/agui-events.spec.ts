/**
 * AGUI 事件订阅端到端测试（G3 事件覆盖缺口修复的回归保障）。
 *
 * 覆盖 02-spec §1.2 映射表中此前未订阅的事件：
 *   1. tool-call-args / tool-call-result — ToolCard 参数流式填充与结果展示
 *   2. step-started / step-finished — 多步 turn 进度指示器
 *   3. compaction-start / compaction-end — 上下文压缩提示
 *   4. auto-retry — 自动重试提示（收到后续 token 自动清除）
 *   5. context-usage — 上下文使用率（ContextIndicator 数据源）
 *
 * 注入方式：window.__mady.agui.dispatch(name, payload)，
 * 等价于 Wails Events 推送后 client.ts 的分发路径。
 */

import { test, expect, type Page } from '@playwright/test'

// ── 辅助函数 ───────────────────────────────────────

async function waitForTestApi(page: Page) {
  // __mady 在 App 挂载时立即可用（不依赖后端就绪事件），
  // store 级断言无需等待 SplashScreen 关闭
  await page.waitForFunction(() => {
    const m = (window as any).__mady
    return m?.agui?.dispatch !== undefined
  }, { timeout: 5_000 })
}

async function dispatch(page: Page, name: string, payload: unknown) {
  await page.evaluate(
    ([n, p]) => (window as any).__mady.agui.dispatch(n, p),
    [name, payload] as const,
  )
}

async function getChatState(page: Page) {
  return page.evaluate(() => (window as any).__mady.agui.getChatState())
}

test.beforeEach(async ({ page }) => {
  await page.goto('/')
  await waitForTestApi(page)
})

// ── 1. 工具调用：参数流式填充 + 结果展示 ─────────────

test('tool-call-args/result 填充 ToolCard 参数与结果', async ({ page }) => {
  await dispatch(page, 'tool-call-start', {
    type: 'TOOL_CALL_START',
    toolCallId: 'tc-1',
    toolCallName: 'read_file',
  })
  await dispatch(page, 'tool-call-args', { type: 'TOOL_CALL_ARGS', toolCallId: 'tc-1', delta: '{"path":' })
  await dispatch(page, 'tool-call-args', { type: 'TOOL_CALL_ARGS', toolCallId: 'tc-1', delta: '"main.go"}' })

  let state = await getChatState(page)
  expect(state.toolCalls).toHaveLength(1)
  expect(state.toolCalls[0].name).toBe('read_file')
  expect(state.toolCalls[0].args).toBe('{"path":"main.go"}')
  expect(state.toolCalls[0].status).toBe('running')

  await dispatch(page, 'tool-call-end', { type: 'TOOL_CALL_END', toolCallId: 'tc-1' })
  await dispatch(page, 'tool-call-result', {
    type: 'TOOL_CALL_RESULT',
    toolCallId: 'tc-1',
    content: 'package main',
  })

  state = await getChatState(page)
  expect(state.toolCalls[0].status).toBe('completed')
  expect(state.toolCalls[0].result).toBe('package main')
})

// ── 2. 步骤进度指示器 ─────────────────────────────

test('step-started/finished 驱动进度指示器', async ({ page }) => {
  await dispatch(page, 'step-started', { type: 'STEP_STARTED', stepName: 'turn_1' })
  let state = await getChatState(page)
  expect(state.currentStep).toBe('turn_1')
  expect(state.stepCount).toBe(1)

  await dispatch(page, 'step-finished', { type: 'STEP_FINISHED', stepName: 'turn_1' })
  state = await getChatState(page)
  expect(state.currentStep).toBeNull()
  expect(state.stepCount).toBe(1) // 计数保留
})

// ── 3. 上下文压缩提示 ─────────────────────────────

test('compaction-start/end 驱动压缩提示', async ({ page }) => {
  await dispatch(page, 'compaction-start', {
    type: 'CUSTOM',
    name: 'compaction_start',
    value: { tokens_before: 120000, context_window: 128000 },
  })
  let state = await getChatState(page)
  expect(state.compaction?.active).toBe(true)
  expect(state.compaction?.tokensBefore).toBe(120000)

  await dispatch(page, 'compaction-end', {
    type: 'CUSTOM',
    name: 'compaction_end',
    value: { tokens_before: 120000, tokens_after: 30000, messages_cut: 6, duration_ms: 150 },
  })
  state = await getChatState(page)
  expect(state.compaction?.active).toBe(false)
  expect(state.compaction?.tokensAfter).toBe(30000)
  expect(state.compaction?.messagesCut).toBe(6)
})

// ── 4. 自动重试提示（收到 token 自动清除） ───────────

test('auto-retry 提示在收到后续 token 后清除', async ({ page }) => {
  await dispatch(page, 'auto-retry', {
    type: 'CUSTOM',
    name: 'auto_retry',
    value: { attempt: 2, max_retries: 3, delay_ms: 4000 },
  })
  let state = await getChatState(page)
  expect(state.retryNotice).toEqual({ attempt: 2, maxRetries: 3, delayMs: 4000 })

  // 重试成功 → 正常 token 流入 → 提示自动清除
  await dispatch(page, 'message-delta', { type: 'TEXT_MESSAGE_CONTENT', delta: '你好' })
  state = await getChatState(page)
  expect(state.retryNotice).toBeNull()
})

// ── 5. 上下文使用率 ───────────────────────────────

test('context-usage 更新上下文使用率', async ({ page }) => {
  await dispatch(page, 'context-usage', {
    type: 'CUSTOM',
    name: 'context_usage',
    usagePercent: 42,
    tokenUsage: { totalTokens: 53760 },
    contextWindow: 128000,
  })
  const state = await getChatState(page)
  expect(state.contextUsagePercent).toBe(42)
})
