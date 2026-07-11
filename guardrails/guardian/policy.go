package guardian

// PatentLegalPolicy is the default safety policy prompt for patent/legal agents.
const PatentLegalPolicy = `你是专利/法律 Agent 的安全审查员。
对每个待执行的工具调用进行评估，输出 JSON：
{"risk_level":"low|medium|high","outcome":"allow|deny","rationale":"简要理由"}

重点关注：
1. 是否涉及真实当事人信息（姓名、地址、证件号）的泄露
2. 是否执行了不可逆的法律操作（提交官方文件、发送正式函件）
3. 是否删除或覆盖已有案件文档
4. 是否在未经确认的情况下修改权利要求/说明书核心内容
5. 是否访问了与当前任务无关的案件文件

风险分级：
- 低风险：只读操作、格式调整、内部草稿编辑
- 中风险：新建文件、检索操作、草稿生成
- 高风险：删除文件、覆盖已有内容、外部发送、提交操作`

// ReviewLevel controls which tools the Guardian reviews.
type ReviewLevel int

const (
	// ReviewOff disables the Guardian entirely.
	ReviewOff ReviewLevel = iota
	// ReviewHighRisk only reviews high-risk tools (delete, bash).
	ReviewHighRisk
	// ReviewAllWriters reviews all non-read-only tools.
	ReviewAllWriters
)

// HighRiskTools is the set of tools always considered high-risk.
var HighRiskTools = map[string]bool{
	"delete": true,
	"bash":   true,
	"move":   true,
}
