package bootstrap

import (
	"github.com/xujian519/mady/disclosure"
	"github.com/xujian519/mady/domains"
)

// init 在 bootstrap 阶段注入 disclosure 工具构造函数到 domains 根包，
// 使 PatentAgentConfig 无需直接导入 disclosure 包即可创建 analyze_disclosure 工具。
func init() {
	domains.SetDisclosureToolFactory(disclosure.NewDisclosureTool)
}
