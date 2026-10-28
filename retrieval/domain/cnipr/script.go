package cnipr

import (
	"encoding/json"
	"strconv"
	"strings"
)

// 脚本生成：脚本体以 stdin 交给 ego-browser 执行。参数直接写进脚本体（ego 的
// Node 运行时只从 stdin 接收脚本、不继承环境变量、不接受 argv）。
//
// 页内只取原文，判定与解析回到 receipt.go 的纯函数；脚本自身只判断两处页面状态：
// 登录态（页头「欢迎您」）与结果页是否渲染完成。

// 回传标签。
const (
	tagSearch     = "CNIPR_SEARCH"
	tagPDFLinks   = "CNIPR_PDFLINKS"
	tagLegal      = "CNIPR_LEGAL"
	taskSpaceName = "sati-patent-cnipr"
)

// sharedPreamble 是三个脚本共用的前缀：登录态等待、页面状态等待与失败分类。
func sharedPreamble(b PageBudgets) string {
	replacements := []string{
		"{{BASE}}", strconv.Quote(SiteBaseURL),
		"{{LOGIN_POLL_MS}}", strconv.Itoa(b.LoginPollMs),
		"{{LOGIN_ATTEMPTS}}", strconv.Itoa(b.LoginAttempts),
		"{{LOGIN_ROUNDS}}", strconv.Itoa(b.LoginRounds),
		"{{NAVIGATION_TIMEOUT_MS}}", strconv.Itoa(b.NavigationTimeoutMs),
		"{{ACTION_TIMEOUT_MS}}", strconv.Itoa(b.ActionTimeoutMs),
		"{{RESULT_TIMEOUT_MS}}", strconv.Itoa(b.ResultTimeoutMs),
		"{{RENDER_TIMEOUT_MS}}", strconv.Itoa(b.RenderTimeoutMs),
		"{{DETAIL_TIMEOUT_MS}}", strconv.Itoa(b.DetailTimeoutMs),
		"{{TASK_SPACE}}", strconv.Quote(taskSpaceName),
	}
	return strings.NewReplacer(replacements...).Replace(preambleTemplate)
}

const preambleTemplate = `const BASE = {{BASE}}
const LOGIN_HINT = '登录|用户名|密码'
const LOGIN_POLL_MS = {{LOGIN_POLL_MS}}
const LOGIN_ATTEMPTS = {{LOGIN_ATTEMPTS}}
const LOGIN_ROUNDS = {{LOGIN_ROUNDS}}
const NAVIGATION_TIMEOUT_MS = {{NAVIGATION_TIMEOUT_MS}}
const ACTION_TIMEOUT_MS = {{ACTION_TIMEOUT_MS}}
const RESULT_TIMEOUT_MS = {{RESULT_TIMEOUT_MS}}
const RENDER_TIMEOUT_MS = {{RENDER_TIMEOUT_MS}}
const DETAIL_TIMEOUT_MS = {{DETAIL_TIMEOUT_MS}}
const readBodyText = () => (document.body ? document.body.innerText : '')
const pageText = async (page) => await page.evaluate(readBodyText)
const failureOf = (error, fallback) => ({
  code: error && error.cniprCode ? error.cniprCode : fallback,
  message: error && error.message ? error.message : String(error),
})
const waitForState = async (page, action, expectation) => {
  try {
    await action()
  } catch (error) {
    const text = await pageText(page).catch(() => '')
    const login = !text.includes('欢迎您') && new RegExp(LOGIN_HINT).test(text)
    const detail = error && error.message ? error.message : String(error)
    throw Object.assign(new Error('等' + expectation + '失败：' + detail), { cniprCode: login ? 'login_required' : 'page_state' })
  }
}
const waitForLoggedIn = async (page) => {
  for (let round = 0; round < LOGIN_ROUNDS; round += 1) {
    for (let attempt = 0; attempt < LOGIN_ATTEMPTS; attempt += 1) {
      if ((await pageText(page)).includes('欢迎您')) return
      await page.waitForTimeout(LOGIN_POLL_MS)
    }
    if (round < LOGIN_ROUNDS - 1) await page.reload({ timeout: NAVIGATION_TIMEOUT_MS, waitUntil: 'domcontentloaded' })
  }
  throw Object.assign(new Error('未检测到 CNIPR 登录态：请在 ego lite 里登录 search.cnipr.com 后重试'), { cniprCode: 'login_required' })
}
const openHome = async (page) => {
  await waitForState(page, () => page.goto(BASE + '/', { timeout: NAVIGATION_TIMEOUT_MS, waitUntil: 'domcontentloaded' }), 'CNIPR 首页')
  await waitForLoggedIn(page)
}`

// emit 生成脚本末尾的回传语句（console 输出在 ego 里走 stderr，会话两路合并解析）。
func emit(tag string, extra []string) string {
	fields := append([]string{"items"}, extra...)
	fields = append(fields, "error: failure")
	quotedTag := mustJSON("EGO_" + tag + ":")
	return "console.log(" + quotedTag + " + JSON.stringify({ " + strings.Join(fields, ", ") + " }))"
}

// mustJSON 序列化脚本参数：参数都是字符串/切片，失败只可能是程序错误，返回 null 兜底。
func mustJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "null"
	}
	return string(data)
}

// BuildSearchScript 生成关键词检索脚本：主页检索框提交后读结果页，按页读取。
func BuildSearchScript(query string, pages int, budgets PageBudgets) string {
	quotedQuery := mustJSON(query)
	body := strings.NewReplacer(
		"{{QUERY}}", quotedQuery,
		"{{PAGES}}", strconv.Itoa(pages),
		"{{TASK_SPACE}}", strconv.Quote(taskSpaceName),
	).Replace(searchBodyTemplate)
	return sharedPreamble(budgets) + "\n" + body + "\n" + emit(tagSearch, []string{"header"}) + "\n"
}

const searchBodyTemplate = `const query = {{QUERY}}
const pages = {{PAGES}}
const readPage = (args) => {
  const header = document.querySelector('#slideselect')
  const rows = Array.from(document.querySelectorAll('div.g_item'))
  return {
    header: header ? header.innerText : null,
    items: rows.map((row, index) => {
      const box = row.querySelector('input[name="recordno"]')
      const patti = row.querySelector('li[name="patti"]')
      const cont = row.querySelector('div.g_cont')
      return {
        cursor: args.offset + index,
        number: box ? box.getAttribute('pnm') : null,
        title: patti ? patti.getAttribute('title') : null,
        text: cont ? cont.innerText : row.innerText,
      }
    }),
  }
}
const clickPagination = (wanted) => {
  const link = Array.from(document.querySelectorAll('#pagination_top a'))
    .find(anchor => anchor.innerText.trim() === String(wanted))
  if (!link) return false
  link.click()
  return true
}
const pageMarkerIs = (wanted) => {
  const marker = document.querySelector('#startPage')
  return !!marker && marker.value === String(wanted)
}
const turnToPage = async (page, target) => {
  const moved = await page.evaluate(clickPagination, target)
  if (!moved) return false
  await waitForState(page, () => page.waitForFunction(pageMarkerIs, target, { timeout: RESULT_TIMEOUT_MS }), '第 ' + target + ' 页')
  await waitForState(page, () => page.waitForSelector('div.g_item', { timeout: RENDER_TIMEOUT_MS }), '第 ' + target + ' 页记录')
  return true
}
const items = []
let header = null
let failure
try {
  const task = await taskSpace({{TASK_SPACE}})
  const page = task.page('p1')
  await openHome(page)
  await waitForState(page, () => page.fill('input[name="q"]', query, { clearFirst: true }), 'CNIPR 检索框')
  await waitForState(page, () => page.click('#search-btn', { timeout: ACTION_TIMEOUT_MS }), 'CNIPR 检索提交')
  await waitForState(page, () => page.waitForURL(/doOverviewSearch4Index/, { timeout: NAVIGATION_TIMEOUT_MS }), '检索结果页')
  await waitForState(page, () => page.waitForSelector('#detailSearch', { timeout: RESULT_TIMEOUT_MS }), '结果页详情表单')
  for (let index = 1; index <= pages; index += 1) {
    if (index > 1 && !(await turnToPage(page, index))) break
    const read = await page.evaluate(readPage, { offset: items.length })
    if (header === null) header = read.header
    if (read.items.length === 0) break
    items.push(...read.items)
  }
} catch (error) {
  failure = failureOf(error, 'page_state')
}`

// BuildPDFLinksScript 生成「按号取全文 PDF 直链」脚本：逐号检索后经详情检索接口取原文抽直链。
func BuildPDFLinksScript(numbers []string, budgets PageBudgets) string {
	quotedNumbers := mustJSON(numbers)
	pattern := mustJSON(strings.ReplaceAll(pdfURLPattern.String(), `\/`, `/`))
	body := strings.NewReplacer(
		"{{NUMBERS}}", quotedNumbers,
		"{{PDF_PATTERN}}", pattern,
		"{{TASK_SPACE}}", strconv.Quote(taskSpaceName),
	).Replace(pdfBodyTemplate)
	return sharedPreamble(budgets) + "\n" + body + "\n" + emit(tagPDFLinks, nil) + "\n"
}

const pdfBodyTemplate = `const numbers = {{NUMBERS}}
const extractLinks = (args) => {
  const form = document.getElementById('detailSearch')
  if (!form) return { error: 'no-detailSearch（未进入结果页？）' }
  const body = new FormData(form)
  body.set('recordCursor', String(args.cursor))
  body.set('startPage', '1')
  body.set('limit', '1')
  return new Promise((resolve) => {
    const xhr = new XMLHttpRequest()
    xhr.open('POST', '/search!doDetailSearch.action', true)
    xhr.onreadystatechange = () => {
      if (xhr.readyState !== 4) return
      try {
        const html = xhr.responseText || ''
        resolve({
          matches: html.match(new RegExp(args.pattern, 'gi')) || [],
          isUnexamined: /Unexamined_patent/i.test(html),
          isAuthorized: /Authorized_patent/i.test(html),
        })
      } catch (error) {
        resolve({ error: String(error) })
      }
    }
    setTimeout(() => resolve({ error: '详情检索 ' + args.detailTimeoutMs + ' 毫秒未返回' }), args.detailTimeoutMs)
    xhr.send(body)
  })
}
const items = []
let failure
try {
  const task = await taskSpace({{TASK_SPACE}})
  const page = task.page('p1')
  await openHome(page)
  for (const number of numbers) {
    try {
      await waitForState(page, () => page.goto(BASE + '/', { timeout: NAVIGATION_TIMEOUT_MS, waitUntil: 'domcontentloaded' }), 'CNIPR 首页')
      await page.fill('input[name="q"]', number, { clearFirst: true })
      await page.click('#search-btn', { timeout: ACTION_TIMEOUT_MS })
      await waitForState(page, () => page.waitForURL(/doOverviewSearch4Index/, { timeout: NAVIGATION_TIMEOUT_MS }), '检索结果页')
      await waitForState(page, () => page.waitForSelector('#detailSearch', { timeout: RESULT_TIMEOUT_MS }), '结果页详情表单')
      const found = await page.evaluate(extractLinks, { cursor: 0, pattern: {{PDF_PATTERN}}, detailTimeoutMs: DETAIL_TIMEOUT_MS })
      items.push({ number: number, matches: found.matches, isUnexamined: found.isUnexamined, isAuthorized: found.isAuthorized, error: found.error })
    } catch (error) {
      items.push({ number: number, matches: [], error: failureOf(error, 'page_state').message })
    }
  }
} catch (error) {
  failure = failureOf(error, 'page_state')
}`

// BuildLegalScript 生成法律状态查询脚本：申请号直接查，公开号先经检索结果页解析申请号。
func BuildLegalScript(queries []LegalQuery, budgets PageBudgets) string {
	type entry struct {
		Number    string `json:"number"`
		AppNumber string `json:"appNumber,omitempty"`
		PubNumber string `json:"pubNumber,omitempty"`
	}
	entries := make([]entry, 0, len(queries))
	for _, query := range queries {
		entries = append(entries, entry(query))
	}
	quotedEntries := mustJSON(entries)
	appPattern := mustJSON(strings.ReplaceAll(appNoSourcePattern, `\/`, `/`))
	body := strings.NewReplacer(
		"{{ENTRIES}}", quotedEntries,
		"{{APP_PATTERN}}", appPattern,
		"{{TASK_SPACE}}", strconv.Quote(taskSpaceName),
	).Replace(legalBodyTemplate)
	return sharedPreamble(budgets) + "\n" + body + "\n" + emit(tagLegal, nil) + "\n"
}

const appNoSourcePattern = `申请号[：:]\s*((?:CN)?\d{12}\.[0-9A-Z])`

const legalBodyTemplate = `const entries = {{ENTRIES}}
const APP_NUMBER = new RegExp({{APP_PATTERN}}, 'i')
const submitHomeSearch = () => {
  const input = document.querySelector('input[name="q"]')
  input.closest('form').querySelector('button').click()
}
const hasAppNumberLabel = () => !!document.body && /申请号[：:]/.test(document.body.innerText)
const resolveAppNumber = async (page, pubNumber) => {
  await waitForState(page, () => page.goto(BASE + '/', { timeout: NAVIGATION_TIMEOUT_MS, waitUntil: 'domcontentloaded' }), 'CNIPR 首页')
  await page.fill('input[name="q"]', pubNumber, { clearFirst: true })
  await page.evaluate(submitHomeSearch)
  await waitForState(page, () => page.waitForURL(/doOverviewSearch4Index/, { timeout: RESULT_TIMEOUT_MS }), '检索结果页')
  await waitForState(page, () => page.waitForFunction(hasAppNumberLabel, undefined, { timeout: RENDER_TIMEOUT_MS }), '结果页渲染')
  return (await pageText(page)).replace(/\s+/g, ' ').match(APP_NUMBER)
}
const hasLegalHeading = () => !!document.body && document.body.innerText.indexOf('法律状态检索结果') >= 0
const queryLegal = async (page, appNumber) => {
  await page.goto(BASE + '/pages!legal.action', { timeout: NAVIGATION_TIMEOUT_MS, waitUntil: 'domcontentloaded' })
  await waitForState(page, () => page.waitForSelector('#strAn', { timeout: RESULT_TIMEOUT_MS }), '法律状态检索表单')
  await page.fill('#strAn', appNumber, { clearFirst: true })
  await page.click('input[type=submit][value=检索]', { timeout: ACTION_TIMEOUT_MS })
  await waitForState(page, () => page.waitForURL(/doLegalSearch/, { timeout: RESULT_TIMEOUT_MS }), '法律状态结果页')
  await waitForState(page, () => page.waitForFunction(hasLegalHeading, undefined, { timeout: RENDER_TIMEOUT_MS }), '法律状态结果渲染')
  return await pageText(page)
}
const items = []
let failure
try {
  const task = await taskSpace({{TASK_SPACE}})
  const page = task.page('p1')
  await openHome(page)
  for (const entry of entries) {
    try {
      if (entry.appNumber) {
        items.push({ number: entry.number, appNumber: entry.appNumber, text: await queryLegal(page, entry.appNumber) })
        continue
      }
      const resolved = await resolveAppNumber(page, entry.pubNumber)
      if (!resolved) {
        items.push({ number: entry.number, resolvedFrom: entry.pubNumber, error: '公开(公告)号 ' + entry.pubNumber + ' 未检索到申请号，无法查询法律状态' })
        continue
      }
      const appNumber = resolved[1].toUpperCase()
      items.push({ number: entry.number, appNumber: appNumber, resolvedFrom: entry.pubNumber, text: await queryLegal(page, appNumber) })
    } catch (error) {
      items.push({ number: entry.number, error: failureOf(error, 'page_state').message })
    }
  }
} catch (error) {
  failure = failureOf(error, 'page_state')
}`
