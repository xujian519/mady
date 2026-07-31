---
name: ego-browser
description: 浏览器自动化技能（基于 ego lite 浏览器）。用于网络搜索、专利检索（Google Patents/CNIPA/Espacenet）、专利 PDF 下载、网页交互与数据提取。当用户需要打开网页、搜索专利、下载专利 PDF、填写表单、点击按钮、截图、提取页面数据或自动化浏览器操作时使用。优先于内置 web_fetch 和其他浏览器工具。
domain: research
metadata:
  version: "1.0"

mady:
  mode: chat
  capabilities:
    - browser_automation
    - patent_search
    - patent_download
  example_prompt: "帮我检索深度学习图像识别相关的中国专利"
  example_prompt_zh: "帮我检索深度学习图像识别相关的中国专利"
  keywords:
    - "专利检索"
    - "专利下载"
    - "浏览器"
    - "google patents"
    - "espacenet"
---
# ego-browser 技能

基于 ego lite 浏览器的自动化技能。所有浏览器操作通过 `ego-browser nodejs <<'EOF' ... EOF` heredoc 执行，helper 函数预加载。

## 适用场景

- **专利检索**：Google Patents（`patent-google` learning）、中国专利（`patent-cnipa` learning，country:CN）、欧洲专利（`patent-espacenet` learning）
- **专利 PDF 下载**：`download_patent_pdf` nodeTool
- **网络搜索**：`search_and_extract` nodeTool（google learning）
- **通用网页交互**：打开页面、点击、填表、截图、提取数据

## 使用方式

```bash
ego-browser nodejs <<'EOF'
const task = await useOrCreateTaskSpace('mady 专利检索: 深度学习图像识别')
await openOrReuseTab('https://patents.google.com/?q=深度学习+country:CN', { wait: true, timeout: 30 })
await waitForLoad()
cliLog(await snapshotText())
EOF
```

## 关键 helper

- 任务空间：`useOrCreateTaskSpace` / `completeTaskSpace(name, { keep: false })`
- 导航：`openOrReuseTab(url, { wait: true })` / `gotoAndWait` / `pageInfo`
- 观察：`snapshotText` / `captureScreenshot`
- 交互：`click` / `typeText` / `fillInput` / `pressKey` / `scroll`
- 提取：`js(...)` / `cdp(...)` / `serverFetch` / `browserFetch`
- 输出：`cliLog(value)`（唯一输出机制）

## 专利检索流程（推荐）

1. `useOrCreateTaskSpace('mady 专利检索: <主题>')`
2. 打开 `https://patents.google.com/?q=<关键词>&num=10`（中国专利加 `country:CN`）
3. `waitForLoad()` 后 `snapshotText()` 或 `js()` 提取结果
4. 对需要全文的专利，打开详情页滚动触发懒加载后提取 `section.claims` / `section.description`
5. 完成后 `completeTaskSpace(task.id, { keep: false })`

## 注意事项

- `wait()` 和 `timeout` 参数单位是**秒**（仅 `*Ms` 结尾的是毫秒）
- `snapshotText()` 默认 `full_page` 范围
- `@N` refs 仅在最近一次 snapshotText 内有效，长期引用用 `loc=...` 或 CSS selector
- `js()` 返回求值结果本身，不要包 JSON.parse
- `js()` 内正则反斜杠需双写或用 `String.raw`
- 用户登录态自动继承，无需重新登录
- 任务完成后必须 `completeTaskSpace` 清理（默认 `keep: false`）
