//go:build darwin

package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	madyserver "github.com/xujian519/mady/server"
)

// withMockGHAPI 将 ghAPIBase 指向 handler 提供的 httptest 服务，并还原。
func withMockGHAPI(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	oldBase := ghAPIBase
	ghAPIBase = srv.URL
	t.Cleanup(func() { ghAPIBase = oldBase })
}

func mustEncodeReleases(t *testing.T, releases []ghRelease) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/xujian519/mady/releases" {
			http.Error(w, "unexpected path: "+r.URL.Path, http.StatusNotFound)
			return
		}
		if err := json.NewEncoder(w).Encode(releases); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

// TestFetchLatestDesktopRelease_skipsNonDesktopTags：v* 的 CLI 发布必须被跳过，
// 只认 desktop-v* tag。
func TestFetchLatestDesktopRelease_skipsNonDesktopTags(t *testing.T) {
	withMockGHAPI(t, mustEncodeReleases(t, []ghRelease{
		{TagName: "v0.3.0", HTMLURL: "https://github.com/xujian519/mady/releases/tag/v0.3.0"},
		{
			TagName: "desktop-v0.2.0",
			HTMLURL: "https://github.com/xujian519/mady/releases/tag/desktop-v0.2.0",
			Assets: []ghAsset{{
				BrowserDownloadURL: "https://github.com/xujian519/mady/releases/download/desktop-v0.2.0/Mady-0.2.0-macos-universal.zip",
			}},
		},
		{TagName: "desktop-v0.1.0", HTMLURL: "https://github.com/xujian519/mady/releases/tag/desktop-v0.1.0"},
	}))

	got, err := fetchLatestDesktopRelease(context.Background())
	if err != nil {
		t.Fatalf("fetchLatestDesktopRelease() error = %v", err)
	}
	if got == nil || got.TagName != "desktop-v0.2.0" {
		t.Fatalf("fetchLatestDesktopRelease() = %+v, want desktop-v0.2.0（最新 desktop 发布，跳过 v0.3.0 CLI 发布）", got)
	}
}

// TestFetchLatestDesktopRelease_noDesktopReleases：无 desktop-v* 发布时返回 nil。
func TestFetchLatestDesktopRelease_noDesktopReleases(t *testing.T) {
	withMockGHAPI(t, mustEncodeReleases(t, []ghRelease{
		{TagName: "v0.3.0"},
		{TagName: "v0.2.0"},
	}))

	got, err := fetchLatestDesktopRelease(context.Background())
	if err != nil {
		t.Fatalf("fetchLatestDesktopRelease() error = %v", err)
	}
	if got != nil {
		t.Fatalf("fetchLatestDesktopRelease() = %+v, want nil", got)
	}
}

// TestFetchLatestDesktopRelease_skipsPrerelease：desktop-v* 预发布不得推送给用户
// （会被误当正式版安装），必须跳过、取下一个正式发布。
func TestFetchLatestDesktopRelease_skipsPrerelease(t *testing.T) {
	withMockGHAPI(t, mustEncodeReleases(t, []ghRelease{
		{TagName: "desktop-v0.2.0", HTMLURL: "https://github.com/xujian519/mady/releases/tag/desktop-v0.2.0"},
		{TagName: "desktop-v0.3.0-beta", HTMLURL: "https://github.com/xujian519/mady/releases/tag/desktop-v0.3.0-beta", Prerelease: true},
		{TagName: "desktop-v0.1.0", HTMLURL: "https://github.com/xujian519/mady/releases/tag/desktop-v0.1.0"},
	}))

	got, err := fetchLatestDesktopRelease(context.Background())
	if err != nil {
		t.Fatalf("fetchLatestDesktopRelease() error = %v", err)
	}
	if got == nil || got.TagName != "desktop-v0.2.0" {
		t.Fatalf("fetchLatestDesktopRelease() = %+v, want desktop-v0.2.0（跳过 v0.3.0-beta 预发布）", got)
	}
}

// TestUpdateDownloadURL_fallbackToReleasePage：无 zip asset 时回退 Release 页。
func TestUpdateDownloadURL_fallbackToReleasePage(t *testing.T) {
	r := &ghRelease{
		HTMLURL: "https://github.com/xujian519/mady/releases/tag/desktop-v0.2.0",
		Assets: []ghAsset{{
			BrowserDownloadURL: "https://github.com/xujian519/mady/releases/download/desktop-v0.2.0/Mady.exe",
		}},
	}
	if got := updateDownloadURL(r); got != r.HTMLURL {
		t.Fatalf("updateDownloadURL() = %q, want fallback %q（无 zip asset）", got, r.HTMLURL)
	}
}

// TestFetchLatestDesktopRelease_httpError：非 200 响应返回错误。
func TestFetchLatestDesktopRelease_httpError(t *testing.T) {
	withMockGHAPI(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "rate limited", http.StatusForbidden)
	})

	if _, err := fetchLatestDesktopRelease(context.Background()); err == nil {
		t.Fatal("fetchLatestDesktopRelease() error = nil, want non-nil for 403")
	}
}

// TestCheckUpdate_newerVersion：发现新版本 → HasUpdate + 下载地址（zip asset 优先）。
func TestCheckUpdate_newerVersion(t *testing.T) {
	oldVersion := desktopVersion
	desktopVersion = "0.1.0"
	t.Cleanup(func() { desktopVersion = oldVersion })

	withMockGHAPI(t, mustEncodeReleases(t, []ghRelease{{
		TagName: "desktop-v0.2.0",
		HTMLURL: "https://github.com/xujian519/mady/releases/tag/desktop-v0.2.0",
		Assets: []ghAsset{{
			BrowserDownloadURL: "https://github.com/xujian519/mady/releases/download/desktop-v0.2.0/Mady-0.2.0-macos-universal.zip",
		}},
	}}))

	app := &App{server: &madyserver.Server{}}
	info, err := app.CheckUpdate()
	if err != nil {
		t.Fatalf("CheckUpdate() error = %v", err)
	}
	if !info.HasUpdate {
		t.Fatalf("CheckUpdate() = %+v, want HasUpdate=true", info)
	}
	if info.LatestVersion != "0.2.0" {
		t.Errorf("LatestVersion = %q, want 0.2.0", info.LatestVersion)
	}
	if !strings.HasSuffix(info.DownloadURL, ".zip") {
		t.Errorf("DownloadURL = %q, want zip asset URL", info.DownloadURL)
	}
}

// TestCheckUpdate_upToDate：远端与当前同版本 → 无更新。
func TestCheckUpdate_upToDate(t *testing.T) {
	oldVersion := desktopVersion
	desktopVersion = "0.2.0"
	t.Cleanup(func() { desktopVersion = oldVersion })

	withMockGHAPI(t, mustEncodeReleases(t, []ghRelease{{
		TagName: "desktop-v0.2.0",
		HTMLURL: "https://github.com/xujian519/mady/releases/tag/desktop-v0.2.0",
	}}))

	app := &App{server: &madyserver.Server{}}
	info, err := app.CheckUpdate()
	if err != nil {
		t.Fatalf("CheckUpdate() error = %v", err)
	}
	if info.HasUpdate {
		t.Fatalf("CheckUpdate() = %+v, want HasUpdate=false（同版本）", info)
	}
	if !strings.Contains(info.Message, "已是最新版本") {
		t.Errorf("Message = %q, want 已是最新版本", info.Message)
	}
}

// TestCheckUpdate_noDesktopRelease：无 desktop 发布时返回无更新 + 说明文案。
func TestCheckUpdate_noDesktopRelease(t *testing.T) {
	withMockGHAPI(t, mustEncodeReleases(t, []ghRelease{{TagName: "v0.5.0"}}))

	app := &App{server: &madyserver.Server{}}
	info, err := app.CheckUpdate()
	if err != nil {
		t.Fatalf("CheckUpdate() error = %v", err)
	}
	if info.HasUpdate {
		t.Fatalf("CheckUpdate() = %+v, want HasUpdate=false（无 desktop 发布）", info)
	}
	if !strings.Contains(info.Message, "暂无可用更新") {
		t.Errorf("Message = %q, want 暂无可用更新", info.Message)
	}
}

// TestCheckUpdate_networkError：网络失败返回错误（不误报「已是最新」）。
func TestCheckUpdate_networkError(t *testing.T) {
	withMockGHAPI(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})

	app := &App{server: &madyserver.Server{}}
	if _, err := app.CheckUpdate(); err == nil {
		t.Fatal("CheckUpdate() error = nil, want non-nil on upstream failure")
	}
}

// TestCheckUpdate_notReady：App 未就绪时快速失败。
func TestCheckUpdate_notReady(t *testing.T) {
	app := &App{}
	if _, err := app.CheckUpdate(); err == nil {
		t.Fatal("CheckUpdate() error = nil, want errServerNotReady")
	} else if !errors.Is(err, errServerNotReady) {
		t.Fatalf("CheckUpdate() error = %v, want errServerNotReady", err)
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"0.1.0", "0.1.0", 0},
		{"0.1.0", "0.2.0", -1},
		{"0.2.0", "0.1.9", 1},
		{"1.0.0", "0.9.9", 1},
		{"v0.1.0", "0.1.0", 0},
		{"desktop-v0.2.0", "0.1.0", 1},
		{"0.1", "0.1.0", 0},
		{"0.1.0-beta", "0.1.0", 0}, // 预发布后缀忽略
		{"0.1.0", "abc", 0},        // 无法解析 → 保守相等
		{"0.1.0", "", 0},
	}
	for _, c := range cases {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestParseVersion(t *testing.T) {
	valid := map[string][3]int{
		"0.1.0":        {0, 1, 0},
		"v0.2.0":       {0, 2, 0},
		"desktop-v0.3": {0, 3, 0},
		"1.2.3-beta":   {1, 2, 3},
	}
	for in, want := range valid {
		got, ok := parseVersion(in)
		if !ok || got != want {
			t.Errorf("parseVersion(%q) = %v, %v; want %v, true", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "abc", "1", "1.2.3.4", "1..2", "-1.0.0", "1.2.x"} {
		if _, ok := parseVersion(in); ok {
			t.Errorf("parseVersion(%q) = ok=true, want false", in)
		}
	}
}
