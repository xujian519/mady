package guardrails

import (
	"context"
	"strings"
	"testing"

	iface "github.com/xujian519/mady/agentcore/iface"
)

// ============================================================================
// verifyOne 判定矩阵（R1 存在性 + R2 语境相关性）
// ============================================================================

func TestVerifyValidCitation(t *testing.T) {
	report := VerifyCitations("根据专利法第22条第3款的规定，权利要求1具备创造性。")
	if len(report.Flagged) != 0 {
		t.Fatalf("合法引用不得标记: %+v", report.Flagged)
	}
	if report.Total != 1 {
		t.Errorf("Total = %d, 期望 1", report.Total)
	}
}

func TestVerifySuspectHallucination(t *testing.T) {
	// v0.8 真实幻觉案例：分案申请错引专利法第 47 条（无效宣告效力条款）。
	text := "3. **专利法第四十七条第一款（分案申请）**：申请人可以在审查过程中提出分案申请。"
	report := VerifyCitations(text)
	if len(report.Flagged) != 1 {
		t.Fatalf("幻觉引用必须命中, 实际 %d 条: %+v", len(report.Flagged), report.Flagged)
	}
	f := report.Flagged[0]
	if f.Verdict != VerdictSuspect {
		t.Errorf("Verdict = %v, 期望 VerdictSuspect", f.Verdict)
	}
	if !strings.Contains(f.Reason, "分案申请") {
		t.Errorf("Reason 应含用途描述: %q", f.Reason)
	}
}

func TestVerifyInvalidArticle(t *testing.T) {
	report := VerifyCitations("依据专利法第二百零八条，该申请应予驳回。")
	if len(report.Flagged) != 1 || report.Flagged[0].Verdict != VerdictInvalid {
		t.Fatalf("超范围编号应判 Invalid: %+v", report.Flagged)
	}
}

func TestVerifyUnknownArticlePasses(t *testing.T) {
	// 第 64 条（保护范围，2020 版）因 2008→2020 条号漂移被静态表剔除，
	// 核验应落 Unknown 放行，不得误报。
	report := VerifyCitations("依据专利法第64条，保护范围以权利要求的内容为准。")
	if len(report.Flagged) != 0 {
		t.Fatalf("未覆盖条目必须放行: %+v", report.Flagged)
	}
}

func TestVerifyUnverifiablePasses(t *testing.T) {
	// 引用后无用途声明 → Unverifiable 放行。
	report := VerifyCitations("修改符合专利法第33条。")
	if len(report.Flagged) != 0 {
		t.Fatalf("无用途声明的引用必须放行: %+v", report.Flagged)
	}
}

func TestVerifyImplementingRules42(t *testing.T) {
	ok := VerifyCitations("根据专利法实施细则第42条，申请人可以就两项以上发明提出分案申请。")
	if len(ok.Flagged) != 0 {
		t.Fatalf("细则 42 条正确引用不得标记: %+v", ok.Flagged)
	}
	suspect := VerifyCitations("根据专利法实施细则第42条，无效宣告请求应当具体说明理由。")
	if len(suspect.Flagged) != 1 || suspect.Flagged[0].Verdict != VerdictSuspect {
		t.Fatalf("细则 42 条张冠李戴必须命中: %+v", suspect.Flagged)
	}
}

// ============================================================================
// 误报对抗集：正确的实务引用句式必须 100% 放行（设计 §6 防线 #4）
// ============================================================================

func TestAdversarialCorrectCitations(t *testing.T) {
	cases := []string{
		"根据专利法第22条第2款，权利要求1相对对比文件1不具备新颖性。",
		"专利法第22条第3款规定的创造性，是指与现有技术相比具有突出的实质性特点。",
		"说明书未满足专利法第26条第3款充分公开的要求。",
		"权利要求得不到说明书支持，不符合专利法第26条第4款的规定。",
		"该主题属于专利法第25条规定的智力活动规则，不授予专利权。",
		"修改超出了原说明书和权利要求书记载的范围，违反专利法第33条。",
		"两件申请不属于一个总的发明构思，不符合专利法第31条单一性的要求。",
		"申请人可依据专利法第32条随时撤回其专利申请。",
		"发明专利申请自申请日起三年内应提出实质审查请求（专利法第35条）。",
		"对驳回决定不服的，可以依照专利法第41条请求复审。",
		"任何单位或个人均可依据专利法第45条请求宣告该专利权无效。",
		"被宣告无效的专利权视为自始不存在（专利法第47条）。",
		"发明专利权的期限为二十年，自申请日起计算（专利法第42条）。",
		"该申请要求了外国优先权，符合专利法第29条的规定。",
		"专利法第5条规定，违反法律的发明创造不授予专利权。",
		"专利权人未缴纳年费，专利权提前终止（专利法第44条）。",
		"专利法第9条规定同样的发明创造只能授予一项专利权。",
		"根据专利法第11条，未经专利权人许可不得实施其专利。",
		"初步审查符合要求后自申请日起满十八个月即行公布（专利法第34条）。",
		"依据专利法实施细则第42条提出分案申请，不得超出原申请记载的范围。",
		// 以下为 v0.8 真实答案回放中校准出的易误报句式（宽松转述/枚举/版本措辞）：
		"专利法第33条（关于专利权范围的变更）需要审查。",                             // 宽松转述，放行
		"对比文件3构成专利法第9条第1款的禁止重复授权情形。",                           // 重复授权=第9条核心
		"专利法第9条（禁止重复授权）应予适用。",                                  // 同上
		"疾病治疗方法属于专利法规定不得授予专利权的客体（第25条第1款）。",                    // 版本措辞差异
		"依照专利法第5条、第25条的规定，或者依照专利法第9条规定不能取得专利权。",                // 枚举引用
		"不符合专利法第22条、第23条、第26条第3款、第26条第4款所列的情形。",                // 枚举引用
		"申请不符合专利法第26条第3款、第26条第4款、第27条第2款、第33条或者本细则第20条第2款的规定。", // 长枚举
		// 以下为第二轮回放（L3 层）校准出的易误报句式：
		"专利法第31条：一件发明或者实用新型专利申请应当限于一项发明或者实用新型。",       // 逐字引用第31条原文（patent_exam_2012_a31_02）
		"请求人在此阶段增加全新的无效宣告理由（专利法第九条），严重违背了一次性提交的程序要求。", // 无效理由同位命名第9条（patent_exam_2009_a2_02）
	}
	for _, text := range cases {
		if report := VerifyCitations(text); len(report.Flagged) != 0 {
			t.Errorf("误报: %q → %+v", text, report.Flagged)
		}
	}
}

// 真实答案回放确认的"换条幻觉"（非 2008_a31_02 案例），必须命中。
func TestVerifyCrossArticleHallucinations(t *testing.T) {
	cases := []string{
		// 抵触申请（现有技术）的规定在第 22 条，错引第 33 条。
		"**《专利法》第33条**：关于申请日之前的申请作为现有技术。",
		// 实用新型定义在第 2 条，错引第 22 条。
		"**《专利法》第2条、第22条**：实用新型专利保护产品的形状、构造或者其结合。",
		// 权利要求清楚/简要在第 26 条，错引第 42 条。
		"**《专利法》第42条**：规定了权利要求书应当清楚、简要。",
		// 智力活动规则（第 25 条客体）错引第 22 条（回放 patent_exam_2013_a26_01 确认的真实错误）。
		`根据《专利法》第22条，单纯的"印制文字"或"广告宣传行为"属于智力活动规则或单纯的商业行为，不具备技术性。`,
	}
	for _, text := range cases {
		report := VerifyCitations(text)
		if len(report.Flagged) == 0 {
			t.Errorf("换条幻觉必须命中: %q", text)
		}
	}
}

// ============================================================================
// Hook 行为（AfterModelCall 处置）
// ============================================================================

// callHook 构造一次 AfterModelCall 调用并返回修改后的 ModelCallContext。
func callHook(t *testing.T, hook iface.LifecycleHook, content string) *iface.ModelCallContext {
	t.Helper()
	ifaceMCC := &iface.ModelCallContext{Content: content}
	hook.AfterModelCall(context.Background(), nil, ifaceMCC)
	return ifaceMCC
}

func TestGateAnnotatesSuspect(t *testing.T) {
	hook := NewCitationGate(WithCitationGateLevel(LevelStandard))
	mcc := callHook(t, hook, "分析如下：专利法第47条（分案申请）允许申请人提出分案。")
	if !strings.Contains(mcc.Content, "引用核验提示") {
		t.Errorf("命中疑点应追加提示: %q", mcc.Content)
	}
	if !strings.Contains(mcc.Content, "第47条") {
		t.Errorf("提示应含被标记引用: %q", mcc.Content)
	}
}

func TestGateSkipsToolCallTurns(t *testing.T) {
	hook := NewCitationGate()
	mcc := &iface.ModelCallContext{
		Content:      "专利法第47条（分案申请）。",
		HasToolCalls: true,
	}
	hook.AfterModelCall(context.Background(), nil, mcc)
	if strings.Contains(mcc.Content, "引用核验提示") {
		t.Errorf("工具调用回合不得标注: %q", mcc.Content)
	}
}

func TestGateCleanPassThrough(t *testing.T) {
	hook := NewCitationGate(WithCitationGateLevel(LevelStandard))
	original := "根据专利法第22条第3款，权利要求1具备创造性。"
	mcc := callHook(t, hook, original)
	if mcc.Content != original {
		t.Errorf("合法答案不得改动: %q", mcc.Content)
	}
}

func TestGateRecorderCalledAtStandard(t *testing.T) {
	var got CitationReport
	var gotContent string
	hook := NewCitationGate(
		WithCitationGateLevel(LevelStandard),
		WithCitationRecorder(func(r CitationReport, content string) { got, gotContent = r, content }),
	)
	callHook(t, hook, "专利法第47条（分案申请）。")
	if len(got.Flagged) != 1 {
		t.Fatalf("Recorder 应收到 1 条标记, 实际 %+v", got)
	}
	// content 必须是追加提示之前的原始输出（供留痕 OriginalOutput）。
	if gotContent != "专利法第47条（分案申请）。" {
		t.Errorf("Recorder content 应为原始输出, 实际 %q", gotContent)
	}
}

func TestGateRecorderNotCalledAtLight(t *testing.T) {
	called := false
	hook := NewCitationGate(
		WithCitationGateLevel(LevelLight),
		WithCitationRecorder(func(CitationReport, string) { called = true }),
	)
	callHook(t, hook, "专利法第47条（分案申请）。")
	if called {
		t.Error("Light 档不得触发 Recorder")
	}
}

// TestGateStrictSuppressesPersist 验证 Strict 档（P2b）：命中疑点时
// SuppressPersist=true（未复核输出不入库），但提示仍追加、Recorder 仍回调——
// 用户可见本次输出，仅持久化被抑制，执行不阻断。
func TestGateStrictSuppressesPersist(t *testing.T) {
	var got CitationReport
	hook := NewCitationGate(
		WithCitationGateLevel(LevelStrict),
		WithCitationRecorder(func(r CitationReport, _ string) { got = r }),
	)
	mcc := callHook(t, hook, "专利法第47条（分案申请）。")
	if !mcc.SuppressPersist {
		t.Error("Strict 档命中疑点必须 SuppressPersist=true")
	}
	if !strings.Contains(mcc.Content, "引用核验提示") {
		t.Error("Strict 档仍应追加存疑提示（用户可见）")
	}
	if len(got.Flagged) != 1 {
		t.Error("Strict 档仍应触发 Recorder 留痕")
	}
}

// TestGateStandardDoesNotSuppress 验证 Standard 档不抑制持久化——
// SuppressPersist 是 Strict 专属处置，Standard 仅标注+留痕。
func TestGateStandardDoesNotSuppress(t *testing.T) {
	hook := NewCitationGate(WithCitationGateLevel(LevelStandard))
	mcc := callHook(t, hook, "专利法第47条（分案申请）。")
	if mcc.SuppressPersist {
		t.Error("Standard 档不得 SuppressPersist")
	}
}

func TestGateNilResponseSafe(t *testing.T) {
	hook := NewCitationGate()
	// nil mcc 不得 panic。
	hook.AfterModelCall(context.Background(), nil, nil)
}
