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

// LoadDefaultStyles loads built-in DocumentStyle YAML files from the
// standard project locations. Results are cached after the first call.
func LoadDefaultStyles() ([]DocumentStyle, error) {
	cachedStylesOnce.Do(func() {
		for _, dir := range defaultStylePaths {
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
					cachedStylesErr = fmt.Errorf("style_embed: %s: %w (skipped)", path, err)
					continue
				}
				cachedStyles = append(cachedStyles, *style)
			}
			if len(cachedStyles) > 0 {
				break // loaded from first available directory
			}
		}
		if len(cachedStyles) == 0 {
			cachedStylesErr = fmt.Errorf("style_embed: no valid style files found in %v", defaultStylePaths)
		}
	})
	return cachedStyles, cachedStylesErr
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
