# 自动更新评估 — Mady Desktop

> 对应 `docs/plans/desktop-next-development-plan.md` W4-T12（缺口 P2-6，`docs/mady-desktop-standards.md` M-DSK-PKG-003）。
> 状态：**评估完成**（2026-07-31）。
> 对应验收标准：① 评估报告输出（三方案对比 + 推荐，本文档）✅；② `CheckUpdate()` 绑定占位存在（返回「当前版本」即可）→ 本期实施范围第 4 章。

---

## 0. 结论速览（TL;DR）

| 项 | 结论 |
|----|------|
| 现状 | `Health().Version` 字段已存在（`server/desktop.go`），但**当前恒返回 `"unknown"`**（未注入构建信息）；前端版本展示来源混杂（SettingsPanel 硬编码 `0.1.0` / StatusBar 兜底 `0.1.0`）；Wails v2 无官方 autoupdate（[wailsapp/wails issue #1178](https://github.com/wailsapp/wails/issues/1178) 仅为社区讨论） |
| 三方案 | ① Sparkle（macOS 原生框架）；② 自实现（HTTP manifest + 校验 + 替换，参考 RWKV-Runner / jcp 模式）；③ GitHub Releases + 手动下载引导 |
| 推荐 | **分阶段**：本期 `CheckUpdate()` 占位 + GitHub Releases 手动引导；公证（W3-T3）决策后下一迭代做**自实现更新通道**；Sparkle 暂不选（macOS 专属 + 无官方 Go 绑定 + 强依赖 Developer ID 前置） |
| 关键约束 | 公证（M-DSK-PKG-001）落地后，**单二进制内替换会破坏签名/公证密封** → 更新必须**整包替换 `.app`**；本期占位不受影响 |
| 工作量 | 占位 0.5-1 人天；完整自实现 3-5 人天；Sparkle 4-6 人天 |

---

## 1. 现状

### 1.1 `Health().Version` 实际结构

| 层 | 位置 | 实际情况 |
|----|------|---------|
| 后端定义 | `server/desktop.go` `HealthInfo` | `type HealthInfo struct { Provider string; Model string; Version string; Uptime string }`（json tag 与前端 `backend.ts` 镜像一致） |
| 后端取值 | `server/desktop.go` `func (s *Server) Health()` | `Version: "unknown"` —— **恒为占位字符串，未注入 commitHash/buildTime** |
| 桌面绑定 | `desktop/app.go` `func (a *App) Health() (HealthInfo, error)` | 透传 `a.server.Health()` |
| 前端消费 | `desktop/frontend/src/lib/backend.ts` `health()` + `components/StatusBar.tsx` | `v{info?.version ?? '0.1.0'}`；`components/SettingsPanel.tsx`「关于」区**硬编码 `0.1.0`** |

版本注入现状：根模块 `Makefile` 已配 `-ldflags "-s -w -X main.commitHash=$(COMMIT_HASH) -X main.buildTime=$(BUILD_TIME)"`，注入目标为 `cmd/mady/main.go` 的包级变量；**`desktop/` 模块的 `main` 包没有对应的 `commitHash`/`buildTime` 变量**，也未注入 `server.Health()`。`wails.json` 的 `productVersion` 为 `0.1.0`。

**结论**：计划的「版本接口复用 `Health().Version`（commitHash + buildTime）」方向可行，但**字段当前是空转的**——实施 `CheckUpdate()` 占位前需先补版本注入（desktop 模块加 ldflags 变量 → 注入 `Server`）。

### 1.2 Wails v2 无官方 autoupdate

- 事实：Wails v2 官方文档/代码中无自更新能力；[wailsapp/wails issue #1178 "Support Self-Updating"](https://github.com/wailsapp/wails/issues/1178) 是公开社区讨论（结论倾向以官方可选库形式存在，一直未落地）。本项目 `docs/mady-desktop-standards.md` M-DSK-PKG-003 亦基于此判断。
- 生态参考（§13.1 已收录）：
  - [RWKV-Runner](https://github.com/josStorer/RWKV-Runner)（6.4k⭐）：Go 后端内置自动更新 + 保留用户配置；
  - [jcp](https://github.com/run-bigpig/jcp)（1.3k⭐）：前端 `updateService.ts` 更新服务（Wails 同栈，Mady 相似度最高）。

---

## 2. 三方案对比

> 外部事实核实时间 2026-07-31：Sparkle 特性经 [sparkle-project 官网](https://sparkle-project.org/) 检索核实；`minio/selfupdate` 经 [GitHub 仓库](https://github.com/minio/selfupdate) / [pkg.go.dev](https://pkg.go.dev/github.com/minio/selfupdate) 检索核实。

### 方案一：Sparkle（macOS 原生更新框架）

| 项 | 说明 |
|----|------|
| 原理 | 应用内嵌 Sparkle 框架，启动时读取开发者维护的 **appcast XML**（含版本号、下载地址、EdDSA 签名），发现新版本后下载、校验、整包安装并重启；支持二进制增量（delta）更新与自定义 UI |
| 优点 | macOS 原生体验（系统弹窗/安装交互标准）；**正确处理签名+公证应用的整包替换**（新版本是开发者签好名的完整 `.app`）；增量更新省流量；开源（MIT）且久经考验 |
| 缺点 | **仅 macOS**（Windows/Linux 需另一套机制）；无官方 Go 绑定，Wails（Go + WebView）集成需 cgo + Objective-C/Swift 桥接或独立 helper；**强依赖 Developer ID 签名 + 公证**（当前 W3-T3 公证前置条件未就绪：Developer ID 证书未确认）；密钥/证书管理成本高 |
| 对 Mady 适配度 | **低-中**：单二进制（15-25MB 叙事，M-DSK-PKG-007）是 `Mady.app` 内含单个可执行文件，Sparkle 更新的是整个 `.app` 包——与单二进制叙事不冲突但更新体量回到整包级；公证流程（M-DSK-PKG-001）是 Sparkle 的硬前置，目前不具备 |

### 方案二：自实现（HTTP 拉取 manifest + 校验 + 替换二进制）

| 项 | 说明 |
|----|------|
| 原理 | 应用内置更新检查：GET 发布服务上的 JSON manifest（最新版本号 / 下载 URL / sha256 / Ed25519 签名）→ 下载新包 → 校验 → 替换本地二进制 → 重启；参考 RWKV-Runner「内置更新 + 保留用户配置」与 jcp `updateService.ts` 模式 |
| 优点 | **跨平台统一**（Windows P2 后置仍可复用）；不依赖 Apple 生态；实现可控（可复用 [minio/selfupdate](https://github.com/minio/selfupdate)——面向单文件目标的自更新库，下载校验后替换，原始实现源自 inconshreveable/go-update）；与单二进制分发模式天然契合；manifest 服务自主可控（GitHub Releases 或自建静态站均可） |
| 缺点 | 需自建安全链路（HTTPS + 校验 + 签名），出问题即供应链风险；**公证后二进制内替换会使代码签名失效**——已公证应用必须改为整包替换并重新公证（见第 5 章），复杂度上升；无增量更新 |
| 对 Mady 适配度 | **中-高**：当前 ad-hoc 签名（未公证）阶段，二进制替换路径完全可行；公证落地后切换为整包替换 + 下载后本地 `codesign --verify` + `spctl --assess` 校验新包；用户配置（`~/.mady/` 下 `desktop-settings.json`、sessions、workspace）**天然在 `.app` 之外**，替换不丢配置 |

### 方案三：GitHub Releases + 手动下载引导

| 项 | 说明 |
|----|------|
| 原理 | 发版走 GitHub Releases（`desktop-dmg` 产物 + `desktop-notarize` 公证后上传）；应用内「检查更新」提示最新版本号并引导打开 Releases 页 / 下载地址手动安装 |
| 优点 | 零更新代码（仅占位接口 + 跳转）；无供应链代码风险；实现最快；与现有 `desktop-dmg`/`desktop-notarize` 工作流无缝衔接 |
| 缺点 | UX 割裂（用户需手动下载、关闭应用、替换）；不会自动更新；下载页即发布页，无签名校验的自动路径 |
| 对 Mady 适配度 | **高**（作为本期过渡）；「检查更新」占位天然落在方案二的接口上，后续平滑升级 |

---

## 3. 推荐方案与理由

**推荐路径（分阶段）：**

1. **本期（W4-T12）**：`CheckUpdate()` 占位（返回当前版本）+ 设置面板「检查更新」入口 + 版本注入修复 + GitHub Releases 手动引导。投入 0.5-1 人天，满足验收标准。
2. **下一迭代（公证 W3-T3 决策后）**：实现**自实现更新通道**——manifest（版本 + sha256 + Ed25519 签名）+ 下载 + 校验 + 替换；公证落地则整包替换 `.app`，未公证则二进制替换。
3. **长期**：若 macOS 成为唯一或绝对主力分发渠道且公证就绪，评估引入 Sparkle 作为原生体验增强（保留自实现通道作为其他平台通道）。

**理由：**

- **与「克制」哲学一致**：Mady 当前连公证都未落地（W3-T3 前置：Developer ID 账号/证书未确认），直接上 Sparkle 是在未满足前置条件（签名 + 公证）的体系上做最重方案。
- **跨平台复用**：Windows 适配（W3-T4）已在计划内，自实现通道天然跨平台；Sparkle 只覆盖 macOS。
- **单一二进制叙事自洽**：M-DSK-PKG-007 把「Wails 单二进制 15-25MB」作为传播卖点；自实现通道可只更新单个二进制（未公证阶段）或整包（公证阶段），两种形态都在控制内。
- **风险可控**：占位接口先行，真实更新链路所需的发布服务/密钥/公证全部后置，不阻塞本期。

---

## 4. 预留实现（占位草案）

> 标注：以下为**接口草案**，本期仅实现「返回当前版本」的占位逻辑；真实更新链路（下载/校验/替换）不在本期范围。涉及新增/修改 Go 绑定方法，完成后需 `wails generate module` 重新生成 `wailsjs`（M-DSK-WLS-007），前端经 `backend.ts` 收敛调用（M-DSK-WLS-008）。

### 4.1 Go 侧（`desktop/app_update.go` 新文件）

```go
// CheckUpdateResult 描述一次更新检查的结果。
// 本期为占位：仅填充当前版本并返回 updateAvailable=false。
type CheckUpdateResult struct {
    CurrentVersion string `json:"currentVersion"`          // 当前运行版本（v0.1.0+commitHash）
    LatestVersion  string `json:"latestVersion"`            // 远端最新版本；未接入远端时与 CurrentVersion 相同
    UpdateAvailable bool   `json:"updateAvailable"`         // 是否有可用更新（本期恒 false）
    DownloadURL    string `json:"downloadUrl,omitempty"`    // 更新包下载地址（本期为空，用于手动引导）
    Message        string `json:"message"`                  // 面向用户文案 key 或文本（配合前端 i18n）
}

// CheckUpdate 检查是否存在新版本。
//
// 本期占位实现：返回当前版本信息与“已是最新”提示；
// 下一迭代接入自实现更新通道（manifest + sha256/Ed25519 校验 + 替换），
// 或根据分发渠道切换为 Sparkle / GitHub Releases 引导。
func (a *App) CheckUpdate(ctx context.Context) (CheckUpdateResult, error) {
    if err := a.ready(); err != nil {
        return CheckUpdateResult{}, err
    }
    v := "v" + a.version() // 占位：返回当前版本（来自注入的 commitHash/buildTime）
    return CheckUpdateResult{
        CurrentVersion:  v,
        LatestVersion:   v,
        UpdateAvailable: false,
        Message:         "checkUpdate.alreadyLatest", // i18n key（对照前端 P0 文案）
    }, nil
}
```

配套改动（版本注入）：

```go
// desktop/main.go（新增，参照 cmd/mady/main.go 的 ldflags 约定）
var (
    commitHash = "unknown" // 由 -ldflags "-X main.commitHash=$(COMMIT_HASH)" 注入
    buildTime  = "unknown" // 由 -ldflags "-X main.buildTime=$(BUILD_TIME)" 注入
)
```

`server.Health().Version` 由 `"unknown"` 改为注入值（`commitHash + buildTime`），`Makefile desktop-dmg` 追加同一组 `-ldflags`。

### 4.2 前端（`desktop/frontend/src/lib/backend.ts`）

```ts
/** 更新检查结果（对应 Go 侧 CheckUpdateResult）。 */
export interface CheckUpdateResult {
  currentVersion: string
  latestVersion: string
  updateAvailable: boolean
  downloadUrl?: string
  message: string
}

/** 检查更新。当前为占位：返回当前版本，updateAvailable 恒为 false。 */
export async function checkUpdate(): Promise<CheckUpdateResult> {
  return callBinding<CheckUpdateResult>('main/App', 'CheckUpdate')
}
```

### 4.3 设置面板入口位置

`desktop/frontend/src/components/SettingsPanel.tsx` **「关于」区**（当前 `0.1.0` 版本行下方，约第 207-227 行）新增一行「检查更新」按钮：

- 点击 → `checkUpdate()` → 有更新：显示新版本 + 「前往下载」（打开 `DownloadURL`，`openUrl` 仅允许 http/https，对照 M-DSK-SEC-003）；无更新：Toast「当前已是最新版本」。
- 「版本」行改为展示 `health().version`（修复硬编码 `0.1.0`），与 StatusBar 一致。

---

## 5. 安全注意

### 5.1 更新包签名校验（防供应链投毒）

自实现通道的最低安全底线（下一迭代实现时强制）：

1. **传输层**：manifest 与更新包一律 HTTPS（`http://` 拒绝）；
2. **完整性**：下载后校验 sha256 与 manifest 一致（可复用 `minio/selfupdate` 的「下载 → 校验 → 替换」流程）；
3. **来源认证**：**manifest 本身用项目发布 Ed25519 私钥签名**（公钥内置在应用内），杜绝「中间人改 manifest + 换包」；私钥只存 CI Secrets（M-DSK-SEC-008），禁止落仓库；
4. **发布侧校验**：公证阶段整包更新时，下载后本地执行 `codesign --verify --deep --strict` 与 `spctl --assess` 确认新 `.app` 签名链完整、公证有效，失败即丢弃；
5. **回滚**：替换前保留当前版本副本，新版本启动健康检查（`Health()` 探活）失败则回滚；
6. **权限**：更新进程非特权运行，临时下载目录权限收紧（`0700`），不落 `/tmp` 共享位。

### 5.2 与公证 / notarization 的关系（关键约束）

- **公证后单二进制内替换 = 签名失效**：M-DSK-PKG-001 要求 `codesign --timestamp --options=runtime`（hardened runtime）+ notarization。对已公证应用，把 `Mady.app` 内的可执行文件直接换掉会破坏代码签名密封（库校验/资源封口），Gatekeeper 在用户机器上会拦截启动（"已损坏"）。
- **推论**：公证落地后，任何更新通道（自实现或 Sparkle）都必须**以完整 `Mady.app`/DMG 为单位交付**，由发布方完成签名 + 公证，用户侧只做整包替换——这与 Sparkle 的机制一致，也是自实现方案公证阶段的实现形态。
- **当前阶段的利好**：应用现为 ad-hoc 签名（见 `docs/plans/desktop-notarization-assessment.md`），尚未公证，二进制替换路径暂时可行；但**不要**基于此放松设计——本期占位、下期实现时默认按「整包替换」设计。
- **用户配置安全**：`desktop-settings.json`、sessions、workspace 均落在 `~/.mady/`，不在 `.app` 内，任何形态的更新都不影响用户数据（与 RWKV-Runner「保留用户配置」对齐）。

---

## 6. 工作量估算

| 任务 | 人天 | 说明 |
|------|------|------|
| **本期占位**：`CheckUpdate()` 绑定 + `CheckUpdateResult` + 版本注入（ldflags 变量、`server.Health()` 接线）+ 前端 `checkUpdate()` + 「关于」区按钮 + 测试（`wails generate module` 后契约校验） | **0.5-1** | 对应 W4-T12 验收标准 2 |
| GitHub Releases 发布工作流（`desktop-dmg` + 公证产物上传 + release notes 模板） | 0.5-1 | 手动引导方案的配套 |
| 自实现更新通道（完整）：manifest 服务 + Go 更新器（`minio/selfupdate` 或自写）+ Ed25519 签名/校验 + 整包/二进制替换 + 回滚 + UI 状态（下载/进度/结果） | 3-5 | 建议公证决策（W3-T3）后 1 个迭代内 |
| Sparkle 集成（若最终选用）：框架内嵌 + cgo/ObjC 桥接 + appcast 生成 + 签名前置 + 公证回归 | 4-6 | 需 Developer ID 前置，本期明确不选 |

**排期建议**：本期（W4-T12）只做占位 + 手动引导；真实更新链路与公证（W3-T3）决策绑定，两者同迭代推进，避免为「未公证」形态写死二进制替换、公证落地后返工。

---

## 附：来源

- 项目内文档：
  - [docs/plans/desktop-next-development-plan.md](../plans/desktop-next-development-plan.md)（W4-T12 小节，P2-6）
  - [docs/mady-desktop-standards.md](../mady-desktop-standards.md)（M-DSK-PKG-001/003/007、M-DSK-SEC-008、§13.1 案例）
  - [docs/plans/desktop-notarization-assessment.md](../plans/desktop-notarization-assessment.md)（W3-T3，公证现状与前置）
  - `server/desktop.go`（`HealthInfo` / `Health()`）、`desktop/app.go`、`desktop/frontend/src/lib/backend.ts`、`components/StatusBar.tsx`、`components/SettingsPanel.tsx`、`Makefile`
- 外部事实（2026-07-31 核实；GitHub API 当日限流，改用本地 SearXNG 检索 + npm/GitHub 页面核对）：
  - [Sparkle — software update framework for macOS（GitHub）](https://github.com/sparkle-project/Sparkle) — MIT 开源；Sparkle 2 支持 App Sandbox、自定义 UI、EdDSA（ed25519）签名 + Apple Code Signing、delta 更新；需签名应用，macOS 专属
  - [Sparkle 文档（sparkle-project.org）](https://sparkle-project.org/documentation/) — 更新归档（dmg/zip）需 EdDSA 签名
  - [wailsapp/wails issue #1178 "Support Self-Updating"](https://github.com/wailsapp/wails/issues/1178) — Wails 生态无官方 autoupdate 的公开佐证（社区讨论状态）
  - [minio/selfupdate — Build self-updating Go programs](https://github.com/minio/selfupdate) / [pkg.go.dev](https://pkg.go.dev/github.com/minio/selfupdate) — 单文件目标自更新、校验后原子替换
  - [RWKV-Runner（GitHub）](https://github.com/josStorer/RWKV-Runner) — 内置自动更新 + 保留用户配置（案例来源，§13.1）
  - [jcp（GitHub）](https://github.com/run-bigpig/jcp) — Wails 同栈，前端 `updateService.ts` 更新服务（案例来源，M-DSK-PKG-003）
