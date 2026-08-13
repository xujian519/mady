//go:build darwin

package main

// app_update.go — 自动更新通道（R-P1-6，W4-T12 / M-DSK-PKG-003）。
//
// 阶段定位（见 docs/plans/desktop-autoupdate-assessment.md）：
//   阶段一（当前）：真实版本检测 + 手动下载引导。CheckUpdate 查询 GitHub Releases
//     （desktop-v* tag），与当前版本比较，发现新版本时返回下载地址供手动安装；
//     不执行自替换（公证未落地前，二进制热替换会破坏签名；且自替换属供应链高风险面）。
//   阶段二（公证后）：manifest（版本 + sha256 + 签名）+ 下载校验 + 整包替换 .app。
//
// desktopVersion 默认值与 desktop/wails.json 的 productVersion 保持一致（0.1.0）；
// 发布管线通过 ldflags 注入真实版本：
//   wails build -ldflags "-X main.desktopVersion=$(VERSION)"
// （Makefile desktop-dmg / desktop-build-quick / CI desktop-release.yml 已注入）。
//
// 更新源 = GitHub Releases（xujian519/mady），与 CI 发布流水线（desktop-release.yml）
// 产出的 Release asset 对齐。以下变量为包级、可在测试中覆盖。

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// desktopVersion 为运行版本号。必须是 var 而非 const，ldflags 才能覆盖；
// 未注入（本地开发 wails dev / go build）时回退到 wails.json 的 productVersion。
var desktopVersion = "0.1.0"

// 更新检查的远端配置（包级变量，测试中可替换 ghAPIBase 指向 httptest 服务）。
var (
	ghAPIBase   = "https://api.github.com"
	ghOwner     = "xujian519"
	ghRepo      = "mady"
	ghUserAgent = "mady-desktop"
	// ghCheckTimeout 单次更新检查的网络超时。
	ghCheckTimeout = 10 * time.Second
)

// UpdateInfo 描述一次更新检查的结果。
type UpdateInfo struct {
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion"`
	HasUpdate      bool   `json:"hasUpdate"`
	DownloadURL    string `json:"downloadUrl,omitempty"`
	Message        string `json:"message"`
}

// ghRelease 是 GitHub Releases API 响应的最小投影（只取用到的字段）。
type ghRelease struct {
	TagName    string    `json:"tag_name"`
	HTMLURL    string    `json:"html_url"`
	Prerelease bool      `json:"prerelease"`
	Assets     []ghAsset `json:"assets"`
}

type ghAsset struct {
	BrowserDownloadURL string `json:"browser_download_url"`
}

// CheckUpdate 检查是否有可用更新。
//
// 行为契约：
//   - App 未就绪（a.server == nil）→ 返回错误（前端转错误 toast）。
//   - 网络/API 失败 → 返回错误（诚实告知「无法连接更新通道」，不做「已是最新」误报）。
//   - 无任何 desktop-v* 发布 → 返回无更新 + 明确说明（发布流水线尚未使用）。
//   - 远端版本 ≤ 当前版本 → 无更新。
//   - 远端版本 > 当前版本 → HasUpdate=true + 下载地址（优先 macOS zip asset，
//     回退 Release 页），Message 附引导文案（M-1 中间态措辞：给出替代路径而非空洞提示）。
func (a *App) CheckUpdate() (UpdateInfo, error) {
	if err := a.ready(); err != nil {
		return UpdateInfo{}, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), ghCheckTimeout)
	defer cancel()

	latest, err := fetchLatestDesktopRelease(ctx)
	if err != nil {
		return UpdateInfo{}, fmt.Errorf("检查更新失败（无法连接更新通道）：%w", err)
	}
	if latest == nil {
		return UpdateInfo{
			CurrentVersion: desktopVersion,
			LatestVersion:  desktopVersion,
			HasUpdate:      false,
			Message:        "暂无可用更新（桌面端发布通道尚未启用）；当前版本 " + desktopVersion,
		}, nil
	}

	latestVersion := strings.TrimPrefix(strings.TrimPrefix(latest.TagName, "desktop-"), "v")
	if compareVersions(desktopVersion, latestVersion) >= 0 {
		return UpdateInfo{
			CurrentVersion: desktopVersion,
			LatestVersion:  latestVersion,
			HasUpdate:      false,
			Message:        "已是最新版本 v" + desktopVersion,
		}, nil
	}
	return UpdateInfo{
		CurrentVersion: desktopVersion,
		LatestVersion:  latestVersion,
		HasUpdate:      true,
		DownloadURL:    updateDownloadURL(latest),
		Message:        "发现新版本 v" + latestVersion + "（当前 v" + desktopVersion + "），可在下载页获取安装包",
	}, nil
}

// fetchLatestDesktopRelease 查询 GitHub Releases，返回最新一条 desktop-v* 发布；
// 没有任何 desktop 发布时返回 (nil, nil)。
func fetchLatestDesktopRelease(ctx context.Context) (*ghRelease, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases?per_page=50", ghAPIBase, ghOwner, ghRepo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", ghUserAgent)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub Releases API 返回状态 %d", resp.StatusCode)
	}

	var releases []ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, err
	}
	// 按 API 返回顺序（新→旧）取第一条 desktop-v* 正式发布。
	// prerelease（如 desktop-v0.2.0-beta）不推送给用户——预发布只用于灰度联调，
	// 一旦推送给终端用户会被误当正式版安装。
	for i := range releases {
		if releases[i].Prerelease {
			continue
		}
		if strings.HasPrefix(releases[i].TagName, "desktop-v") {
			return &releases[i], nil
		}
	}
	return nil, nil
}

// updateDownloadURL 优先返回 macOS zip 分发包地址（与 desktop-release.yml 的
// Mady-<version>-macos-universal.zip 对齐），否则回退到 Release 页面。
func updateDownloadURL(r *ghRelease) string {
	for _, a := range r.Assets {
		if strings.HasSuffix(a.BrowserDownloadURL, ".zip") {
			return a.BrowserDownloadURL
		}
	}
	return r.HTMLURL
}

// compareVersions 比较两个版本号（支持 v 前缀与 desktop- 前缀，仅主.次.补丁）。
// 返回 -1 / 0 / 1；任一版本无法解析时视为相等（保守不误报更新）。
func compareVersions(a, b string) int {
	pa, okA := parseVersion(a)
	pb, okB := parseVersion(b)
	if !okA || !okB {
		return 0
	}
	for i := 0; i < 3; i++ {
		if pa[i] < pb[i] {
			return -1
		}
		if pa[i] > pb[i] {
			return 1
		}
	}
	return 0
}

// parseVersion 解析 x.y.z（允许 2 段）为 [3]int；预发布后缀忽略。
func parseVersion(s string) ([3]int, bool) {
	s = strings.TrimPrefix(s, "desktop-")
	s = strings.TrimPrefix(s, "v")
	core := strings.SplitN(s, "-", 2)[0]
	parts := strings.Split(core, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return [3]int{}, false
	}
	var v [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return [3]int{}, false
		}
		v[i] = n
	}
	return v, true
}
