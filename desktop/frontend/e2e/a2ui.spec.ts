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
  }, { timeout: 5_000 })
}

// ── Envelope 构造辅助 ─────────────────────────────

const BASIC_CATALOG = 'https://a2ui.org/specification/v0_9_1/catalogs/basic/catalog.json'

function makeCreateSurface(surfaceId: string) {
  return {
    version: 'v0.9.1',
    createSurface: { surfaceId, catalogId: BASIC_CATALOG },
  }
}

function makeUpdateComponents(surfaceId: string, components: any[]) {
  return {
    version: 'v0.9.1',
    updateComponents: { surfaceId, components },
  }
}

function makeUpdateDataModel(surfaceId: string, path: string, value: any) {
  return {
    version: 'v0.9.1',
    updateDataModel: { surfaceId, path, value },
  }
}

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
