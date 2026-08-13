# 桌面端开源社区开发规范与流程调研（Mady 对齐版）

> **调研日期**：2026-08-13 | **目的**：为「优化桌面端使其适合用户使用」提供社区规范与流程基准
> **定位**：本文档补充 `mady-desktop-standards.md`（v1.0，聚焦**设计与工程规范**）的**流程（Process）维度**——
> 版本管理、CI/CD 发布管线、签名公证、自动更新、打包资产、测试门禁、跨平台 UX 检查表。
> 规则编号沿用 `M-DSK-*`；对已有结论仅引用、不重复推导。
>
> **一手来源**：Wails v2.13 官方文档（application-development / crossplatform-build / code-signing）、
> Tauri v2 官方发布管线文档、Apple HIG、Microsoft Win32 UX Guide、
> 开源案例实测（achhabra2/riftshare、safing/portmaster、opskat/opskat、tiny-rdm、RWKV-Runner、jcp、WailBrew）。

---

## 目录

1. [调研结论速览](#1-调研结论速览)
2. [社区技术路线与架构共识](#2-社区技术路线与架构共识)
3. [版本管理流程](#3-版本管理流程)
4. [CI/CD 发布流水线](#4-cicd-发布流水线)
5. [签名、公证与分发](#5-签名公证与分发)
6. [自动更新](#6-自动更新)
7. [跨平台打包资产](#7-跨平台打包资产)
8. [测试门禁流程](#8-测试门禁流程)
9. [跨平台桌面 UX 检查表](#9-跨平台桌面-ux-检查表)
10. [Mady 差距分析与建议行动](#10-mady-差距分析与建议行动)
11. [参考来源](#11-参考来源)

---

## 1. 调研结论速览

社区（Electron / Tauri / Wails 三大 WebView 生态 + 桌面原生社区）在桌面开发上已形成高度一致的共识，可用五句话概括：

1. **架构**：核心逻辑与 UI 壳严格分层——「胖核心 + 薄壳」。UI 只是状态反射层，业务逻辑放非 UI 层（RiftShare `internal/`、Portmaster `service/` + `desktop/` 分离）。
2. **发布**：**tag 驱动 + 平台 matrix CI + 签名公证 + GitHub Release 上传** 是事实标准流水线；构建、签名、发布拆成独立 job/stage。
3. **版本**：版本号**单点权威源**（如 `wails.json` / `tauri.conf.json` / `package.json`），配 bump 脚本联动各平台元数据（Info.plist / appxmanifest / .desktop），人工改版本号必然漏改。
4. **更新**：Wails v2 生态无官方自动更新（v3 已内置），社区方案统一为「GitHub Release 为源 + semver 比较 + 自更新库」（RiftShare 用 `rhysd/go-github-selfupdate`）；macOS 上 Zip + `.app` 替换、Windows 上替换 exe 是通行做法。
5. **体验**：跨平台桌面 App 的 UX 共识是「**平台惯例优先于应用内一致性**」——菜单、快捷键、窗口行为跟随所在系统，视觉品牌可以统一，交互语法必须入乡随俗。

---

## 2. 社区技术路线与架构共识

### 2.1 WebView 三强路线（2026 视角）

| 框架 | 内核 | 社区规模 | 规范成熟度 | 与 Mady 的关系 |
|------|------|---------|-----------|---------------|
| Electron | Chromium | 最大（VS Code / Slack） | 发布/签名/更新生态最完整 | 流程模式可参考（electron-builder + GH Release + electron-updater） |
| Tauri | 系统 WebView（Wry） | 增长最快 | 发布管线文档化最好（tauri-action） | **流程范本**：单 action 完成 build + release + updater JSON |
| Wails v2 | 系统 WebView（WKWebView/WebView2） | Go 生态主流 | 官方提供 CI 模板 + 签名指南，自动更新缺位 | Mady 当前技术栈 |

> **关键信号**：Portmaster（原 Wails 大用户）已从 Wails 迁移到 Tauri，其架构保持「`service/` 核心守护进程 + `desktop/` 薄 UI」。这说明**架构分层比框架选择更稳定**——即使未来换壳，业务逻辑不受影响。Mady 的桌面端绑定层（`desktop/app*.go`）已经是薄适配器形态，符合此共识（M-DSK-WLS-001）。

### 2.2 架构模式（社区实测）

**模式 A：胖核心 + 薄壳（推荐，多案例采用）**

```
RiftShare:  app.go(薄绑定) + internal/{settings,transport,update}(业务) + frontend/(UI)
Portmaster: desktop/(UI) + service/(核心逻辑，独立进程)
tiny-rdm:   backend(Go 服务) + frontend(Vue UI)，桌面/Web 同代码库
```

**模式 B：业务在 Go、状态在 Go，前端只做反射（Wails 维护者 leaanthony 官方答复，discussion #909）**

> "For me, the frontend just reflects the state in Go. For others, they may see Go as a shell/persistence layer. There's no hard and fast rules."
> —— 例外：文本编辑器类场景（CodeMirror 编辑器状态可留在前端）。

**Mady 现状评估**：`desktop/app*.go` 已按 app_files / app_mcp / app_settings / app_skills 拆分，业务经 `tools/path.go` 沙箱与根模块 domain 层，符合模式 A。**建议**：纯业务/纯算法（如版本比较、更新检查、设置校验）从绑定文件下沉到 `desktop/internal/`，让绑定文件保持"只有入参校验 + 调 domain + 序列化"。

---

## 3. 版本管理流程

社区共识：

1. **单点权威源**：版本号只在一个文件维护，其余全部派生。
   - Tauri：`tauri.conf.json`（`version` 字段）
   - Electron：`package.json`
   - Wails 社区：`wails.json` 的 `info.productVersion`（Mady 已在 Makefile 单点维护 ✅）
2. **bump 脚本联动平台元数据**：RiftShare `scripts/bump-version.sh` 一次替换 Info.plist、appxmanifest.xml、exe.manifest、selfupdate.go 的 Version 常量。
3. **SemVer + tag**：发版 tag 形如 `v0.1.0`（Wails 官方 CI 模板 `v*`，Tauri `app-v*`），CI 由 tag 触发。
4. **CI 注入版本**：构建时经 ldflags 注入（`-X main.desktopVersion=...`，Mady `DESKTOP_LDFLAGS` 已实现 ✅），避免源码里写死版本导致发版要改代码。

**Mady 建议动作（P1）**：
- 新增 `scripts/bump-desktop-version.sh`（读 `desktop/wails.json` → 联动 `build/darwin/Info.plist` 的 CFBundleShortVersionString / CFBundleVersion、windows appxmanifest、`desktop/app_update.go` 的 Version 常量，若引入）。
- `desktop/wails.json` 与 `desktop/frontend/package.json` 的 version 目前都是 0.1.0 且各自独立维护——应统一为**只有 wails.json 是权威源**，package.json version 由脚本对齐或置为占位。

---

## 4. CI/CD 发布流水线

### 4.1 社区事实标准（Tauri 发布管线 + Wails 官方 CI 模板）

```
push tag v*（或 release 分支）
   └─ matrix 平台构建 job（fail-fast: false，一平台失败不影响其他）
        ├─ windows-latest  → wails build -nsis → 签名(signtool) → 上传制品
        ├─ macos-latest    → wails build darwin/universal → 导入证书 → gon 签名+公证 → 上传制品
        └─ ubuntu-latest   → apt 安装 webkit2gtk 依赖 → wails build → AppImage/deb → 上传制品
   └─ release job（needs: package）收集各平台制品 → 创建 GitHub Release（draft: true）
```

要点（Tauri 官方文档与 Wails 官方模板交叉验证）：

| 实践 | 出处 | 说明 |
|------|------|------|
| `fail-fast: false` | Wails 官方 / Tauri 官方 | 单平台构建失败不阻塞其余平台 |
| `NODE_OPTIONS=--max-old-space-size=4096` | Wails 官方模板 | 前端构建 OOM 防护 |
| 前端依赖缓存（setup-node cache / rust-cache） | Tauri 官方 | 构建提速 |
| `releaseDraft: true` | Tauri 官方 | 先草稿、人工确认后发布 |
| releaseName 用版本号（`App v__VERSION__`） | Tauri 官方 | Release 标题与版本强绑定 |
| 构建 job 与 release job 分离 | RiftShare / Tauri | package 与发布解耦，可加签名校验步骤 |

### 4.2 真实开源案例（实测核验）

**RiftShare `.github/workflows/build.yaml`**（Wails v2 官方签名指南推荐的参考项目）：
- 触发：`push tags: v*`
- matrix: `[macos-11, windows-latest]`（Linux 步骤注释保留，说明其 CI 演进历史）
- macOS：`Apple-Actions/import-codesign-certs@v1` 导入 p12 → brew 装 gon → `scripts/build-macos.sh`（内含 codesign + notarytool）
- Windows：`choco install mingw` → `wails build -platform windows/amd64 -clean` → zip
- 独立 `release` job（`needs: package`）：下载制品 → `marvinpinto/action-automatic-releases` 建 Release

**opskat/opskat `build-desktop.yml`**（活跃仓库，2026 实测）：
- `wails build -platform ${{ matrix.goos }}/${{ matrix.goarch }}`，Windows 加 `-nsis`
- ldflags 注入 version + commit ID
- Apple 证书导入仅在 darwin job（无证书时跳过，不阻塞）
- `go build` 先产出 embedded CLI 再打进桌面包（多产物打包模式）

### 4.3 Mady 现状与差距

**现状**（已具备）：
- `Makefile`：`desktop-dmg`（darwin/universal）、`desktop-notarize`（codesign→notarytool→stapler→spctl 全链路）、`desktop-build-quick`
- `.github/workflows/release.yml`：goreleaser 全平台（根模块 CLI 发布）
- `.github/workflows/ci.yml`：构建/测试（含 desktop 模块？见差距）

**差距（P1，本轮优化重点）**：
| 差距 | 说明 | 建议 |
|------|------|------|
| **桌面端无独立发布流水线** | `release.yml` 走 goreleaser 发 CLI 产物，`desktop/` 的 Wails 产物（.app/.exe/installer）未进 GitHub Release（**✅ 2026-08-13 已补齐**：`desktop-release.yml`，desktop-v* tag 触发） | 见 R-P1-2 |
| **桌面端前端门禁链不全** | typecheck / lint / vitest / e2e 已在 CI desktop job（`ci.yml`），但 **`pnpm build`（vite 生产构建）缺位**（2026-08-13 已补齐）；`make desktop-test` 只跑 Go 测试 | CI 补 `pnpm build` 门禁；`make desktop-test` 聚合 Go 测试 + 前端四门禁（M-DSK-TST-006） |
| **Linux 桌面构建缺失** | 官方模板三平台，当前仅 macOS 成熟 | P2 后置（先保证 macOS 可用 + Windows 可构建不 panic，M-DSK-PKG-002） |

---

## 5. 签名、公证与分发

### 5.1 社区标准流程

**macOS（Wails 官方签名指南）**：
1. 证书以 base64 p12 存 CI Secrets（`APPLE_DEVELOPER_CERTIFICATE_P12_BASE64` + `_PASSWORD` + `APPLE_PASSWORD`）
2. `Apple-Actions/import-codesign-certs@v1` 导入
3. `gon`（Mitchell Hashimoto 的 Go 工具）执行签名 + notarize，配置 `build/darwin/gon-sign.json` + `gon-notarize.json`
4. 权限在 `entitlements.plist` 声明（沙箱/网络/文件选择）
5. 现代做法（Mady 已采用）：`notarytool`（altool 已废弃，Wails issue #3290）

**Windows**：
- 标准代码签名证书（EV 非必需，除非内核驱动）
- `signtool sign /fd sha256 /tr <timestamp-server> /f cert.pfx`；注意部分供应商要求 `/tr`（时间戳），通用 GitHub Action 可能不支持
- Wails `-nsis` 出安装器（opskat 模式）

### 5.2 Mady 现状

- `Makefile desktop-notarize`：`codesign --timestamp --options=runtime` → `notarytool submit --wait` → `stapler staple` → `spctl --assess` ✅ 符合社区标准
- `build/darwin/Info.plist` 存在 ✅
- **差距（P1）**：~~未自动化进 CI~~ **✅ 2026-08-13 已进 `desktop-release.yml`**（macOS 内嵌 import-codesign-certs + Makefile 公证链路，凭据驱动跳过）；entitlements.plist 尚缺（网络/文件访问权限未声明），Windows 证书待配置后联调。

---

## 6. 自动更新

### 6.1 社区现状

| 路线 | 方案 | 备注 |
|------|------|------|
| Wails v2 社区 | `rhysd/go-github-selfupdate`（RiftShare 实测） | GitHub Release 为源，semver 比较，Windows/Linux 直接替换二进制；macOS 用 curl 下载 zip + ditto 解压替换 .app |
| Wails v3 | **内置 Updater**（官方 tutorial 04） | v3 已把自动更新做进框架 |
| Tauri v2 | `@tauri-apps/plugin-updater` + minisign 签名 + 静态 JSON 清单 | tauri-action 自动生成 updater JSON，CDN 化 |
| Electron | electron-updater（electron-builder 生态） | 最成熟，但机制同上：版本清单 + 签名 + 差分下载 |

**共识模式**：`检测版本（远端清单）→ semver 比较 → 下载 → 校验签名 → 替换 → 重启`。更新源 = GitHub Release（开源项目默认）。

### 6.2 Mady 现状与建议

- `desktop/app_update.go` 已存在（版本检查通道），`Health().Version` 已有 ✅
- `docs/plans/desktop-autoupdate-assessment.md` 已有专项评估 ✅
- **建议（P1，与 6 章节联动）**：若采用 RiftShare 模式（`go-github-selfupdate`），需配套：
  1. 发版流水线把各平台二进制作为 Release asset（第 4 章 gap 的产出物）
  2. `app_update.go` 的 Version 常量改为 ldflags 注入（避免与 wails.json 双源漂移）
  3. macOS 更新走 .zip + .app 替换（RiftShare `DoSelfUpdateMac` 模式），注意签名校验

---

## 7. 跨平台打包资产

社区标准目录（RiftShare 为范本，Wails build/ 目录约定）：

```
build/
├── appicon.png                  # 通用图标源（wails 会按平台派生）
├── darwin/
│   ├── Info.plist               # CFBundleIdentifier/Version/ShortVersionString
│   ├── entitlements.plist       # 沙箱+网络+文件权限声明
│   ├── gon-sign.json            # gon 签名配置（application_identity）
│   └── gon-notarize.json        # gon 公证配置（apple_id/team_id）
├── windows/
│   ├── icon.ico                 # exe 图标
│   ├── appxmanifest.xml         # Windows Store / MSIX 元数据
│   ├── RiftShare.exe.manifest   # 清单（DPI 感知等）
│   └── mapping.txt / priconfig.xml  # 资源打包映射
└── linux/
    ├── *.desktop                # 桌面入口（Name/Exec/Icon）
    └── *.metainfo.xml           # AppStream 元数据（Linux 软件商店标准，含截图/许可/发布说明）
```

**Mady 差距（P1/P2）**：
- `build/darwin/Info.plist` ✅；`entitlements.plist` ❌（P1）
- `build/windows/` 目录 ❌（P2，当前仅 `main_windows.go` 构建不 panic 级别）
- `build/linux/` ❌（P2）
- `build/appicon.png` ✅

---

## 8. 测试门禁流程

社区桌面项目测试分层共识（Electron/Tauri/Wails 一致）：

| 层 | 工具 | 社区做法 |
|----|------|---------|
| Go/核心逻辑 | `go test -race` | 绑定方法、生命周期、事件透传（Mady ✅） |
| 前端纯逻辑 | Vitest（node） | store/reducer/解析器（Mady ✅） |
| 组件 | Vitest + jsdom + Testing Library | **必须**用 `getByRole` 优先、`user-event`、web-first 断言（M-DSK-TST-002 缺口未闭合，见下） |
| e2e | Playwright | 关键用户路径（AC-1~AC-5）；Wails 桌面 e2e 走 webkit/chromium 壳或 mock binding（Mady `a2ui_e2e_test.go` ✅） |
| CI 契约 | 生成物漂移校验 | `wailsjs` 生成类型 vs `backend.ts` 包装类型漂移检测（M-DSK-TST-005） |

**Mady 差距（P0/P1，与 4.3 联动）**：
- P0-1：`vitest.component.config.ts` 已建但组件测试环境需确认闭合（W4-T1 计划内）
- P0-2：`desktop/` 根目录已入库二进制 `Mady` / `desktop.exe`（42MB + 2.4MB）——社区标准是构建产物全部 gitignore，需清理（W4-T2）
- P1：vite 生产构建门禁已补（2026-08-13，见 R-P1-1）；`make desktop-test` 已聚合 Go + 前端四门禁

---

## 9. 跨平台桌面 UX 检查表

### 9.1 Windows UX Checklist（Microsoft Win32 官方，经典必查）

- 窗口在 **96 DPI (800×600) / 120 DPI (1024×768) / 144 DPI (1200×900)** 三种模式测试，布局不破、文本不截断
- 窗口默认尺寸、最小尺寸合理；可调整大小则内容随之适配
- 标题栏/菜单栏/工具栏/状态栏齐全且语义正确；对话框按钮位置遵循平台惯例
- 错误信息可操作（说明原因 + 给出修复路径），不弹空白错误
- 勿用「帮助我」/「下一步」式空向导

### 9.2 Apple HIG（macOS 关键项，Mady 主平台）

- 菜单栏完整：应用菜单（About/Quit）、编辑菜单（Undo/Redo/Cut/Copy/Paste，⌘Z/⇧⌘Z）、窗口菜单；快捷键跟随系统惯例
- 工具栏：可自定义/可收起；拖放语义（同容器移动/跨容器复制）
- 对话框按钮：动词短语 + 破坏性按钮非默认 + Esc/⌘. 取消（M-DSK-IX-010 已对齐 ✅）

### 9.3 跨平台一致性（todesktop / 社区实践）

- **视觉品牌可统一，交互语法必须入乡随俗**：macOS 菜单在顶部系统栏、Windows 在窗口内；快捷键 Cmd vs Ctrl 自动跟随（Mady 已有 `main_windows.go` build tag 意识，M-DSK-PKG-002）
- 通知：不发送指示用户做具体任务的打扰式通知；长任务完成通知用系统原生（M-DSK-PKG-005 P2）

### 9.4 「适合用户使用」专项检查（对 Mady 的启示）

结合社区实践与本项目已有规范，用户可用性优先项：

1. **首次启动体验**：启动不弹窗（M-DSK-IX-009 ✅）；首页应有明确的「空状态引导」（未开案件时引导创建/导入案件，参照桌面专业工具的 empty state 惯例）
2. **状态可见性**：长分析任务必须有进度反馈 + 可取消（M-DSK-PRF-008）；流式输出批渲染（M-DSK-PRF-001）
3. **窗口状态记忆**：尺寸/位置/布局比例持久化（Mady `window_state.go` ✅；W4-T13 布局比例待做）
4. **崩溃恢复**：Checkpoint/会话恢复（agentcore 已有能力，桌面端接线）
5. **诊断可获取**：日志位置明确、`Help → 查看日志` 入口（社区桌面应用标准配置）

---

## 10. Mady 差距分析与建议行动

> 优先级定义沿用 `mady-desktop-standards.md` §14.2：P0=阻塞/修复必做，P1=近期重要，P2=后续。

### P0（发布前必须）

| ID | 差距 | 建议 |
|----|------|------|
| R-P0-1 | `desktop/` 入库 42MB 二进制 `Mady`/`desktop.exe` | `git rm --cached` + `.gitignore` 补 `desktop/Mady`、`desktop/desktop.exe`、`desktop/frontend/dist` 等（与 M-DSK-WLS-003、W4-T2 同项） |

### P1（本轮优化核心，按依赖顺序）

| ID | 差距 | 建议 | 依赖 |
|----|------|------|------|
| R-P1-1 | 桌面端前端门禁链缺 `pnpm build` | **✅ 2026-08-13 已落地**：`ci.yml` desktop job 补 vite 生产构建门禁；`make desktop-test` 聚合 Go + typecheck + lint + vitest + build | — |
| R-P1-2 | 无桌面端发布流水线 | **✅ 2026-08-13 已落地**：`.github/workflows/desktop-release.yml`（`desktop-v*` tag 触发，与 CLI `v*` 互不冲突）——matrix 构建（macos universal / windows amd64+NSIS）→ 凭据驱动签名公证 → 上传制品 → release job 建草稿（actionlint 校验通过） | — |
| R-P1-3 | 签名公证未自动化 | **🔄 已接线 + 门禁加固（2026-08-13）**：macOS 内嵌 import-codesign-certs + `make desktop-notarize`（notarytool 链路，尾部 spctl 校验）；Windows signtool 签名 + `verify /pa` 校验；门禁一致性加固（公证步骤要求 p12 证书 + Apple ID 同时存在，缺证书跳过而非误导性失败）；macOS 构建链路本地端到端验证通过（wails build + ldflags 注入 + 打包 + 自签名）。**剩余：真实证书配置后 CI 联调（外部依赖，需 Developer ID 证书 + Apple ID）** | R-P1-2 ✅ |
| R-P1-4 | entitlements.plist 缺失 | **✅ 2026-08-13 已落地**：`desktop/build/darwin/entitlements.plist`（hardened runtime + WKWebView JIT 三例外，**非沙箱**——沙箱与任意工作区访问/MadyHome 数据目录不兼容），Makefile `desktop-notarize` codesign 已接 `--entitlements`；`plutil -lint` 通过 | R-P1-3 |
| R-P1-5 | 版本多源漂移 | **✅ 2026-08-13 已落地**：`scripts/bump-desktop-version.sh`（wails.json → app_update.go 默认值 → frontend/package.json 三处联动，构建期 Info.plist 由 Wails 模板自动生成无需手改）；发布版本经 ldflags 注入（R-P1-2 已接） | — |
| R-P1-6 | 自动更新未闭环 | **🔄 阶段一落地（2026-08-13）**：`CheckUpdate()` 由占位升级为**真实检测**（GitHub Releases desktop-v* tag + semver 比较 + 下载地址返回，10 个单测含 httptest mock）；设置面板「检查更新」发现新版本时展示「打开下载页」入口（BrowserOpenURL，http/https 白名单）。**自替换刻意后置**：公证未落地前不引入 go-github-selfupdate 自替换（供应链风险面 + hardened runtime 破坏签名，见 autoupdate-assessment 阶段二） | R-P1-2, R-P1-5 |
| R-P1-7 | 首次使用空状态引导 | **✅ 已有实现覆盖（2026-08-13 核查）**：ChatView 空态（`!hasActiveContent`）即「开始新对话」+ 品牌光晕 + 领域快捷引导（专利新颖性分析/权利要求撰写/OA 答复策略），符合 Chatbox/Jan onboarding 模式；CWD 瞬态项目保证始终有工作区上下文（AGENTS.md）。无需重复建设，后续可按需增强（如首次启动欢迎语） | — |

### P2（后续）

| ID | 差距 | 建议 |
|----|------|------|
| R-P2-1 | Windows 打包资产目录 | `build/windows/`（icon.ico / manifest / appxmanifest）+ `wails build -nsis` |
| R-P2-2 | Linux 构建与分发 | `build/linux/`（.desktop + AppStream metainfo.xml），CI 增 ubuntu job |
| R-P2-3 | 业务下沉 `desktop/internal/` | 更新检查/设置校验等纯业务移出绑定文件（架构模式 A 对齐） |
| R-P2-4 | 托盘 + 长任务通知 | M-DSK-PKG-005（W4-T11） |
| R-P2-5 | 崩溃恢复接线 | 会话恢复入口 + 日志查看入口 |

### 建议执行顺序（与 `desktop-next-development-plan.md` 四波计划衔接）

```
R-P0-1 ──► R-P1-1 ──► R-P1-2 ──► R-P1-3/4 ──► R-P1-5 ──► R-P1-6 ──► R-P1-7
(清理)    (CI门禁)   (发布管线)   (签名公证)    (版本单源)  (自动更新)  (空状态)
   └── 与 W4（规范对齐与质量门禁）并行，R-P1-1/2/5 落地即 W4 门禁闭环
```

---

## 11. 参考来源

### 官方规范（一手来源）

- Wails v2.13 官方文档：Application Development / Crossplatform build with Github Actions / Code Signing（含 RiftShare 参考） / Single Instance Lock / NSIS installer / Manual Builds
- Wails v3：Self-Updating Wails App（官方 tutorial 04）
- Tauri v2：Distribute → GitHub Pipelines（tauri-action 标准流水线）、Updater 文档
- Apple HIG（Human Interface Guidelines）
- Microsoft Win32 UX Guide：UX checklist for desktop applications（top-violations）
- Playwright Best Practices / Vitest / Testing Library 官方

### 开源案例（实测核验）

- [achhabra2/riftshare](https://github.com/achhabra2/riftshare)：Wails v2 完整工程（internal/ 分层、三平台打包资产、CI 签名发布、go-github-selfupdate 自动更新、bump-version.sh）
- [safing/portmaster](https://github.com/safing/portmaster)：Wails→Tauri 迁移案例，胖核心+薄壳架构
- [opskat/opskat](https://github.com/opskat/opskat)：活跃 Wails 多平台 CI（-nsis / ldflags 注入 / Apple 证书条件导入）
- tiny-rdm / RWKV-Runner / jcp / WailBrew / Image-Studio：见 `mady-desktop-standards.md` §13 已调研

### 项目内关联文档

- `docs/mady-desktop-standards.md`（设计/工程规范 v1.0，本调研的规范侧基准）
- `docs/plans/desktop-next-development-plan.md`（四波开发计划）
- `docs/plans/desktop-autoupdate-assessment.md`（自动更新专项评估）
- `docs/plans/desktop-notarization-assessment.md`（公证专项评估）
- `docs/plans/desktop-i18n-assessment.md` / `desktop-reasonix-reference-plan.md`
