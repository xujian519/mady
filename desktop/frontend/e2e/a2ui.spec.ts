/**
 * A2UI 渲染器端到端测试。
 *
 * 测试场景：
 *   1. 应用启动冒烟测试（React 渲染 + 就绪状态）
 *   2. 通过 page.evaluate 注入 createSurface envelope，验证 SurfaceStore 响应
 *   3. 注入 updateComponents + updateDataModel，验证组件树渲染
 */

import { test, expect } from '@playwright/test'

// ── 辅助函数 ───────────────────────────────────────

async function waitForApp(page: import('@playwright/test').Page) {
  // 等待 React 应用挂载（头部标题渲染即视为可用）。
  // 不等待后端就绪事件：dev 模式无 Wails 后端，
  // 且 __mady 测试接口在挂载时立即可用。
  await page.waitForSelector('h1', { timeout: 10_000 })
}

async function getA2UI(page: import('@playwright/test').Page) {
  // 等待 __mady 测试接口就绪
  return page.waitForFunction(() => {
    const m = (window as any).__mady
    return m?.a2ui?.applyEnvelope !== undefined
  }, { timeout: 10_000 })
}

// ── Envelope 构造辅助 ─────────────────────────────

const BASIC_CATALOG = 'https://a2ui.org/specification/v0_9_1/catalogs/basic/catalog.json'

// ── 测试 1：冒烟测试 ──────────────────────────────

test('应用启动冒烟测试', async ({ page }) => {
  await page.goto('/')
  await waitForApp(page)
  await expect(page.locator('h1').first()).toHaveText('Mady')
  // React 主界面已渲染（三栏布局的 main 区域存在）
  await expect(page.locator('main')).toBeVisible()
})

// ── 测试 2：createSurface 创建 surface ─────────────

test('createSurface 创建 A2UI surface', async ({ page }) => {
  await page.goto('/')
  await waitForApp(page)
  await getA2UI(page)

  const applied = await page.evaluate((catalog) => {
    const a2ui = (window as any).__mady.a2ui
    const env = {
      version: 'v0.9.1',
      createSurface: { surfaceId: 'test', catalogId: catalog },
    }
    try {
      a2ui.applyEnvelope(env)
      const srf = a2ui.getSurface('test')
      return srf ? { id: srf.id, catalogId: srf.catalogId } : null
    } catch (e: any) {
      return { error: e.message }
    }
  }, BASIC_CATALOG)

  expect(applied).toEqual({ id: 'test', catalogId: BASIC_CATALOG })
})

// ── 测试 3：updateComponents 渲染组件树 ────────────

test('updateComponents 后 root 组件正确', async ({ page }) => {
  await page.goto('/')
  await waitForApp(page)
  await getA2UI(page)

  const result = await page.evaluate((catalog) => {
    const a2ui = (window as any).__mady.a2ui

    // 创建 surface
    a2ui.applyEnvelope({
      version: 'v0.9.1',
      createSurface: { surfaceId: 'demo', catalogId: catalog },
    })

    // 添加组件
    a2ui.applyEnvelope({
      version: 'v0.9.1',
      updateComponents: {
        surfaceId: 'demo',
        components: [
          { id: 'root', component: 'Column', children: ['txt1'] },
          { id: 'txt1', component: 'Text', content: 'Hello A2UI' },
        ],
      },
    })

    // 验证
    const srf = a2ui.getSurface('demo')
    if (!srf) return { error: 'surface not found' }

    const root = srf.components.get('root')
    if (!root) return { error: 'root component not found' }

    const txt1 = srf.components.get('txt1')
    if (!txt1) return { error: 'txt1 component not found' }

    return {
      rootType: root.type,
      txt1Type: txt1.type,
      txt1Content: txt1.props.content,
    }
  }, BASIC_CATALOG)

  expect(result).toEqual({
    rootType: 'Column',
    txt1Type: 'Text',
    txt1Content: 'Hello A2UI',
  })
})

// ── 测试 4：data model 更新 ────────────────────────

test('updateDataModel 更新 data model', async ({ page }) => {
  await page.goto('/')
  await waitForApp(page)
  await getA2UI(page)

  const result = await page.evaluate((catalog) => {
    const a2ui = (window as any).__mady.a2ui

    a2ui.applyEnvelope({
      version: 'v0.9.1',
      createSurface: { surfaceId: 'dm', catalogId: catalog, sendDataModel: true },
    })

    a2ui.applyEnvelope({
      version: 'v0.9.1',
      updateDataModel: { surfaceId: 'dm', path: '/user/name', value: 'Alice' },
    })

    const srf = a2ui.getSurface('dm')
    if (!srf) return { error: 'surface not found' }
    return { name: (srf.dataModel as any)?.user?.name }
  }, BASIC_CATALOG)

  expect(result).toEqual({ name: 'Alice' })
})

// ── 测试 5：按钮点击 → sendAction 闭环 ────────────

test('按钮点击触发 sendAction 到后端', async ({ page }) => {
  await page.goto('/')
  await waitForApp(page)
  await getA2UI(page)

  // Stub Wails Go binding：记录 sendAction 调用参数。
  // 生产环境 Wails 注入 window.go.main.App.SendAction；
  // 此处注入 stub 以在无后端 dev 模式验证完整调用链。
  await page.evaluate(() => {
    const stub = {
      main: {
        App: {
          SendAction: async (surfaceId: string, action: unknown) => {
            ;(window as any).__sendActionCalls =
              (window as any).__sendActionCalls ?? []
            ;(window as any).__sendActionCalls.push({ surfaceId, action })
          },
        },
      },
    }
    ;(window as any).go = stub
  })

  // 创建 surface + Button（带 action.event）
  await page.evaluate(
    ([catalog]) => {
      const a2ui = (window as any).__mady.a2ui
      a2ui.applyEnvelope({
        version: 'v0.9.1',
        createSurface: { surfaceId: 'action-demo', catalogId: catalog },
      })
      a2ui.applyEnvelope({
        version: 'v0.9.1',
        updateComponents: {
          surfaceId: 'action-demo',
          components: [
            {
              id: 'root',
              component: 'Column',
              children: ['btn1'],
            },
            {
              id: 'btn1',
              component: 'Button',
              label: '批准',
              // 扁平 wire format：除 id/component 外均并入 props
              action: {
                event: { name: 'approve', context: { id: 'req-1' } },
              },
            },
          ],
        },
      })
    },
    [BASIC_CATALOG] as any,
  )

  // A2UIOverlay 应渲染 Button 到 DOM
  const button = page.locator('[data-a2ui-id="btn1"]')
  await expect(button).toBeVisible({ timeout: 10_000 })

  // 点击按钮 → 触发 onAction → sendAction
  await button.click()

  // 验证 SendAction 被调用且参数正确
  await page.waitForFunction(() => {
    return (window as any).__sendActionCalls?.length > 0
  }, undefined, { timeout: 10_000 })
  const recorded = await page.evaluate(() => (window as any).__sendActionCalls)
  expect(recorded.length).toBe(1)
  expect(recorded[0].surfaceId).toBe('action-demo')
  expect(recorded[0].action).toMatchObject({
    name: 'approve',
    sourceComponentId: 'btn1',
    context: { id: 'req-1' },
  })
})
