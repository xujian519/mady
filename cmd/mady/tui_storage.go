package main

// tui_storage.go 提供 TUI 启动时的统一存储预检（写探针 + 错误分类 + UI 提示）。
//
// StorageProbeResult 收集路径可写性检测结果，供 tui.go 在启动时向用户展示
// 降级提示（而非仅 log.Printf 静默降级）。

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/xujian519/mady/pkg/util"
)

// StorageProbeResult 描述一个存储路径的写探针结果。
type StorageProbeResult struct {
	// Name 是存储项的名称，如 "sessions" / "settings" / "approvals"。
	Name string
	// Path 是探测的完整路径。
	Path string
	// ResolvedDir 是解析后的目录路径（仅 Name=="sessions" 时有意义）。
	ResolvedDir string
	// Unavailable 为 true 表示该存储不可用（需降级）。
	Unavailable bool
	// Message 是技术性错误消息（日志用）。
	Message string
	// UserMessage 是面向用户的降级提示文案（UI 用）。
	UserMessage string
}

// probeSessionDir 检测 session 持久化目录的可写性。
// 返回探测结果，其中 ResolvedDir 为最终解析的目录路径（即使不可写也返回）。
func probeSessionDir(envDir, madyHome, workspaceDir string) StorageProbeResult {
	r := StorageProbeResult{Name: "sessions"}

	sessionDir := envDir
	if sessionDir == "" {
		if madyHome != "" {
			sessionDir = filepath.Join(madyHome, "sessions")
		} else {
			dir, err := util.ResolveDataDir("sessions")
			if err != nil {
				r.Unavailable = true
				r.Message = fmt.Sprintf("resolve sessions dir: %v", err)
				r.UserMessage = "会话持久化未启用，当前为仅内存模式"
				return r
			}
			sessionDir = dir
		}
	}
	r.ResolvedDir = sessionDir
	r.Path = sessionDir

	// 尝试创建目录（不存在时创建）并写入测试文件。
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		r.Unavailable = true
		r.Message = fmt.Sprintf("mkdir %s: %v", sessionDir, err)
		r.UserMessage = "会话持久化未启用，当前为仅内存模式（无法创建会话目录）"
		return r
	}

	// 写探针：尝试创建临时文件。
	testFile := filepath.Join(sessionDir, ".mady-write-test")
	if err := os.WriteFile(testFile, []byte{}, 0o600); err != nil {
		r.Unavailable = true
		r.Message = fmt.Sprintf("write test to %s: %v", sessionDir, err)
		r.UserMessage = "会话持久化未启用，当前为仅内存模式（会话目录不可写）"
		_ = os.Remove(testFile)
		return r
	}
	_ = os.Remove(testFile)

	r.Unavailable = false
	r.Message = ""
	return r
}

// probeSettingsStore 检测 settings.json 的可读写性。
func probeSettingsStore(homeDir string) StorageProbeResult {
	r := StorageProbeResult{Name: "settings"}

	settingsDir := filepath.Join(homeDir, ".mady")
	settingsPath := filepath.Join(settingsDir, "settings.json")
	r.Path = settingsPath

	// 目录不一定存在，先尝试创建。
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		r.Unavailable = true
		r.Message = fmt.Sprintf("mkdir settings dir: %v", err)
		r.UserMessage = "设置持久化未启用，部分设置将在重启后丢失"
		return r
	}

	// 如果文件已存在，检查可读写性。
	if _, err := os.Stat(settingsPath); err == nil {
		// 尝试打开以确认权限。
		f, err := os.OpenFile(settingsPath, os.O_RDWR, 0o600)
		if err != nil {
			r.Unavailable = true
			r.Message = fmt.Sprintf("open settings: %v", err)
			r.UserMessage = "设置文件不可写，部分设置将在重启后丢失"
			return r
		}
		_ = f.Close()
	} else if !os.IsNotExist(err) {
		r.Unavailable = true
		r.Message = fmt.Sprintf("stat settings: %v", err)
		r.UserMessage = "设置持久化未启用，部分设置将在重启后丢失"
		return r
	}
	// 文件不存在：目录已创建，NewSettingsStore 会新建它。

	r.Unavailable = false
	return r
}

// probeApprovalStore 检测 approvals.db 的可写性。
func probeApprovalStore(workspaceDir, madyHome string) StorageProbeResult {
	r := StorageProbeResult{Name: "approvals"}

	baseDir := workspaceDir
	if baseDir == "" {
		if madyHome != "" {
			baseDir = filepath.Join(madyHome, "workspace")
		} else {
			baseDir = filepath.Join(os.TempDir(), "mady")
		}
	}
	dbPath := filepath.Join(baseDir, "approvals.db")
	r.Path = dbPath

	// 确保父目录存在。
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		r.Unavailable = true
		r.Message = fmt.Sprintf("mkdir approval dir: %v", err)
		r.UserMessage = "审批留痕未落盘（无法创建审批数据库目录）"
		return r
	}

	// 写探针：尝试打开一个测试数据库（不同于真正的 approvals.db）。
	// 这里只做目录级别的写检测，真正的 SQLite open 在 runTui 的
	// openApprovalStore() 中进行。
	testFile := filepath.Join(baseDir, ".mady-approval-test")
	if err := os.WriteFile(testFile, []byte{}, 0o600); err != nil {
		r.Unavailable = true
		r.Message = fmt.Sprintf("write test to %s: %v", baseDir, err)
		r.UserMessage = "审批留痕未落盘（审批数据目录不可写）"
		_ = os.Remove(testFile)
		return r
	}
	_ = os.Remove(testFile)

	r.Unavailable = false
	return r
}

// storageDegradationTag 从存储探针结果列表中提取降级标签。
// 无降级时返回空字符串。
func storageDegradationTag(probes []StorageProbeResult) string {
	var tags []string
	for _, p := range probes {
		if p.Unavailable {
			switch p.Name {
			case "sessions":
				tags = append(tags, "mem-session")
			case "approvals":
				tags = append(tags, "mem-approval")
			case "settings":
				tags = append(tags, "mem-settings")
			}
		}
	}
	if len(tags) == 0 {
		return ""
	}
	return "⚠ " + strings.Join(tags, ",")
}

// storageDegradationMessages 从探针结果中提取所有面向用户的降级提示。
func storageDegradationMessages(probes []StorageProbeResult) []string {
	var msgs []string
	for _, p := range probes {
		if p.Unavailable && p.UserMessage != "" {
			msgs = append(msgs, p.UserMessage)
		}
	}
	return msgs
}
