package novelty

import (
	"context"
	"fmt"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/graph"
)

// =============================================================================
// runLLMStep — 消除 LLM 节点的样板重复代码
// =============================================================================

// runLLMStep 封装了所有 LLM 节点的通用执行骨架：
// 跳过检查 → 输入提取 → prompt 构建 → Agent 创建 → LLM 调用 → 结果存储。
// 各节点只需提供 buildPrompt 回调，其余逻辑完全一致。
func runLLMStep(ctx context.Context, state graph.PregelState,
	provider agentcore.Provider,
	name string,
	schema map[string]any,
	stateKey string,
	buildPrompt func(input *NoveltyInput) string) (graph.PregelState, error) {

	if stateHasSkip(state) {
		return state, nil
	}

	input := extractInput(state)
	if input == nil {
		return state, nil
	}

	prompt := buildPrompt(input)
	inputText := buildInputText(input)
	agent := newNoveltyAgent(provider, name, prompt, schema)
	defer agent.Close()

	output, err := agent.Run(ctx, inputText)
	if err != nil {
		return state, fmt.Errorf("%s: %w", name, err)
	}

	state[stateKey] = output
	return state, nil
}

// =============================================================================
// 节点实现
// =============================================================================

// loadInputNode 从 PregelState 读取 NoveltyInput。
// 当 EvidenceCoverage == "none" 或无对比文件时跳过，设置 Skipped=true。
func loadInputNode() graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		raw, ok := state[StateKeyNoveltyInput]
		if !ok {
			state[StateKeyNoveltyResult] = &NoveltyResult{
				Assessed:   false,
				Skipped:    true,
				SkipReason: "未提供输入数据（novelty_input state key 为空）",
			}
			return state, nil
		}

		input, ok := raw.(*NoveltyInput)
		if !ok || input == nil {
			state[StateKeyNoveltyResult] = &NoveltyResult{
				Assessed:   false,
				Skipped:    true,
				SkipReason: "输入数据格式无效",
			}
			return state, nil
		}

		if input.EvidenceCoverage == "none" ||
			(len(input.PriorArtDocs) == 0 && len(input.ConflictApps) == 0) {
			state[StateKeyNoveltyResult] = &NoveltyResult{
				Assessed:   false,
				Skipped:    true,
				SkipReason: "无检索证据或对比文件，无法进行新颖性评估",
			}
			return state, nil
		}

		state[StateKeyNoveltyInput] = input
		return state, nil
	}
}

// stepPriorArtCheckNode 现有技术审查节点。
// 覆盖：A22.5 现有技术定义、"为公众所知"严格/宽松标准、
// 书面公开、互联网公开、公开使用与默示保密义务、销售公开、充分公开要求。
func stepPriorArtCheckNode(provider agentcore.Provider) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		return runLLMStep(ctx, state, provider, "novelty-prior-art", priorArtSchema(), stateKeyPriorArt,
			func(input *NoveltyInput) string {
				prompt := "你是一名资深专利审查员。请执行新颖性评估的第 1 步：现有技术审查。\n\n"
				prompt += personSkilledDefinition() + "\n\n"
				prompt += "**法条依据**：《专利法》第 22 条第 5 款\n"
				prompt += "「本法所称现有技术，是指申请日以前在国内外为公众所知的技术。」\n\n"
				prompt += "**审查指南依据**：审查指南（2023 修订）第二部分第三章第 2.1 节\n\n"
				prompt += "### 判断流程\n\n"
				prompt += "#### [1] 确定有效申请日\n"
				prompt += "- 以申请日为基准；如主张优先权则以优先权日为基准\n"
				prompt += "- 申请当日公开的技术不构成现有技术（即公开日=申请日时，不破坏新颖性）\n"
				if input.PriorityDate != "" {
					prompt += "- **注意**：本申请主张优先权，优先权日为 " + input.PriorityDate + "，以此为准\n"
				}
				prompt += "\n"
				prompt += "#### [2] 判断是否「为公众所知」\n\n"
				prompt += "对每篇对比文件，判断其在申请日前是否已处于为公众所知的状态。\n\n"
				prompt += "**认定标准**：\n"
				prompt += "- 严格标准：技术内容已脱离保密状态，有可能被一个不负保密义务的人知悉\n"
				prompt += "- 宽松标准（审查指南采用）：技术内容处于「公众想得知就能够得知」的状态，\n"
				prompt += "  不取决于是否有公众实际得知\n\n"
				prompt += "**书面公开（出版物公开）**：\n"
				prompt += "- 专利法意义上的出版物范围极广：书籍、期刊、专利文献、学术论文、\n"
				prompt += "  技术手册、产品样本、广告宣传册、缩微胶片、光盘等\n"
				prompt += "- 互联网/在线数据库形式存在的文件也属于出版物\n"
				prompt += "- 关键不在于复制件数目，而在于流通渠道是否对公众开放\n"
				prompt += "- 印有「内部资料」「内部发行」等字样且确系在特定范围内发行并保密的，\n"
				prompt += "  不属于公开出版物\n"
				prompt += "- 出版发行量多少、是否有人阅读过、申请人是否知道，均无关紧要\n\n"
				prompt += "**互联网公开**：\n"
				prompt += "- 互联网上的信息资料属于出版物公开的一种特殊形式\n"
				prompt += "- 判断标准：信息是否发布在**对公众开放**的平台上\n"
				prompt += "- 需要密码或审批才能访问 → 一般不构成公开\n"
				prompt += "- 完全开放访问 → 构成公开\n"
				prompt += "- 注册即可访问（无需审核）→ 一般构成公开\n"
				prompt += "- 网页被搜索引擎收录可作为公开的佐证，但非决定性因素\n\n"
				prompt += "**公开使用**：\n"
				prompt += "- 使用公开包括：制造、使用、销售、进口、交换、馈赠、演示、展出等\n"
				prompt += "- 产品置于公共场所但内部技术特征无法从外观观察得知 → 不构成使用公开\n"
				prompt += "- 即使需破坏产品才能得知其结构和功能 → 也属于使用公开\n"
				prompt += "- 默示保密义务可来自社会观念或商业习惯（如新产品试验中的合作方）\n"
				prompt += "- 单纯设备买卖关系不产生默示保密义务\n\n"
				prompt += "**销售公开**：\n"
				prompt += "- 产品销售广告如果能确认销售的就是专利产品 → 构成销售公开\n"
				prompt += "- 但如果产品的技术方案（如配方）无法通过分析产品本身获得 → 该技术方案可申请专利\n\n"
				prompt += "**口头公开**（以其他方式为公众所知）：\n"
				prompt += "- 包括口头交谈、报告、讨论会发言、广播、电视、电影等\n"
				prompt += "- 公开日：口头交谈以发生之日为准，广播电视以播放日为准\n"
				prompt += "- 举证难度极大：事后难以证明谈话内容是否足够详细到使技术人员能实施\n\n"
				prompt += "#### [3] 充分公开（可实施性）要求\n"
				prompt += "- 一项在先文献要构成现有技术，必须对技术方案作了完整、清楚、准确的描述，\n"
				prompt += "  使得该领域的熟练技术人员能够实施该发明方案\n"
				prompt += "- 两层含义：客观上能够实施 + 主观上相信能够实施\n"
				prompt += "- 公众「可以看到」或「接触到」某一产品，并不等于该产品的具体技术方案\n"
				prompt += "  已经处于公开状态（如可口可乐配方）\n\n"
				prompt += "请对输入中的每篇对比文件逐一分析，然后输出 JSON 格式。"
				return prompt
			})
	}
}

// stepSingleCompareNode 单独对比节点。
// 覆盖：单独对比原则、全部特征对比、上下位概念、惯用手段直接置换、
// 数值范围 8 种情形、四要素综合判断、性能/参数/用途/制备方法特征。
func stepSingleCompareNode(provider agentcore.Provider) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		return runLLMStep(ctx, state, provider, "novelty-compare", compareSchema(), stateKeyCompare,
			func(input *NoveltyInput) string {
				prompt := "你是一名资深专利审查员。请执行新颖性评估的第 2 步：单独对比。\n\n"
				prompt += personSkilledDefinition() + "\n\n"
				prompt += "**法条依据**：《专利法》第 22 条第 2 款\n"
				prompt += "「新颖性，是指该发明或者实用新型不属于现有技术……」\n\n"
				prompt += "**审查指南依据**：审查指南（2023 修订）第二部分第三章第 3 节\n\n"
				prompt += "### 核心原则\n"
				prompt += "- **单独对比原则**：只能将一项权利要求与**单独一份现有技术**进行对比，\n"
				prompt += "  不得将两份或多份现有技术组合起来与权利要求对比\n"
				prompt += "- **全部特征对比**：只有当一项现有技术公开了权利要求的**全部技术特征**，\n"
				prompt += "  才能得出不具备新颖性的结论\n"
				prompt += "- 新颖性判断的方法与专利侵权判断中的字面侵权思路一致：如果实施现有技术\n"
				prompt += "  将对权利要求构成字面侵权，则该现有技术破坏新颖性\n\n"
				prompt += "### 判断流程\n\n"
				prompt += "#### [1] 提取技术特征\n"
				prompt += "- 将权利要求的技术方案拆分为独立的技术特征列表\n"
				prompt += "- 从对比文件中提取公开的技术特征\n\n"
				prompt += "#### [2] 逐项比对 —— 判断以下类型\n\n"
				prompt += "**2.1 简单文字变换**\n"
				prompt += "- 权利要求与对比文件区别仅在于文字表述不同但技术实质相同 → 无新颖性\n\n"
				prompt += "**2.2 上下位概念**\n"
				prompt += "- 对比文件公开**下位概念**（如「铜」）→ 破坏**上位概念**（如「金属」）的新颖性\n"
				prompt += "- 对比文件公开**上位概念**（如「金属」）→ 不破坏**下位概念**（如「铜」）的新颖性\n"
				prompt += "- 下位概念破坏上位概念的新颖性，反之则不可\n\n"
				prompt += "**2.3 惯用手段的直接置换**\n"
				prompt += "- 如果权利要求与对比文件的区别仅仅是所属技术领域的**惯用手段的直接置换**，\n"
				prompt += "  则不具备新颖性\n"
				prompt += "- 示例：螺钉↔螺栓、皮带传动↔链条传动\n"
				prompt += "- 判断需以本领域技术人员的知识水平为标准\n"
				prompt += "- 注意：这一标准的适用容易模糊新颖性与创造性的界限，应审慎使用\n\n"
				prompt += "**2.4 数值范围（8 种判断情形）**\n"
				prompt += "情形一：对比文件的具体数值落在权利要求数值范围内 → 破坏新颖性\n"
				prompt += "  例：权利要求 10%-35%，对比文件 20% → 无新颖性\n"
				prompt += "情形二：对比文件的数值范围与权利要求范围部分重叠或有一个共同端点 → 破坏新颖性\n"
				prompt += "  例：权利要求 1-10h，对比文件 4-12h → 无新颖性（4-10h重叠）\n"
				prompt += "情形三：对比文件范围端点等于权利要求的离散值 → 破坏该离散值的新颖性\n"
				prompt += "  例：权利要求离散温度 40℃/58℃/75℃/100℃，对比文件 40℃-100℃ → 40℃和100℃无新颖性\n"
				prompt += "情形四：权利要求的离散值在对比文件范围内但非端点 → 不破坏新颖性\n"
				prompt += "  例：上例中 58℃和75℃ → 具有新颖性\n"
				prompt += "情形五：权利要求的具体数值或范围完全落在对比文件范围内且无共同端点 → 不破坏新颖性\n"
				prompt += "  例：权利要求 95mm，对比文件 70-105mm → 有新颖性\n"
				prompt += "情形六：对比文件范围宽于权利要求范围，但前者完全包围后者且无共同端点 → 不破坏\n"
				prompt += "情形七：封闭式撰写（「由……组成」）vs 开放式撰写（「含有……」）→ 影响新颖性判断\n"
				prompt += "情形八：数值范围的端点破坏离散数值端点的新颖性，但不破坏中间值的新颖性\n\n"
				prompt += "**2.5 四要素综合判断**\n"
				prompt += "当技术方案存在差异时，需综合考量以下四个要素是否实质上相同：\n"
				prompt += "1. **技术领域**：是否相同或相近\n"
				prompt += "2. **所解决的技术问题**：是否相同（注意：不要求对比文件明确撰写其技术问题，\n"
				prompt += "   只要本领域技术人员判断后能确定二者解决相同问题即可）\n"
				prompt += "3. **技术方案**：是否实质上相同\n"
				prompt += "4. **预期效果**：是否相同\n"
				prompt += "注意：四要素中技术方案是核心要素。如果技术方案完全相同，无需再看其他三要素。\n"
				prompt += "即使特征完全公开，如果技术问题或预期效果实质性不同 → 可能仍具备新颖性。\n"
				prompt += "**防范「事后诸葛亮」**：不得在知晓发明内容后反向推导其与现有技术的相同性。\n\n"
				prompt += "**2.6 性能/参数/用途/制备方法特征**\n"
				prompt += "- 性能、参数特征：如果隐含了特定结构/组成 → 则有新颖性；否则可推定无新颖性\n"
				prompt += "- 用途特征：如果用途由产品固有特性决定且未改变结构/组成 → 则无新颖性\n"
				prompt += "- 制备方法特征：如果方法导致产品结构/组成不同 → 则有新颖性\n\n"
				prompt += "#### [3] 隐含公开内容\n"
				prompt += "- 对比文件虽然没有明确文字记载，但本领域技术人员从上下文能**直接、毫无疑义地**\n"
				prompt += "  推导出的内容，也属于对比文件公开的内容\n"
				prompt += "- 「隐含公开」的标准是「直接、毫无疑义地确定」，不同于创造性判断中的「启示」标准\n"
				prompt += "- 对比文件的优选实施方式中公开的内容可用于评价新颖性\n\n"
				prompt += "#### [4] 权利要求撰写方式的影响\n"
				prompt += "- 封闭式撰写（「由……组成」）：即使与现有组合物解决的技术问题相同 → 仍有新颖性\n"
				prompt += "- 开放式撰写（「含有……」）：如果与现有组合物解决的技术问题相同 → 无新颖性\n"
				prompt += "- 排除法撰写（指明不含某组分）：仍有新颖性\n\n"
				prompt += "请对输入中的每项权利要求与每篇对比文件进行完整的单独对比分析，然后输出 JSON 格式。"
				return prompt
			})
	}
}

// stepConflictCheckNode 抵触申请审查节点。
// 覆盖：抵触申请三要件、全文内容制、效力的不可逆性、2008 修法变化。
func stepConflictCheckNode(provider agentcore.Provider) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		if stateHasSkip(state) {
			return state, nil
		}
		input := extractInput(state)
		if input == nil || len(input.ConflictApps) == 0 {
			state[stateKeyConflict] = ""
			return state, nil
		}

		return runLLMStep(ctx, state, provider, "novelty-conflict", conflictSchema(), stateKeyConflict,
			func(input *NoveltyInput) string {
				prompt := "你是一名资深专利审查员。请执行新颖性评估的第 3 步：抵触申请审查。\n\n"
				prompt += personSkilledDefinition() + "\n\n"
				prompt += "**法条依据**：《专利法》第 22 条第 2 款（抵触申请部分）\n"
				prompt += "「……也没有任何单位或者个人就同样的发明或者实用新型在申请日以前\n"
				prompt += "  向国务院专利行政部门提出过申请，并记载在申请日以后公布的专利申请\n"
				prompt += "  文件或者公告的专利文件中。」\n\n"
				prompt += "**审查指南依据**：审查指南（2023 修订）第二部分第三章第 2.2 节\n\n"
				prompt += "### 抵触申请三要件（须同时满足）\n\n"
				prompt += "**要件一：时间条件**\n"
				prompt += "- 在先申请的申请日（有优先权的指优先权日）**早于**在后申请的申请日（优先权日）\n"
				prompt += "- 两件申请的申请日相同 → 不构成抵触申请（适用《专利法》第 9 条禁止重复授权原则）\n\n"
				prompt += "**要件二：公开条件**\n"
				prompt += "- 在先申请的公开日期（或公告日期）**晚于或等于**在后申请的申请日\n"
				prompt += "- 即在先申请在在后申请日时处于「秘密状态」，在后申请日后才公开\n\n"
				prompt += "**要件三：内容条件**\n"
				prompt += "- 在后申请的权利要求所要求保护的技术方案已被在先申请的**完整申请文件**所披露\n"
				prompt += "- 以在先申请的**全文**为比对基础（说明书 + 权利要求书 + 附图），不限于权利要求书\n"
				prompt += "- 即使在先申请人对某项技术方案未提出权利要求，只要在说明书中记载 → 仍构成抵触\n\n"
				prompt += "### 抵触申请的重要特性\n\n"
				prompt += "**1. 效力的不可逆性**\n"
				prompt += "- 在先申请无论后续结局如何（撤回、视为撤回、被驳回、被放弃、被宣告无效），\n"
				prompt += "  其作为在后申请的抵触申请的效力不变\n"
				prompt += "- 但在先申请如果在公开之前被申请人撤回 → 技术方案不再进入公共领域，\n"
				prompt += "  不构成抵触申请\n\n"
				prompt += "**2. 仅限中国申请**\n"
				prompt += "- 抵触申请必须是向中国国务院专利行政部门提出的中国专利申请\n"
				prompt += "- PCT 进入中国国家阶段的申请也属于「向国务院专利行政部门提出」\n"
				prompt += "- 外国申请不构成抵触申请\n\n"
				prompt += "**3. 不构成现有技术**\n"
				prompt += "- 抵触申请不构成现有技术的一部分（因为在后申请日之前尚未公开）\n"
				prompt += "- 抵触申请**仅用于新颖性判断**（比对技术方案是否相同），\n"
				prompt += "  **不能用于创造性判断**（不能结合其他文献评价显而易见性）\n\n"
				prompt += "**4. 适用新颖性全部判断原则**\n"
				prompt += "- 抵触申请审查同样适用单独对比原则\n"
				prompt += "- 适用上下位概念、惯用手段直接置换、数值范围等规则\n"
				prompt += "- 适用四要素综合判断（领域/问题/方案/效果）\n\n"
				prompt += "**5. 外观设计不构成抵触申请**\n"
				prompt += "- 外观设计专利申请不能作为发明和实用新型的抵触申请\n\n"
				prompt += "### 2008 年修法的重要变化\n"
				prompt += "- 2008 年修改前：抵触申请的范围限于「他人」申请，不包括申请人自身\n"
				prompt += "- 2008 年修改后：扩大到「任何单位或者个人」（包括申请人自身）\n"
				prompt += "- 同一申请人的在先申请也可以构成在后申请的抵触申请\n"
				prompt += "- 同时将「申请日以后公布的专利申请文件」改为包含「公告的专利文件」\n\n"
				prompt += "请对每件抵触申请逐一分析，然后输出 JSON 格式。"
				return prompt
			})
	}
}

// stepGracePriorityNode 宽限期与优先权节点。
// 覆盖：A24 宽限期三种情形、6 个月期限、第三方独立公开穿透、
// A29 国际优先权与本国优先权、"相同主题"四要素判断。
func stepGracePriorityNode(provider agentcore.Provider) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		return runLLMStep(ctx, state, provider, "novelty-grace-priority", gracePrioritySchema(), stateKeyGracePriority,
			func(input *NoveltyInput) string {
				prompt := "你是一名资深专利审查员。请执行新颖性评估的第 4 步：宽限期与优先权例外审查。\n\n"
				prompt += personSkilledDefinition() + "\n\n"
				prompt += "### 一、宽限期（《专利法》第 24 条）\n\n"
				prompt += "**法条原文**：\n"
				prompt += "「申请专利的发明创造在申请日以前六个月内，有下列情形之一的，不丧失新颖性：\n"
				prompt += "  （一）在中国政府主办或者承认的国际展览会上首次展出的；\n"
				prompt += "  （二）在规定的学术会议或者技术会议上首次发表的；\n"
				prompt += "  （三）他人未经申请人同意而泄露其内容的。」\n\n"
				prompt += "**三种法定情形**：\n\n"
				prompt += "**情形一：国际展览会首次展出**\n"
				prompt += "- 「中国政府承认的国际展览会」是指国际展览局注册或认可的国际展览会（范围很有限）\n"
				prompt += "- 「展出」包括实物展示、图片照片展示、散发书面资料、销售展品等\n"
				prompt += "- 「首次」意味着第一次展出须在宽限期内，但不排除后续多次展出\n\n"
				prompt += "**情形二：学术/技术会议首次发表**\n"
				prompt += "- 会议范围：仅限于国务院有关主管部门或全国性学术团体组织召开的会议\n"
				prompt += "- 仅限在我国举办的会议（不存在「中国政府承认的外国会议」）\n"
				prompt += "- 「发表」包括口头报告和书面资料\n"
				prompt += "- 会议须是公开举行的（参加者不负有保密义务）\n\n"
				prompt += "**情形三：他人未经申请人同意泄露**\n"
				prompt += "- 他人公开的发明创造必须是直接或间接从申请人处获知的\n"
				prompt += "- 他人独立作出的或从独立第三人处获知的 → 不能适用\n"
				prompt += "- 申请人应当事先采取了防止泄露的必要措施（明示保密要求或默示保密义务）\n\n"
				prompt += "**宽限期的效力限制**：\n"
				prompt += "- 宽限期为 **6 个月**（自首次公开日起算）\n"
				prompt += "- 宽限期**仅保护申请人的特定公开行为**，不是将公开日视为申请日\n"
				prompt += "- 第三方**独立**作出同样的发明并在宽限期内提出申请 → 双方都不能获得专利权\n"
				prompt += "- 宽限期内申请人以其他方式公开（如在出版物上发表）→ 仍影响新颖性\n"
				prompt += "- 他人从展览会或会议获知后再以其他方式公开 → 可能影响新颖性\n"
				prompt += "- 程序要求：第（一）（二）项须在申请时声明 + 2 个月内提交证明文件\n\n"
				prompt += "### 二、优先权（《专利法》第 29 条）\n\n"
				prompt += "**法条原文**：\n"
				prompt += "「申请人自发明或者实用新型在外国第一次提出专利申请之日起十二个月内，\n"
				prompt += "  又在中国就相同主题提出专利申请的，……可以享有优先权。」\n\n"
				prompt += "**国际优先权**：\n"
				prompt += "- 期限：发明和实用新型为 12 个月，外观设计为 6 个月\n"
				prompt += "- 要求：在中国就相同主题提出专利申请\n"
				prompt += "- 「相同主题」指**技术领域、所解决的技术问题、技术方案和预期效果相同**\n\n"
				prompt += "**本国优先权**：\n"
				prompt += "- 期限：12 个月\n"
				prompt += "- 在先申请自中国在后申请提出之日起即被视为撤回\n"
				prompt += "- 不得作为本国优先权基础的情形：已要求过优先权的、已授权的、分案申请的\n"
				prompt += "- 主要作用：发明与实用新型之间的转换、合并多个在先申请\n\n"
				prompt += "**优先权的审查要点**：\n"
				prompt += "- 「相同主题」的判断标准：在后申请的权利要求是否可从在先申请中\n"
				prompt += "  **直接、毫无疑义地得出**\n"
				prompt += "- 文字表述不同但技术实质相同 → 优先权成立\n"
				prompt += "- 在后申请增加在先申请未记载的技术特征 → 该权利要求不能享受优先权\n"
				prompt += "- 一件申请可以要求多项优先权（可以来自不同国家）\n"
				prompt += "- 部分优先权：除首次申请记载的内容外，可以包含新的技术方案\n"
				prompt += "- 组合方案不能享受优先权（由两件以上独立申请的不同特征组合而成）\n\n"
				prompt += "**优先权的效力**：\n"
				prompt += "- 优先权日之后的公开不损害在后申请的新颖性\n"
				prompt += "- 不能产生第三方的先用权\n"
				prompt += "- 与最初申请是否获得授权无直接联系\n\n"
				prompt += "请根据输入信息判断是否存在宽限期或优先权的例外情形，然后输出 JSON 格式。"
				return prompt
			})
	}
}

// stepSpecialDomainNode 特殊领域节点。
// 仅当 TechDomain 非空时通过条件边激活。
func stepSpecialDomainNode(provider agentcore.Provider) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		if stateHasSkip(state) {
			return state, nil
		}
		input := extractInput(state)
		if input == nil {
			return state, nil
		}

		return runLLMStep(ctx, state, provider, "novelty-special-domain", specialDomainSchema(), stateKeySpecialDomain,
			func(input *NoveltyInput) string {
				prompt := "你是一名资深专利审查员。请执行特殊领域的新颖性判断。\n\n"
				prompt += personSkilledDefinition() + "\n\n"
				prompt += "当前技术领域：" + input.TechDomain + "\n\n"
				switch input.TechDomain {
				case "chemistry":
					prompt += chemistryNoveltyFramework()
				default:
					prompt += "请分析该领域特殊的新颖性判断规则。\n"
				}
				prompt += "\n请分析该领域特殊规则对新颖性判断的影响，然后输出 JSON 格式。"
				return prompt
			})
	}
}

// generateConclusionNode 汇总所有步骤的产出，生成最终新颖性评估结论。
func generateConclusionNode(provider agentcore.Provider) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		if stateHasSkip(state) {
			return state, nil
		}

		priorArt := getStateString(state, stateKeyPriorArt)
		compare := getStateString(state, stateKeyCompare)
		conflict := getStateString(state, stateKeyConflict)
		gracePriority := getStateString(state, stateKeyGracePriority)
		specialDomain := getStateString(state, stateKeySpecialDomain)

		prompt := "你是一名资深专利审查员。请基于新颖性评估各步骤的产出，生成最终的结构化评估结论。\n\n"
		prompt += personSkilledDefinition() + "\n\n"
		prompt += "**判断逻辑**：新颖性 = 现有技术未覆盖全部特征 AND 抵触申请不成立\n"
		prompt += "（在考虑宽限期和优先权例外后，如果对比文件未公开权利要求的全部技术特征，\n"
		prompt += " 且不存在有效的抵触申请，则权利要求具备新颖性。）\n\n"
		prompt += "结论应包含：\n"
		prompt += "1. 整体判断：该技术方案是否具备新颖性\n"
		prompt += "2. 不具备新颖性的权利要求编号列表\n"
		prompt += "3. 置信度：high/medium/low\n\n"
		prompt += "请输出 JSON 格式。"

		inputText := fmt.Sprintf("第 1 步（现有技术审查）:\n%s\n\n第 2 步（单独对比）:\n%s\n\n第 3 步（抵触申请审查）:\n%s\n\n第 4 步（宽限期与优先权）:\n%s\n\n特殊领域:\n%s",
			priorArt, compare, conflict, gracePriority, specialDomain)

		agent := newNoveltyAgent(provider, "novelty-conclusion", prompt, conclusionSchema())
		defer agent.Close()

		output, err := agent.Run(ctx, inputText)
		if err != nil {
			return state, fmt.Errorf("generate_conclusion: %w", err)
		}

		result := buildResult(priorArt, compare, conflict, gracePriority, output)

		state[StateKeyNoveltyResult] = result
		return state, nil
	}
}
