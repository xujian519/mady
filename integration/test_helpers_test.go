//go:build integration

package integration_test

import (
	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/tools"
)

// testToolExt 为集成测试创建一个最小工具扩展。
func testToolExt() agentcore.Extension {
	return tools.NewExtension(tools.ExtensionConfig{
		WorkingDir: "/tmp/mady-integration-test",
	})
}

// testToolExtTuple 返回三个工具扩展，供 UnifiedAgentConfig 集成测试使用。
func testToolExtTuple() (agentcore.Extension, agentcore.Extension, agentcore.Extension) {
	base := testToolExt()
	return base, base, base
}
