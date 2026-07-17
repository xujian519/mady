package mcp

// C7 安全修复：从 $PWD 发现的 .mcp.json 在执行其 stdio command 前必须通过
// 信任校验，防止不可信目录（如克隆的恶意仓库）借配置文件静默执行命令。
//
// 信任模型：$MADY_HOME/trusted-mcp.json 记录「配置文件绝对路径 → 内容
// SHA-256」。文件内容变化后哈希失配，需重新信任。可通过
// `mady trust-mcp [path]` 写入信任记录；设置 MADY_MCP_TRUST_CWD=1 可跳过
// 校验（开发逃生门，不推荐长期使用）。

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// trustStoreFile 是 trusted-mcp.json 的磁盘格式。
type trustStoreFile struct {
	// Configs 记录已信任的 MCP 配置文件：绝对路径 → 内容 SHA-256（hex）。
	Configs map[string]string `json:"configs"`
}

// trustStorePath 返回信任存储文件路径。
func trustStorePath(madyHome string) string {
	return filepath.Join(madyHome, "trusted-mcp.json")
}

// fileSHA256 计算文件内容的 SHA-256（hex 编码）。
func fileSHA256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// loadTrustStore 读取信任存储；文件不存在时返回空存储。
func loadTrustStore(madyHome string) (*trustStoreFile, error) {
	store := &trustStoreFile{Configs: map[string]string{}}
	data, err := os.ReadFile(trustStorePath(madyHome))
	if err != nil {
		if os.IsNotExist(err) {
			return store, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, store); err != nil {
		return nil, fmt.Errorf("mcp: parse trust store: %w", err)
	}
	if store.Configs == nil {
		store.Configs = map[string]string{}
	}
	return store, nil
}

// TrustMCPConfigFile 将指定配置文件的当前内容哈希写入信任存储，
// 之后 DiscoverMCPExtensions 将允许执行其中的 stdio command。
// path 会转为绝对路径记录；madyHome 为空时使用默认解析（$MADY_HOME > ~/.mady）。
func TrustMCPConfigFile(path, madyHome string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("mcp: resolve %s: %w", path, err)
	}
	hash, err := fileSHA256(abs)
	if err != nil {
		return fmt.Errorf("mcp: hash %s: %w", abs, err)
	}
	if madyHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("mcp: resolve home dir: %w", err)
		}
		madyHome = os.Getenv("MADY_HOME")
		if madyHome == "" {
			madyHome = filepath.Join(home, ".mady")
		}
	}
	store, err := loadTrustStore(madyHome)
	if err != nil {
		return err
	}
	store.Configs[abs] = hash
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return fmt.Errorf("mcp: marshal trust store: %w", err)
	}
	if err := os.MkdirAll(madyHome, 0o755); err != nil {
		return fmt.Errorf("mcp: create mady home: %w", err)
	}
	// 信任存储含本地路径信息，仅所有者可读写。
	if err := os.WriteFile(trustStorePath(madyHome), data, 0o600); err != nil {
		return fmt.Errorf("mcp: write trust store: %w", err)
	}
	return nil
}

// isConfigTrusted 校验配置文件当前内容哈希是否与信任记录一致。
// 信任记录缺失、哈希失配或存储损坏时返回 false（fail-closed）。
func isConfigTrusted(path, madyHome string) bool {
	if madyHome == "" {
		return false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	store, err := loadTrustStore(madyHome)
	if err != nil {
		return false
	}
	want, ok := store.Configs[abs]
	if !ok {
		return false
	}
	got, err := fileSHA256(abs)
	if err != nil {
		return false
	}
	return got == want
}

// cwdTrustBypassed 报告是否通过环境变量跳过 cwd 配置信任校验。
// MADY_MCP_TRUST_CWD=1 是开发逃生门：信任任意 $PWD/.mcp.json。
func cwdTrustBypassed() bool {
	return os.Getenv("MADY_MCP_TRUST_CWD") == "1"
}
