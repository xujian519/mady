// Package knowledgeinit initializes patent knowledge base data at startup.
package knowledgeinit

import (
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/xujian519/mady/knowledge"
	"github.com/xujian519/mady/pkg/util"
)

// InitPatentKnowledge loads built-in patent knowledge files into the
// knowledge Store. Scans data/knowledge/ for Markdown files and indexes
// them by domain for use by patent agents.
//
// Expected directory layout:
//
//	$MADY_HOME/data/knowledge/
//	  patent-law-full.md       — 专利法全文
//	  guidelines.md            — 审查指南全文
//	  ipc-classes.md           — IPC 分类表
//	  invalidation-top100.md   — 精选无效决定案例
//
// Files that do not exist are silently skipped so the function is safe to
// call even when the user has not provided their own knowledge data.
func InitPatentKnowledge(store *knowledge.Store) error {
	base, err := util.ResolveDataDir("knowledge")
	if err != nil {
		return fmt.Errorf("resolve knowledge dir: %w", err)
	}

	documents := []struct {
		domain string
		docID  string
		file   string
	}{
		{domain: "patent-law", docID: "patent-law-full", file: "patent-law-full.md"},
		{domain: "guidelines", docID: "guidelines", file: "guidelines.md"},
		{domain: "ipc", docID: "ipc-classes", file: "ipc-classes.md"},
		{domain: "invalidation", docID: "invalidation-top100", file: "invalidation-top100.md"},
	}

	for _, d := range documents {
		path := filepath.Join(base, d.file)
		if err := store.LoadDocument(d.domain, d.docID, path); err != nil {
			// File not found is non-fatal — user provides their own data.
			slog.Info("knowledgeinit: 跳过", "doc", d.docID, "path", path)
			continue
		}
		slog.Info("knowledgeinit: 已加载", "doc", d.docID, "domain", d.domain)
	}

	return nil
}
