package i18n

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

// Catalog 是线程安全的翻译目录。
// 键为消息标识符，值为 Locale → 翻译文本 的映射。
type Catalog struct {
	mu       sync.RWMutex
	messages map[string]map[Locale]string // key → locale → text
	locale   Locale
}

// New 创建空的翻译目录。
func New(locale Locale) *Catalog {
	return &Catalog{
		messages: make(map[string]map[Locale]string),
		locale:   locale,
	}
}

// SetLocale 设置当前语言。
func (c *Catalog) SetLocale(l Locale) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.locale = l
}

// Locale 返回当前语言。
func (c *Catalog) Locale() Locale {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.locale
}

// Add 添加单条翻译。
func (c *Catalog) Add(key string, locale Locale, text string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.messages[key] == nil {
		c.messages[key] = make(map[Locale]string)
	}
	c.messages[key][locale] = text
}

// T 返回当前语言的翻译文本。支持 %s/%d 格式化参数。
// 如果 key 不存在，返回 key 本身（fallback）。
// 如果 key 存在但当前语言无翻译，回退到 zh-CN。
func (c *Catalog) T(key string, args ...any) string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	localeMap, ok := c.messages[key]
	if !ok {
		return key
	}

	text, ok := localeMap[c.locale]
	if !ok {
		text, ok = localeMap[LocaleZhCN]
		if !ok {
			return key
		}
	}

	if len(args) > 0 {
		return fmt.Sprintf(text, args...)
	}
	return text
}

// LoadYAML 从 YAML 文件加载翻译。
// 支持的格式：
//
//	key.locale: text
//
// 或按 locale 分组：
//
//	greeting:
//	  zh-CN: "你好"
//	  en-US: "Hello"
func (c *Catalog) LoadYAML(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("i18n: 读取翻译文件失败 %s: %w", path, err)
	}

	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("i18n: 解析翻译文件失败 %s: %w", path, err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for key, val := range raw {
		c.loadValue(key, val)
	}

	return nil
}

// loadValue 递归处理 YAML 节点，将翻译导入 catalog。
func (c *Catalog) loadValue(prefix string, val any) {
	switch v := val.(type) {
	case string:
		if c.messages[prefix] == nil {
			c.messages[prefix] = make(map[Locale]string)
		}
		c.messages[prefix][DefaultLocale] = v
	case map[string]any:
		hasLocale := false
		for k := range v {
			if _, ok := supportedLocales[k]; ok {
				hasLocale = true
				break
			}
		}
		if hasLocale {
			if c.messages[prefix] == nil {
				c.messages[prefix] = make(map[Locale]string)
			}
			for localeStr, textVal := range v {
				if text, ok := textVal.(string); ok {
					locale := ParseLocale(localeStr)
					c.messages[prefix][locale] = text
				}
			}
		} else {
			for subKey, subVal := range v {
				c.loadValue(prefix+"."+subKey, subVal)
			}
		}
	}
}

// supportedLocales 用于 YAML 解析时判断是否为 locale 键。
var supportedLocales = map[string]bool{
	"zh-CN": true,
	"zh_CN": true,
	"zh":    true,
	"en-US": true,
	"en_US": true,
	"en":    true,
}

// LoadDir 从目录加载所有 .yaml 翻译文件。
func (c *Catalog) LoadDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("i18n: 读取翻译目录失败 %s: %w", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if ext := filepath.Ext(entry.Name()); ext != ".yaml" && ext != ".yml" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if err := c.LoadYAML(path); err != nil {
			return err
		}
	}

	return nil
}

// Global 返回全局默认翻译目录。
func Global() *Catalog {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return global
}

// SetGlobal 设置全局翻译目录。
func SetGlobal(c *Catalog) {
	globalMu.Lock()
	defer globalMu.Unlock()
	global = c
}

// T 快捷方式：Global().T(key, args...)
func T(key string, args ...any) string {
	return Global().T(key, args...)
}

var (
	global   *Catalog
	globalMu sync.RWMutex
)

func init() {
	global = New(DefaultLocale)
}
