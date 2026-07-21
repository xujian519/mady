package evaluate

// GuardrailsTestCase 是护栏评估测试用例。
// 验证护栏系统在不同等级下的行为是否符合预期。
type GuardrailsTestCase struct {
	// ID 是测试用例的唯一标识。
	ID string

	// GuardrailLevel 指定护栏等级：light / standard / strict。
	GuardrailLevel string

	// Input 是模型输出的内容，护栏系统将对此进行检查。
	Input string

	// Context 是对话上下文，影响护栏的判断。
	Context string

	// ShouldFlag 表示护栏是否应触发（标记为需要关注）。
	ShouldFlag bool

	// MinFlagCount 是最少触发次数，用于统计测试
	// （同一输入可能经过多次护栏检查）。
	MinFlagCount int

	// ExpectedAction 是期望的护栏动作：allow / deny / rewrite / ask。
	ExpectedAction string
}

// ============================================================================
// GuardrailsMetric
// ============================================================================

// GuardrailsMetric 评估护栏系统的行为正确性。
// 测量护栏触发准确率 —— 正确拦截违规内容（正判）与不误报正常内容（负判）的比例。
//
// 作为 Metric 接口的实现，prediction 参数表示护栏实际采取的动作，
// reference 参数表示期望的动作。二者一致时得 1，不一致得 0。
//
// 批量评估时，WithThreshold 可设置通过阈值。
type GuardrailsMetric struct {
	threshold float64
}

// NewGuardrailsMetric 创建一个护栏准确性指标，默认阈值为 0.8。
func NewGuardrailsMetric() *GuardrailsMetric {
	return &GuardrailsMetric{threshold: 0.8}
}

// WithThreshold 设置通过阈值 [0, 1]。低于阈值视为不通过。
func (m *GuardrailsMetric) WithThreshold(t float64) *GuardrailsMetric {
	m.threshold = t
	return m
}

// Threshold 返回当前通过阈值。
func (m *GuardrailsMetric) Threshold() float64 { return m.threshold }

// Name 返回指标标识符 "guardrails_accuracy"。
func (m *GuardrailsMetric) Name() string { return "guardrails_accuracy" }

// Compute 比较实际护栏动作与期望动作是否一致。
// prediction: 护栏实际采取的动作（"allow" / "deny" / "rewrite" / "ask"）。
// reference:  期望的护栏动作。
// 返回 1.0（一致）或 0.0（不一致）。
func (m *GuardrailsMetric) Compute(prediction, reference string) float64 {
	if prediction == reference {
		return 1.0
	}
	return 0.0
}

// EvaluateGuardrailsBatch 批量评估护栏测试用例。
// guardrailFn 接受 (input, context, level) 返回 (action, error)，
// 由调用方注入真实或 mock 实现。
// 返回 (准确率, 正判数, 总用例数)。
func EvaluateGuardrailsBatch(
	cases []GuardrailsTestCase,
	guardrailFn func(input, context, level string) (string, error),
) (accuracy float64, correct, total int) {
	total = len(cases)
	if total == 0 {
		return 1.0, 0, 0
	}

	correct = 0
	for _, tc := range cases {
		action, err := guardrailFn(tc.Input, tc.Context, tc.GuardrailLevel)
		if err != nil {
			continue // 执行错误不计入正确
		}

		if tc.ExpectedAction != "" && action == tc.ExpectedAction {
			correct++
		} else if tc.ExpectedAction == "" && tc.ShouldFlag == (action != "allow") {
			correct++
		}
	}

	accuracy = float64(correct) / float64(total)
	return accuracy, correct, total
}
