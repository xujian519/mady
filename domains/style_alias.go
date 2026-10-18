package domains

import "github.com/xujian519/mady/domains/config"

// 写作风格（DocumentStyle）的实现位于 domains/config（config/style.go 与
// config/style_embed.go）。本文件为根包保留转发，维持既有调用方不变：
//   - AddStylePath：bootstrap/setup.go 与 desktop/app.go 注册 $MADY_HOME/styles
//   - styleInjection：legal.go 与 patent.go 注入 patent/legal 域的 system prompt，
//     两者属敏感路径，不在此处改动其调用形式
//
// DocumentStyle 本身及其加载/筛选/渲染 API 除上述注入路径外没有其他调用方，
// 因此不在根包重复暴露别名——需要时直接 import domains/config。

// AddStylePath 追加一个风格文件搜索目录（见 config.AddStylePath）。
// 必须在首次调用 config.LoadDefaultStyles 之前注册。
func AddStylePath(path string) { config.AddStylePath(path) }

// styleInjection 返回注入指定域风格的 system prompt 片段
// （见 config.StyleInjection）；无可用风格时返回空串。
func styleInjection(domain string) string { return config.StyleInjection(domain) }
