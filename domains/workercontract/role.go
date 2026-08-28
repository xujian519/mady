// role.go 实现专利团队的角色契约与立场配对（DSH patent-teams role-contracts 引入）。
//
// 核心是"立场配对 + 中立裁判"：对抗性场景（无效/诉讼/OA 答复/撰写对立评审）
// 由一对立场相反的角色 + 一个中立裁判组成，裁判禁止参与任一方的策略起草。
// 角色契约产出 persona 段落供子代理装配，越界禁止明确写进 persona——
// 立场约束靠显式声明与确定性校验，不靠提示语自觉。

package workercontract

import (
	"fmt"
	"sort"
	"strings"
)

// Stance 是角色在对抗性分析中的立场（DSH role-contracts 七值）。
type Stance string

const (
	// StanceNeutral 中立立场（技术核验/技术调查）。
	StanceNeutral Stance = "neutral"
	// StanceAgentSide 申请人侧代理（撰写方/权利人代理视角）。
	StanceAgentSide Stance = "agent-side"
	// StanceExaminer 审查员立场（OA 意见重构）。
	StanceExaminer Stance = "examiner"
	// StanceApplicant 对抗中的权利人/专利权人立场。
	StanceApplicant Stance = "applicant"
	// StanceDefense 被告/答辩方立场。
	StanceDefense Stance = "defense"
	// StanceAttacker 攻击方立场（无效请求人/对立评审）。
	StanceAttacker Stance = "attacker"
	// StanceJudge 中立裁判（合议组/范围审视）。
	StanceJudge Stance = "judge"
)

// Valid 报告立场是否为已知值。
func (s Stance) Valid() bool {
	switch s {
	case StanceNeutral, StanceAgentSide, StanceExaminer, StanceApplicant,
		StanceDefense, StanceAttacker, StanceJudge:
		return true
	}
	return false
}

// adversarialSide 报告立场是否属于对抗性一方（非中立、非裁判）。
func (s Stance) adversarialSide() bool {
	return s == StanceAgentSide || s == StanceExaminer || s == StanceApplicant ||
		s == StanceDefense || s == StanceAttacker
}

// RoleContract 是单个团队角色的契约。
type RoleContract struct {
	ID     string `json:"id"`    // 角色标识，如 "opposing-reviewer"
	Title  string `json:"title"` // 中文名称，如 "对立评审员"
	Stance Stance `json:"stance"`

	// StrategyDrafting 表示该角色承担策略起草职责（撰写主张/答辩策略）。
	// 裁判立场必须为 false——由 Validate 强制，这是"裁判不参与任一方策略"
	// 的结构性保证。
	StrategyDrafting bool `json:"strategy_drafting,omitempty"`

	// HITL 表示该角色的产出必须经人工确认后才能进入交付物。
	HITL bool `json:"hitl,omitempty"`

	// Duties 是职责清单（进入 persona）。
	Duties []string `json:"duties,omitempty"`
	// Boundaries 是越界禁止清单（进入 persona，明确禁止跨立场）。
	Boundaries []string `json:"boundaries,omitempty"`
}

// Validate 校验角色契约：必填字段齐全、立场合法、裁判不得承担策略起草。
func (r RoleContract) Validate() error {
	if r.ID == "" || r.Title == "" {
		return fmt.Errorf("角色契约缺 id/title: %+v", r)
	}
	if !r.Stance.Valid() {
		return fmt.Errorf("角色 %s 非法立场 %q", r.ID, r.Stance)
	}
	if r.Stance == StanceJudge && r.StrategyDrafting {
		return fmt.Errorf("角色 %s：裁判立场禁止承担策略起草职责", r.ID)
	}
	return nil
}

// PersonaSegment 生成装配子代理时使用的 persona 段落：职责 + 越界禁止 +
// HITL 提示。裁判角色自动附带中立性声明。
func (r RoleContract) PersonaSegment() string {
	var b strings.Builder
	fmt.Fprintf(&b, "角色：%s（%s），立场 %s。\n", r.Title, r.ID, r.Stance)
	if r.Stance == StanceJudge {
		b.WriteString("你是中立裁判：只基于双方提交的材料独立判断，禁止参与或影响任何一方的策略起草。\n")
	}
	if len(r.Duties) > 0 {
		b.WriteString("职责：\n")
		for _, d := range r.Duties {
			fmt.Fprintf(&b, "- %s\n", d)
		}
	}
	if len(r.Boundaries) > 0 {
		b.WriteString("越界禁止：\n")
		for _, x := range r.Boundaries {
			fmt.Fprintf(&b, "- %s\n", x)
		}
	}
	if r.HITL {
		b.WriteString("注意：你的产出属于人工确认范围，未经人工复核不得进入最终交付物。\n")
	}
	return b.String()
}

// TeamComposition 是一个场景的角色编排（有序）。
type TeamComposition struct {
	Scenario string         `json:"scenario"` // 与专利工作流 manifest 的 case_type 对应
	Roles    []RoleContract `json:"roles"`
}

// Validate 校验立场配对：角色各自合法；裁判至多一个；出现裁判时必须同时
// 存在至少两个不同的对抗性立场（对立双方，如 攻击方×防御方、权利人×被告）。
func (t TeamComposition) Validate() error {
	judges := 0
	sides := make(map[Stance]bool)
	for _, r := range t.Roles {
		if err := r.Validate(); err != nil {
			return fmt.Errorf("团队 %s: %w", t.Scenario, err)
		}
		switch {
		case r.Stance == StanceJudge:
			judges++
		case r.Stance.adversarialSide():
			sides[r.Stance] = true
		}
	}
	if judges > 1 {
		return fmt.Errorf("团队 %s: 裁判角色至多一个，当前 %d 个", t.Scenario, judges)
	}
	if judges == 1 && len(sides) < 2 {
		return fmt.Errorf("团队 %s: 裁判必须搭配对立双方（至少两个不同的对抗性立场）", t.Scenario)
	}
	return nil
}

// 内置场景编排。Scenario 与 domains/workflows/patent 的 case_type 对应，
// 供 patent_team_resolve 工具按场景解析。
var teamCompositions = map[string]TeamComposition{
	"patent-drafting": {
		Scenario: "patent-drafting",
		Roles: []RoleContract{
			{ID: "drafter", Title: "撰写代理人", Stance: StanceAgentSide, StrategyDrafting: true,
				Duties:     []string{"基于交底书起草权利要求与说明书", "确定布局策略"},
				Boundaries: []string{"禁止因检索结果直接放弃撰写，放弃决策须上报"}},
			{ID: "opposing-reviewer", Title: "对立评审员", Stance: StanceAttacker,
				Duties:     []string{"以攻击视角评审草稿：检索漏洞、保护范围缺陷、支持性问题"},
				Boundaries: []string{"禁止代为修改草稿，只提出攻击意见"}},
			{ID: "technical-verifier", Title: "技术核验员", Stance: StanceNeutral,
				Duties: []string{"核验技术方案描述与附图一致性"}},
			{ID: "scope-reviewer", Title: "范围审视（裁判）", Stance: StanceJudge, HITL: true,
				Duties:     []string{"综合撰写与对立意见，审定最终保护范围建议"},
				Boundaries: []string{"禁止参与撰写策略或攻击策略的起草"}},
		},
	},
	"invalidation": {
		Scenario: "invalidation",
		Roles: []RoleContract{
			{ID: "requestor", Title: "无效请求人", Stance: StanceAttacker, StrategyDrafting: true,
				Duties:     []string{"构建无效理由地图与证据组合"},
				Boundaries: []string{"禁止同时为专利权人辩护"}},
			{ID: "patent-holder", Title: "专利权人", Stance: StanceApplicant, StrategyDrafting: true,
				Duties:     []string{"构建答辩策略与权利要求修改方案"},
				Boundaries: []string{"禁止同时为请求人构建攻击理由"}},
			{ID: "collegial-judge", Title: "合议组（裁判）", Stance: StanceJudge, HITL: true,
				Duties:     []string{"基于双方主张与证据独立裁定"},
				Boundaries: []string{"禁止参与任一方策略起草"}},
		},
	},
	"oa-response": {
		Scenario: "oa-response",
		Roles: []RoleContract{
			{ID: "examiner", Title: "审查员（意见重构）", Stance: StanceExaminer,
				Duties:     []string{"重构审查意见的指控逻辑与证据链"},
				Boundaries: []string{"禁止替申请人构思答复策略"}},
			{ID: "responder", Title: "答复代理人", Stance: StanceAgentSide, StrategyDrafting: true,
				Duties:     []string{"针对指控逐条构建答复与修改方案"},
				Boundaries: []string{"禁止弱化或回避审查意见中的指控"}},
			{ID: "arbitrator", Title: "定稿裁判", Stance: StanceJudge, HITL: true,
				Duties:     []string{"评审答复方案的争辩强度与修改风险"},
				Boundaries: []string{"禁止参与答复策略起草"}},
		},
	},
	"infringement-analysis": {
		Scenario: "infringement-analysis",
		Roles: []RoleContract{
			{ID: "patentee-side", Title: "专利权人方", Stance: StanceApplicant, StrategyDrafting: true,
				Duties:     []string{"构建全面覆盖/等同主张"},
				Boundaries: []string{"禁止同时构建被告抗辩"}},
			{ID: "defendant-side", Title: "被控侵权方", Stance: StanceDefense, StrategyDrafting: true,
				Duties:     []string{"构建不侵权抗辩（禁止反悔/捐献/现有技术）"},
				Boundaries: []string{"禁止同时构建侵权主张"}},
			{ID: "tech-investigator", Title: "技术调查官", Stance: StanceNeutral,
				Duties: []string{"就技术特征相同性提供中立意见"}},
			{ID: "litigation-judge", Title: "裁判", Stance: StanceJudge, HITL: true,
				Duties:     []string{"综合双方主张与调查意见给出比对结论"},
				Boundaries: []string{"禁止参与任一方策略起草"}},
		},
	},
}

// ResolveTeamComposition 按场景解析角色编排；未知场景返回 false。
// 返回前做立场配对校验——内置编排在编译期即应通过，此处防运行期改动。
func ResolveTeamComposition(scenario string) (TeamComposition, error) {
	t, ok := teamCompositions[scenario]
	if !ok {
		var scenes []string
		for k := range teamCompositions {
			scenes = append(scenes, k)
		}
		sort.Strings(scenes)
		return TeamComposition{}, fmt.Errorf("未知场景 %q（可选: %s）", scenario, strings.Join(scenes, ", "))
	}
	if err := t.Validate(); err != nil {
		return TeamComposition{}, err
	}
	return t, nil
}

// TeamScenarios 返回全部内置场景名（排序）。
func TeamScenarios() []string {
	var scenes []string
	for k := range teamCompositions {
		scenes = append(scenes, k)
	}
	sort.Strings(scenes)
	return scenes
}
