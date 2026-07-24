package domains

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var (
	cachedStyles     []DocumentStyle
	cachedStylesOnce sync.Once
	cachedStylesErr  error
)

// defaultStylePaths lists directories where built-in style YAML files are
// located. These are loaded once and cached. The first directory that
// yields at least one valid style file wins; remaining directories are
// skipped.
var defaultStylePaths = []string{
	"styles", // project root (development)
}

// AddStylePath appends an additional search path for style files.
// Must be called before the first LoadDefaultStyles() invocation.
func AddStylePath(path string) {
	defaultStylePaths = append(defaultStylePaths, path)
}

// LoadDefaultStyles loads DocumentStyle YAML files from all registered
// search paths and merges them. Results are cached after the first call.
//
// defaultStylePaths 的顺序为内置目录在前、用户目录（经 AddStylePath 追加，
// 例如 $MADY_HOME/styles）在后。同名风格后者覆盖前者，因此用户自定义风格
// 优先于内置；不同域的风格各自独立保留。
func LoadDefaultStyles() ([]DocumentStyle, error) {
	cachedStylesOnce.Do(func() {
		cachedStyles, cachedStylesErr = loadStylesFromPaths(defaultStylePaths)
	})
	return cachedStyles, cachedStylesErr
}

// loadStylesFromPaths 从给定目录列表加载并合并风格：同名后者覆盖前者，
// 不同 name 各自保留。不存在的目录被静默跳过。
//
// 解析错误的文件被跳过（不阻断其余有效风格的加载），仅当没有任何有效风格
// 时才返回错误。
func loadStylesFromPaths(paths []string) ([]DocumentStyle, error) {
	var styles []DocumentStyle
	byName := make(map[string]int) // name → styles 索引，用于同名覆盖
	for _, dir := range paths {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue // directory may not exist in all environments
		}
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			style, err := LoadStyle(path)
			if err != nil {
				// Skip malformed files; don't abort all styles.
				continue
			}
			if idx, ok := byName[style.Name]; ok {
				styles[idx] = *style // 同名：后加载（用户）覆盖先加载（内置）
			} else {
				byName[style.Name] = len(styles)
				styles = append(styles, *style)
			}
		}
	}
	if len(styles) == 0 {
		return nil, fmt.Errorf("style_embed: no valid style files found in %v", paths)
	}
	return styles, nil
}

// bestStyleForDomain picks the best-matching style for a domain.
func bestStyleForDomain(styles []DocumentStyle, domain string) *DocumentStyle {
	domainStyles := StylesForDomain(styles, domain)
	if len(domainStyles) > 0 {
		return &domainStyles[0]
	}
	return nil
}

// styleInjection returns a system-prompt block to inject a domain style.
func styleInjection(domain string) string {
	styles, err := LoadDefaultStyles()
	if err != nil {
		return ""
	}
	s := bestStyleForDomain(styles, domain)
	if s == nil {
		return ""
	}
	return "\n\n" + strings.TrimSpace(s.SystemPrompt())
}
