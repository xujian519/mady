package main

import (
	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/domains"
)

// withPlantaskAutoEnter 返回挂接自动进入规划态回调的选项
// （03-design §1.3：Gateway 分类 High → plantask.AutoEnterPlanning）。
// fc.PlantaskExt 不可用时返回 no-op 选项，保证各入口行为一致。
func withPlantaskAutoEnter(fc *frameworkContext) domains.UnifiedAgentOption {
	if fc == nil || fc.PlantaskExt == nil {
		return domains.WithGatewayModifier(func(*agentcore.Gateway) {})
	}
	return domains.WithGatewayModifier(func(g *agentcore.Gateway) {
		g.OnHighComplexity = fc.PlantaskExt.AutoEnterPlanning
	})
}
