// Package workercontract provides the patent_worker_validate tool for
// checking patent Worker outputs against their declared contracts.
//
// Unlike Sati's text-field validator, Mady's Worker contracts are path-based
// and tied to the Pregel state machine. This tool bridges the two models by
// performing a lightweight content-level check against the built-in Worker
// catalog (agentcore/worker.DefaultWorkers). Hard-contract keywords must be
// present; soft-contract keywords are reported but do not degrade the output.
package workercontract

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/agentcore/worker"
	"github.com/xujian519/mady/domains/provenance"
)

// provenanceLog 是包级溯源日志器；nil 时静默（Log 自身 nil-safe）。
var provenanceLog *provenance.ProvenanceLogger

// SetProvenance 注入溯源日志器（bootstrap 装配）；传 nil 时溯源静默关闭。
func SetProvenance(l *provenance.ProvenanceLogger) { provenanceLog = l }

// ValidationResult is the structured output of patent_worker_validate.
type ValidationResult struct {
	Valid             bool     `json:"valid"`
	Degraded          bool     `json:"degraded"`
	MissingHardFields []string `json:"missing_hard_fields"`
	MissingSoftFields []string `json:"missing_soft_fields"`
	Message           string   `json:"message"`
}

// hardKeywords maps built-in Worker names to content patterns that must
// appear in the output for the contract to be considered satisfied.
// These are derived from each Worker's purpose and output format.
var hardKeywords = map[string][]string{
	"patent-technical-analyzer":      {"技术问题", "技术方案", "有益效果", "技术特征"},
	"patent-claim-drafter":           {"权利要求", "独立权利要求", "从属权利要求"},
	"patent-spec-drafter":            {"说明书", "技术领域", "背景技术", "具体实施方式"},
	"patent-oa-response-drafter":     {"审查意见", "答复", "权利要求", "修改"},
	"patent-novelty-analyzer":        {"新颖性", "对比文件", "技术特征"},
	"patent-inventiveness-analyzer":  {"创造性", "区别特征", "技术启示", "三步法"},
	"patent-infringement-analyzer":   {"侵权", "权利要求", "被控", "特征比对"},
	"patent-invalidation-analyzer":   {"无效", "权利要求", "对比文件", "创造性"},
	"patent-debate-simulator":        {"审查员", "代理人", "权利要求"},
	"patent-reexamination-drafter":   {"复审", "驳回决定", "权利要求"},
	"patent-search-planner":          {"检索", "关键词", "IPC"},
	"patent-search-executor":         {"对比文件", "检索结果", "专利"},
	"patent-search-commander":        {"对比文件", "检索", "遗漏"},
	"reviewer":                       {"审查", "权利要求", "说明书"},
	"quality_checker":                {"清晰性", "支持性", "保护范围"},
	"patent-claim-formality-checker": {"形式", "权利要求", "引用"},
	"patent-slop-cleaner":            {"套话", "清洗", "表达"},
	"legal-case-comparator":          {"案例", "法条", "比对"},
}

// softKeywords are advisory checks that do not fail the validation.
var softKeywords = map[string][]string{
	"patent-inventiveness-analyzer": {"预料不到的效果", "显而易见"},
	"patent-novelty-analyzer":       {"单独对比", "完全相同"},
	"quality_checker":               {"评分", "结论"},
}

// NewPatentWorkerValidateTool creates the patent_worker_validate tool.
func NewPatentWorkerValidateTool() *agentcore.Tool {
	return &agentcore.Tool{
		Name:        "patent_worker_validate",
		Description: "按内置 Worker 契约校验专利 Worker 的输出文本。硬性字段缺失会标记 degraded（不中断执行），返回缺失字段清单与判定结果。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"worker_name": map[string]any{"type": "string", "description": "Worker 名称（如 patent-inventiveness-analyzer）"},
				"output_text": map[string]any{"type": "string", "description": "待校验的 Worker 输出文本"},
			},
			"required": []string{"worker_name", "output_text"},
		},
		ReadOnly: true,
		Func:     handleValidate,
	}
}

func handleValidate(_ context.Context, args json.RawMessage) (any, error) {
	var p struct {
		WorkerName string `json:"worker_name"`
		OutputText string `json:"output_text"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		// 参数错误转为结构化失败响应返回调用方。
		return agentcore.NewFailureResult("参数解析失败", "Worker 校验参数格式错误"), nil
	}

	result := ValidateWorkerOutput(p.WorkerName, p.OutputText)

	// 溯源写入失败不阻断校验结果返回（Log nil-safe，fail-open）。
	_ = provenanceLog.Log(provenance.ProvenanceEvent{
		Kind:    provenance.KindContractValidate,
		Tool:    "patent_worker_validate",
		Details: fmt.Sprintf("worker=%s valid=%v degraded=%v", p.WorkerName, result.Valid, result.Degraded),
	})

	data, err := json.Marshal(result)
	if err != nil {
		return agentcore.NewFailureResult("序列化失败", err.Error()), nil
	}
	return string(data), nil
}

// ValidateWorkerOutput checks the output text against the known Worker contract.
func ValidateWorkerOutput(workerName, outputText string) ValidationResult {
	catalog := worker.NewCatalogFromDefault()
	def := catalog.Get(workerName)
	if def == nil {
		return ValidationResult{
			Valid:   false,
			Message: fmt.Sprintf("未知 worker %q", workerName),
		}
	}

	lower := strings.ToLower(outputText)
	missingHard := missingKeywords(lower, hardKeywords[def.Name])
	missingSoft := missingKeywords(lower, softKeywords[def.Name])

	valid := len(missingHard) == 0 && outputText != ""
	msg := fmt.Sprintf("patent_worker_validate(%s): ", def.Name)
	if valid {
		msg += "通过 ✅"
	} else {
		msg += "降级 ⚠️（硬性字段缺失）"
	}

	return ValidationResult{
		Valid:             valid,
		Degraded:          !valid,
		MissingHardFields: missingHard,
		MissingSoftFields: missingSoft,
		Message:           msg,
	}
}

// missingKeywords 返回 kws 中未在 text（已小写）里出现的词。
func missingKeywords(text string, kws []string) []string {
	var missing []string
	for _, kw := range kws {
		if !strings.Contains(text, strings.ToLower(kw)) {
			missing = append(missing, kw)
		}
	}
	return missing
}
