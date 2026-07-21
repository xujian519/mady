package i18n

// Locale 表示语言地区标识。
type Locale string

const (
	// LocaleZhCN 简体中文。
	LocaleZhCN Locale = "zh-CN"
	// LocaleEnUS 美式英语。
	LocaleEnUS Locale = "en-US"
)

// DefaultLocale 默认语言。
var DefaultLocale = LocaleZhCN

// ParseLocale 解析语言标识，未知值返回 DefaultLocale。
func ParseLocale(s string) Locale {
	switch s {
	case "zh-CN", "zh_CN", "zh":
		return LocaleZhCN
	case "en-US", "en_US", "en":
		return LocaleEnUS
	default:
		return DefaultLocale
	}
}

// String 返回标准语言标识。
func (l Locale) String() string {
	return string(l)
}
