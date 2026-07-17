package mcp

// C7 安全修复回归测试：$PWD/.mcp.json 信任机制。
// 未通过信任校验的项目配置不得执行其中的 stdio command。

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupCWDDiscovery 构造 cwd 配置发现场景：临时目录作为 cwd，写入
// $PWD/.mcp.json（stdio server 指向不存在的二进制，避免真的执行），
// 并隔离 MCP_CONFIG / HOME 环境。返回配置路径与 madyHome。
func setupCWDDiscovery(t *testing.T) (cfgPath, madyHome string) {
	t.Helper()
	cwd := t.TempDir()
	madyHome = t.TempDir()
	cfgPath = filepath.Join(cwd, ".mcp.json")
	writeJSON(t, cfgPath, MCPConfigFile{
		MCPServers: map[string]MCPServerConfig{
			"evil": {Command: "/nonexistent/mady-mcp-test-binary"},
		},
	})
	t.Chdir(cwd)
	t.Setenv("MCP_CONFIG", "/nonexistent/mcp.json")
	t.Setenv("HOME", t.TempDir())
	return cfgPath, madyHome
}

func hasWarningContaining(warnings []error, substr string) bool {
	for _, w := range warnings {
		if strings.Contains(w.Error(), substr) {
			return true
		}
	}
	return false
}

func TestDiscoverMCPExtensions_UntrustedCWDConfigSkipped(t *testing.T) {
	_, madyHome := setupCWDDiscovery(t)

	exts, warnings := DiscoverMCPExtensions(context.Background(), madyHome)
	if len(exts) != 0 {
		t.Fatalf("expected 0 extensions for untrusted config, got %d", len(exts))
	}
	if !hasWarningContaining(warnings, "untrusted project config") {
		t.Fatalf("expected untrusted-config warning, got %v", warnings)
	}
}

func TestDiscoverMCPExtensions_TrustedCWDConfigPassesTrustGate(t *testing.T) {
	cfgPath, madyHome := setupCWDDiscovery(t)

	if err := TrustMCPConfigFile(cfgPath, madyHome); err != nil {
		t.Fatalf("TrustMCPConfigFile failed: %v", err)
	}
	_, warnings := DiscoverMCPExtensions(context.Background(), madyHome)
	// 信任后不再被信任门禁拦截（二进制不存在导致的创建失败属于预期，
	// 说明流程已经越过信任校验进入真正的扩展创建阶段）。
	if hasWarningContaining(warnings, "untrusted project config") {
		t.Fatalf("trusted config must not hit trust gate, got %v", warnings)
	}
	if !hasWarningContaining(warnings, "mady-mcp-test-binary") &&
		!hasWarningContaining(warnings, `server "evil"`) {
		t.Fatalf("expected extension-creation warning past the trust gate, got %v", warnings)
	}
}

func TestDiscoverMCPExtensions_CWDTrustBypassEnv(t *testing.T) {
	_, madyHome := setupCWDDiscovery(t)
	t.Setenv("MADY_MCP_TRUST_CWD", "1")

	_, warnings := DiscoverMCPExtensions(context.Background(), madyHome)
	if hasWarningContaining(warnings, "untrusted project config") {
		t.Fatalf("MADY_MCP_TRUST_CWD=1 must bypass trust gate, got %v", warnings)
	}
}

func TestTrustMCPConfigFile_HashInvalidation(t *testing.T) {
	dir := t.TempDir()
	madyHome := t.TempDir()
	cfgPath := filepath.Join(dir, ".mcp.json")
	writeJSON(t, cfgPath, MCPConfigFile{
		MCPServers: map[string]MCPServerConfig{"a": {Command: "x"}},
	})

	// 信任前不可信；信任后可信。
	if isConfigTrusted(cfgPath, madyHome) {
		t.Fatal("config must not be trusted before TrustMCPConfigFile")
	}
	if err := TrustMCPConfigFile(cfgPath, madyHome); err != nil {
		t.Fatalf("TrustMCPConfigFile failed: %v", err)
	}
	if !isConfigTrusted(cfgPath, madyHome) {
		t.Fatal("config must be trusted after TrustMCPConfigFile")
	}

	// 内容变化后哈希失配，重新 fail-closed。
	writeJSON(t, cfgPath, MCPConfigFile{
		MCPServers: map[string]MCPServerConfig{"a": {Command: "y"}},
	})
	if isConfigTrusted(cfgPath, madyHome) {
		t.Fatal("modified config must fail trust check")
	}

	// 信任存储文件权限：仅所有者可读写。
	info, err := os.Stat(trustStorePath(madyHome))
	if err != nil {
		t.Fatalf("trust store missing: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("trust store perm = %o, want 600", perm)
	}
}
