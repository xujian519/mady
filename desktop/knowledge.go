//go:build darwin

package main

// knowledge.go — 知识库管理后端绑定。
//
// 提供知识库状态查询 API，读取 knowledge/ 目录下的索引文件元数据
// 和源文档数量，供前端 KnowledgeView 渲染。

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/xujian519/mady/pkg/util"
)

// KnowledgeStatus 返回知识库的运行状态概览。
type KnowledgeStatus struct {
	DocCount    int      `json:"docCount"`
	IndexSizeMB int      `json:"indexSizeMB"`
	LastUpdated string   `json:"lastUpdated"`
	SourceDirs  []string `json:"sourceDirs"`
	IsIndexing  bool     `json:"isIndexing"`
}

// GetKnowledgeStatus 返回知识库状态概览。
// 读取 knowledge/ 目录下的 SQLite 索引文件元数据作为状态展示，
// 同时统计 laws/wiki 源目录中的文档文件数量。
func (a *App) GetKnowledgeStatus() KnowledgeStatus {
	madyHome, err := util.MadyHome()
	if err != nil || madyHome == "" {
		// 回退到应用启动时已设置的 fc.MadyHome
		if a.fc != nil && a.fc.MadyHome != "" {
			madyHome = a.fc.MadyHome
		} else {
			return KnowledgeStatus{}
		}
	}

	kbDir := filepath.Join(madyHome, "knowledge")
	sourceDirs := []string{
		filepath.Join(kbDir, "laws"),
		filepath.Join(kbDir, "wiki"),
	}

	// 检查是否存在 knowledge SQLite 索引文件
	indexPath := filepath.Join(kbDir, "knowledge.db")
	var docCount int
	var indexSizeMB int
	var lastUpdated string

	if fi, err := os.Stat(indexPath); err == nil {
		indexSizeMB = int(fi.Size() / (1024 * 1024))
		if indexSizeMB < 1 && fi.Size() > 0 {
			indexSizeMB = 1
		}
		lastUpdated = fi.ModTime().Format("2006-01-02 15:04")
	}

	// 统计 laws/wiki 目录中的文档文件数（过滤掉目录和隐藏文件）
	for _, dir := range sourceDirs {
		docCount += countDocFiles(dir)
	}

	return KnowledgeStatus{
		DocCount:    docCount,
		IndexSizeMB: indexSizeMB,
		LastUpdated: lastUpdated,
		SourceDirs:  sourceDirs,
		IsIndexing:  false,
	}
}

// countDocFiles 统计目录中的文档文件（.md / .pdf / .txt，排除隐藏文件和子目录）。
func countDocFiles(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if strings.HasSuffix(name, ".md") || strings.HasSuffix(name, ".pdf") || strings.HasSuffix(name, ".txt") {
			n++
		}
	}
	return n
}
