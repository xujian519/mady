# macOS 公证（Notarization）评估 — Mady Desktop

> 对应 `desktop-next-development-plan.md` W3-T3（缺口 G7）。
> 状态：**评估完成，待账号就绪后执行**（2026-07-31）。

## 1. 现状

- 桌面端当前构建产物为 **ad-hoc 签名**（`wails build` 默认行为），
  未公证分发。用户首次运行需手动解除隔离（`xattr -cr`）。
- 已具备：完整 `.app` Bundle（`desktop/build/bin/Mady.app`）、
  `desktop-dmg` Makefile 目标（universal 构建）、App Icon。
- Bundle 标识符：`com.mady.desktop`（`build/Info.plist`）。

## 2. 前置条件

| 项 | 要求 | 当前状态 |
|----|------|----------|
| Apple Developer Program 账号 | $99/年，需申请 | ⏳ 未确认 |
| Developer ID Application 证书 | 用于签名（`Developer ID Application: <Team>`） | ⏳ 未确认 |
| Xcode 16+ `notarytool` | 替代已废弃的 altool | ✅（macOS 13+ 均可用） |
| App-specific password / API Key | 公证鉴权（`--apple-id` + `--password` 或 `--apiKey`） | ⏳ 未配置 |

## 3. 公证流程

```bash
# 1. 签名（Developer ID 证书）
codesign --deep --force --options runtime \
  --sign "Developer ID Application: <TEAM>" \
  ./desktop/build/bin/Mady.app

# 2. 公证（notarytool，替代 altool）
xcrun notarytool submit ./desktop/build/bin/Mady.app \
  --apple-id "$APPLE_ID" \
  --team-id "$TEAM_ID" \
  --password "$APP_PASSWORD" \
  --wait

# 3. 装订（staple，离线可验）
xcrun stapler staple ./desktop/build/bin/Mady.app

# 4. 验证
spctl --assess --type execute --verbose ./desktop/build/bin/Mady.app
```

环境变量（CI 或本地 shell 注入，**不提交到仓库**）：

| 变量 | 用途 |
|------|------|
| `APPLE_ID` | Apple 开发者账号邮箱 |
| `TEAM_ID` | Team ID（开发者后台可查） |
| `APP_PASSWORD` | App-specific password（开发者后台生成） |

## 4. 沙箱策略说明

`build/Info.plist` 当前**未启用** App Sandbox（`com.apple.security.app-sandbox` 未设置）。

- **启用沙箱**：需在 `Info.plist` 加 `com.apple.security.app-sandbox: true` +
  `com.apple.security.files.user-selected.read-write: true`（项目文件夹选择）。
  启用后 Wails WebView 正常；文件对话框经 NSOpenPanel 仍可用。
  **注意**：沙箱开启后 `~/Library/Caches/mady/` 与 `~/.mady/` 写入路径
  受容器重定向影响（`~/Library/Containers/com.mady.desktop/`），
  需验证 `window_state.go` 与 `app_settings.go` 的路径解析。
- **不启用沙箱**：直接公证分发即可；App Store 上架才强制沙箱。
  Mady 通过开发者网站/官网分发（非 App Store），**建议本期不启用沙箱**，
  保持文件系统自由度（项目目录任意位置）。

## 5. 降级方案（无开发者账号时）

- README 增加"首次运行"说明：`xattr -cr Mady.app` 解除隔离属性
- `desktop-run` 后输出引导提示

## 6. 风险与注意

| 风险 | 缓解 |
|------|------|
| 公证需稳定网络（上传 ~40MB 二进制） | CI 中执行；本地可跳过 |
| 证书过期/吊销 | 定期检查 Developer 后台 |
| 公证后修改 Bundle 导致签名失效 | 先改代码再签名公证，顺序固定 |
| Gatekeeper 新版本收紧 | 跟随 Apple 政策；保留 xattr 降级路径 |

## 7. 待办

- [ ] 确认 Apple Developer Program 账号状态
- [ ] 生成 Developer ID Application 证书并安装到 keychain
- [ ] 配置 `APPLE_ID` / `TEAM_ID` / `APP_PASSWORD` 环境变量
- [ ] 执行 `make desktop-notarize`（签名 + 公证 + 装订）
- [ ] 首次运行验证（下载/解压/启动/文件访问）
