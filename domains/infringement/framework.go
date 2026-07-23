package infringement

// ArticleFrameworkProvider supplies legal framework text for infringement analysis.
// When nil, built-in default frameworks are used as fallback.
type ArticleFrameworkProvider interface {
	InfringementFramework() string
	DefensesFramework() string
	RemediesFramework() string
}

// DefaultFrameworks returns the hardcoded fallback legal frameworks.
func DefaultFrameworks() struct{ Infringement, Defenses, Remedies string } {
	return struct{ Infringement, Defenses, Remedies string }{
		Infringement: defaultInfringementFramework,
		Defenses:     defaultDefensesFramework,
		Remedies:     defaultRemediesFramework,
	}
}

const defaultInfringementFramework = `
## 侵权判定法律框架

### 法律依据
- 《专利法》第11条：未经专利权人许可，任何单位或个人不得以生产经营目的制造、使用、许诺销售、销售、进口专利产品
- 《专利法》第59条第1款：发明或实用新型专利权的保护范围以其权利要求的内容为准，说明书及附图可以用于解释权利要求
- 最高人民法院关于审理侵犯专利权纠纷案件应用法律若干问题的解释 第7条：全部技术特征原则

### 侵权判定步骤
1. 确定专利权的保护范围（权利要求解释）
2. 将被诉侵权技术方案与权利要求进行特征分解
3. 适用全部技术特征规则进行比对
4. 字面侵权不成立时，进一步判断等同侵权
5. 审查是否存在法定抗辩事由

### 全面覆盖原则（全部技术特征规则）
- 被诉侵权技术方案必须包含权利要求记载的全部技术特征，才落入保护范围
- 缺少一个以上技术特征：不落入保护范围
- 增加其他技术特征：仍然构成侵权

### 等同原则
- 等同特征：以基本上相同的手段，实现基本上相同的功能，达到基本上相同的效果
- 必须是本领域普通技术人员无须经过创造性劳动就能联想到的
- 必须针对各个技术特征分别适用，不能整体等同
- "变劣发明"不构成等同侵权

### 禁止反悔原则
- 专利权人在授权或无效程序中对权利要求作出的限制性修改或意见陈述，在侵权诉讼中不得反悔
- 法院可依职权主动适用
- 仅在等同侵权中适用

### 捐献规则
- 仅在说明书或附图中描述但未写入权利要求的技术方案，视为捐献
- 不得通过等同原则重新纳入保护范围
`

const defaultDefensesFramework = `
## 抗辩体系法律框架

### 现有技术抗辩（专利法第62条）
- 被控侵权技术属于现有技术/现有设计的，不构成侵权
- 只能依据一项现有技术进行单独对比
- 被控侵权技术与现有技术完全相同或无实质性差异时抗辩成立
- 不能依据抵触申请进行抗辩
- 在相同侵权和等同侵权中均可适用

### 先用权抗辩（专利法第69条第(二)项）
- 申请日前已制造相同产品/使用相同方法/做好必要准备
- 仅在原有范围内继续制造、使用
- 技术来源必须善意合法
- 是抗辩权而非独立权利，不能单独转让

### 合法来源抗辩（专利法第70条）
- 仅适用于使用者、许诺销售者、销售者
- 需同时证明"不知道"和"合法来源"双重条件
- 不承担赔偿责任但仍需停止侵权

### 权利用尽（专利法第69条第(一)项）
- 专利权人售出产品后，后续使用/销售/许诺销售不侵权
- 不适用于重新制造行为

### 权利冲突抗辩
- 被告后申请专利的行为不能当然免于侵权
- 法院应分析被告专利的具体情况，不能仅以后专利为由驳回原告诉讼请求
`

const defaultRemediesFramework = `
## 救济措施法律框架

### 损害赔偿（专利法第65条）
递进计算顺序：
1. 实际损失 = 侵权产品销售量 × 专利产品合理利润
2. 侵权所得 = 侵权产品销售总数 × 每件侵权产品合理利润
3. 许可费倍数 = 参照许可费 × 1~3倍
4. 法定赔偿 = 1万~500万（2020年修法）
合理开支（调查取证费、律师费等）另行附加

### 临时禁令（专利法第66条）
五大审查要素：
1. 侵权可能性
2. 难以弥补的损害
3. 双方困难权衡
4. 担保情况
5. 公共利益

### 永久禁令
- 侵权成立后停止侵权是基本原则
- 拒绝禁令的例外：公共利益、双方利益重大失衡
- 实用新型/外观设计应提交专利权评价报告

### 惩罚性赔偿（2020年修正）
- 故意侵权可处以确定数额的1~5倍
- 新增举证妨碍规则
`
