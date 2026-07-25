package evidence

import "github.com/xujian519/mady/agentcore"

func newTypeSpecificTool(engine *DefaultEngine) *agentcore.Tool {
	return &agentcore.Tool{Name: "judge_type_specific"}
}
