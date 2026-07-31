#!/usr/bin/env node
// @ts-check
/**
 * check-wailsjs-contract.mjs
 *
 * 校验前端手工包装层与 Wails CLI 自动生成声明之间的一致性（类型漂移拦截）。
 *
 * 背景：
 *   desktop/frontend/src/lib/backend.ts 手动包装 Wails Go binding，形如
 *     callBinding<T>('main/App', 'XxxMethod', ...)
 *   而 desktop/frontend/wailsjs/go/ 下的 .d.ts / models.ts 由 `wails dev/build`
 *   自动生成（DO NOT EDIT）。Go 侧修改方法名或结构体字段后，前端会静默失配，
 *   本脚本在 CI 中静态拦截此类漂移。
 *
 * 校验项：
 *   1) 方法名契约（硬性，缺失 → exit 1）
 *      backend.ts 中每个 callBinding 引用的方法名，必须存在于对应
 *      wailsjs/go/<module>.d.ts 的 `export function XxxMethod(...)` 声明中。
 *   2) 接口字段对比（backend.ts interface ↔ models.ts 生成类，按字段名集合）
 *      - backend 有而生成类没有的字段：运行时读到 undefined，视为漂移
 *        → 硬性错误（除非列入 ALLOWED_INTERIM_TYPES 的占位类型）。
 *      - 生成类有而 backend 没有的字段：结构子集（Go 侧新增字段），仅提示。
 *
 * 用法：
 *   node scripts/check-wailsjs-contract.mjs
 *   node scripts/check-wailsjs-contract.mjs <backend.ts 路径>   # 定制/测试
 *
 * 退出码：0 = 通过；1 = 存在漂移。
 *
 * 零第三方依赖（Node >= 18，ESM），纯正则 + 花括号配对扫描。
 */

import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const ROOT_DIR = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const DEFAULT_BACKEND_TS = path.join(ROOT_DIR, 'desktop/frontend/src/lib/backend.ts')
const WAILSJS_GO_DIR = path.join(ROOT_DIR, 'desktop/frontend/wailsjs/go')
const MODELS_TS = path.join(WAILSJS_GO_DIR, 'models.ts')

/**
 * 已知的“初步/占位”接口：backend.ts 中明确注释为初步类型、尚未按真实 Go
 * 结构补齐的类型。字段对比结果仅提示，不拦截 CI。
 * 对应 UI 任务完成后应从本清单移除。
 */
const ALLOWED_INTERIM_TYPES = new Set(['ThreadSnapshot'])

/**
 * 手工接口 → 生成类 名称对照（backend.ts 命名与 Go 结构体命名不一致时使用）。
 * backend.ts 以 ModelInfo 描述模型条目，Go 侧/生成物命名为 ModelEntry。
 */
const TYPE_ALIASES = { ModelInfo: 'ModelEntry' }

// ── 解析工具 ────────────────────────────────────────────

/** 返回 src[openIndex] 处 `{` 的匹配 `}` 下标；未闭合返回 -1。 */
function findMatchingBrace(src, openIndex) {
  let depth = 0
  for (let i = openIndex; i < src.length; i++) {
    const ch = src[i]
    if (ch === '{') depth++
    else if (ch === '}') {
      depth--
      if (depth === 0) return i
    }
  }
  return -1
}

/** 去掉代码块中的行注释与块注释（models.ts 的 `// Go type: time` 注释会干扰字段提取）。 */
function stripComments(block) {
  return block.replace(/\/\*[\s\S]*?\*\//g, ' ').replace(/\/\/[^\n]*/g, ' ')
}

/** 从代码块中提取字段名（匹配 `name:` / `name?:` 声明，去重，保序）。 */
function extractFields(block) {
  const names = [...stripComments(block).matchAll(/\b([A-Za-z_$][\w$]*)\s*\??\s*:/g)].map((m) => m[1])
  return [...new Set(names)]
}

/** 提取 `export interface Name { ... }` 的接口名与字段名列表。 */
function extractInterfaces(src) {
  const out = []
  let pos = 0
  while (pos < src.length) {
    const m = /\bexport interface\s+(\w+)\s*\{/.exec(src.slice(pos))
    if (!m) break
    const name = m[1]
    const openIdx = pos + m.index + m[0].length - 1
    const closeIdx = findMatchingBrace(src, openIdx)
    if (closeIdx < 0) break
    out.push({ name, fields: extractFields(src.slice(openIdx + 1, closeIdx)) })
    pos = closeIdx + 1
  }
  return out
}

/** 提取 models.ts 中 namespace 下的生成类及其字段名列表。 */
function extractGeneratedClasses(src) {
  const out = []
  let nsPos = 0
  while (nsPos < src.length) {
    const nsM = /\bexport namespace\s+(\w+)\s*\{/.exec(src.slice(nsPos))
    if (!nsM) break
    const nsName = nsM[1]
    const nsOpenIdx = nsPos + nsM.index + nsM[0].length - 1
    const nsCloseIdx = findMatchingBrace(src, nsOpenIdx)
    if (nsCloseIdx < 0) break
    const nsBody = src.slice(nsOpenIdx + 1, nsCloseIdx)

    let clsPos = 0
    while (clsPos < nsBody.length) {
      const clsM = /\bexport class\s+(\w+)\s*\{/.exec(nsBody.slice(clsPos))
      if (!clsM) break
      const clsName = clsM[1]
      const clsOpenIdx = clsPos + clsM.index + clsM[0].length - 1
      const clsCloseIdx = findMatchingBrace(nsBody, clsOpenIdx)
      if (clsCloseIdx < 0) break
      const clsBody = nsBody.slice(clsOpenIdx + 1, clsCloseIdx)
      // 字段声明只出现在 static createFrom / constructor 之前
      const fieldsPart = clsBody.split('static createFrom')[0].split(/constructor\s*\(/)[0]
      out.push({ namespace: nsName, name: clsName, fields: extractFields(fieldsPart) })
      clsPos = clsCloseIdx + 1
    }
    nsPos = nsCloseIdx + 1
  }
  return out
}

// ── 主流程 ──────────────────────────────────────────────

function main() {
  const backendTs = process.argv[2] ? path.resolve(process.argv[2]) : DEFAULT_BACKEND_TS
  const rel = (p) => path.relative(ROOT_DIR, p) || p
  const issues = [] // { level: 'error'|'warn'|'info', msg }

  if (!fs.existsSync(backendTs)) {
    console.error(`[ERROR] 找不到 backend.ts: ${backendTs}`)
    return 1
  }
  const backendSrc = fs.readFileSync(backendTs, 'utf8')

  // ── 1. 方法名契约 ──────────────────────────────────
  // 提取 callBinding<T>('main/App', 'XxxMethod', ...) 的 (module, method)
  const callPattern =
    /\bcallBinding\s*<\s*[^>]*>\s*\(\s*(['"])([^'"]+)\1\s*,\s*(['"])([A-Za-z_$][\w$]*)\3/g
  const seen = new Set()
  const calls = []
  for (const m of backendSrc.matchAll(callPattern)) {
    const module = m[2]
    const method = m[4]
    const key = `${module}::${method}`
    if (seen.has(key)) continue
    seen.add(key)
    calls.push({ module, method, line: backendSrc.slice(0, m.index).split('\n').length })
  }

  const modules = [...new Set(calls.map((c) => c.module))]
  let generatedFnTotal = 0
  const generatedModules = []

  for (const mod of modules) {
    const parts = mod.split('/')
    const dtsFile = path.join(
      WAILSJS_GO_DIR,
      ...parts.slice(0, -1),
      `${parts[parts.length - 1]}.d.ts`,
    )
    const modCalls = calls.filter((c) => c.module === mod)
    if (!fs.existsSync(dtsFile)) {
      issues.push({
        level: 'error',
        msg: `生成声明缺失：${mod} 对应 ${rel(dtsFile)} 不存在（共 ${modCalls.length} 处调用受影响：backend.ts 第 ${modCalls.map((c) => c.line).join(', ')} 行）。可能 wails 未生成或 Go 侧包结构变更`,
      })
      continue
    }
    const fnNames = new Set(
      [...fs.readFileSync(dtsFile, 'utf8').matchAll(/^export function\s+([A-Za-z_$][\w$]*)\s*\(/gm)].map(
        (x) => x[1],
      ),
    )
    generatedFnTotal += fnNames.size
    generatedModules.push(mod)

    for (const c of modCalls) {
      if (!fnNames.has(c.method)) {
        issues.push({
          level: 'error',
          msg: `方法缺失：backend.ts:${c.line} 调用 callBinding('${mod}', '${c.method}')，但 ${rel(dtsFile)} 中没有对应 export function。Go 侧可能已改方法名/签名，需同步 backend.ts 或重新生成 wailsjs`,
        })
      }
    }
    for (const fn of fnNames) {
      if (!modCalls.some((c) => c.method === fn)) {
        issues.push({
          level: 'info',
          msg: `未使用的 binding：${mod}.${fn}（生成声明存在但 backend.ts 未调用，可能只是尚未接线）`,
        })
      }
    }
  }

  // ── 2. 接口字段对比（backend.ts interface ↔ models.ts 类） ──
  let compared = 0
  if (!fs.existsSync(MODELS_TS)) {
    issues.push({ level: 'warn', msg: `models.ts 不存在（${rel(MODELS_TS)}），跳过接口字段对比` })
  } else {
    const classes = extractGeneratedClasses(fs.readFileSync(MODELS_TS, 'utf8'))
    const classByName = new Map()
    for (const cls of classes) {
      if (!classByName.has(cls.name)) classByName.set(cls.name, [])
      classByName.get(cls.name).push(cls)
    }

    for (const iface of extractInterfaces(backendSrc)) {
      const alias = TYPE_ALIASES[iface.name]
      const matches = (classByName.get(iface.name) ?? []).concat(
        alias ? classByName.get(alias) ?? [] : [],
      )
      if (matches.length === 0) {
        issues.push({
          level: 'info',
          msg: `接口 ${iface.name} 在 models.ts 中没有同名生成类，未参与字段对比`,
        })
        continue
      }
      compared++
      const ifaceSet = new Set(iface.fields)
      const union = new Set()
      for (const cls of matches) for (const f of cls.fields) union.add(f)
      const backendOnly = iface.fields.filter((f) => !union.has(f)).sort()
      const modelOnly = [...union].filter((f) => !ifaceSet.has(f)).sort()
      const scope = matches.map((c) => `${c.namespace}.${c.name}`).join(', ')

      if (alias) {
        issues.push({
          level: 'info',
          msg: `接口 ${iface.name} ↔ 生成类 ${scope}（经 TYPE_ALIASES 对照）`,
        })
      }
      if (backendOnly.length > 0) {
        const interim = ALLOWED_INTERIM_TYPES.has(iface.name)
        issues.push({
          level: interim ? 'warn' : 'error',
          msg:
            `接口 ${iface.name} 声明了生成类 ${scope} 中不存在的字段：${backendOnly.join(', ')}` +
            (interim
              ? '（该接口列入 ALLOWED_INTERIM_TYPES，仅提示；占位类型完成后应从清单移除）'
              : '（运行时将读到 undefined，疑似 Go 结构体字段已变更）'),
        })
      }
      if (modelOnly.length > 0) {
        issues.push({
          level: 'info',
          msg: `接口 ${iface.name} 未覆盖生成类字段：${modelOnly.join(', ')}（结构子集，允许）`,
        })
      }
    }
  }

  // ── 3. 报告与退出码 ────────────────────────────────
  const errors = issues.filter((i) => i.level === 'error')
  const warnings = issues.filter((i) => i.level === 'warn')
  const infos = issues.filter((i) => i.level === 'info')

  console.log('=== Wails JS 契约校验 ===')
  console.log(`  backend.ts : ${rel(backendTs)}`)
  console.log(`  wailsjs 目录: ${rel(WAILSJS_GO_DIR)}`)
  console.log(`  callBinding : ${calls.length} 个唯一方法（${modules.length} 个模块）`)
  console.log(
    `  生成方法    : ${generatedFnTotal} 个 export function（${generatedModules.join(', ') || '-'}）`,
  )
  console.log(`  接口字段对比: ${compared} 个接口`)
  console.log('')

  const dump = (lv, msg) => console.log(`  [${lv.toUpperCase()}] ${msg}`)
  if (infos.length > 0) {
    console.log('── 信息 ──')
    for (const i of infos) dump(i.level, i.msg)
    console.log('')
  }
  if (warnings.length > 0) {
    console.log('── 警告（不阻塞 CI，建议处理） ──')
    for (const i of warnings) dump(i.level, i.msg)
    console.log('')
  }
  if (errors.length > 0) {
    console.log('── 漂移（阻塞 CI） ──')
    for (const i of errors) dump(i.level, i.msg)
    console.log('')
  }

  if (errors.length === 0) {
    console.log('结论：✓ 通过，未发现契约漂移')
    return 0
  }
  console.log(`结论：✗ 失败，发现 ${errors.length} 处漂移（exit 1）`)
  return 1
}

process.exitCode = main()
