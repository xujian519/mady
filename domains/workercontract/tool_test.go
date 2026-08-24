package workercontract

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/xujian519/mady/agentcore"
)

func TestValidateWorkerOutput_Pass(t *testing.T) {
	res := ValidateWorkerOutput("patent-inventiveness-analyzer", "本发明具有创造性。区别特征为X，对比文件D1没有技术启示，适用三步法分析。")
	if !res.Valid {
		t.Errorf("expected valid, got %v", res)
	}
	if res.Degraded {
		t.Error("expected not degraded")
	}
	if len(res.MissingHardFields) > 0 {
		t.Errorf("unexpected missing hard fields: %v", res.MissingHardFields)
	}
}

func TestValidateWorkerOutput_Missing(t *testing.T) {
	res := ValidateWorkerOutput("patent-inventiveness-analyzer", "这是一个普通输出。")
	if res.Valid {
		t.Error("expected invalid")
	}
	if !res.Degraded {
		t.Error("expected degraded")
	}
	if len(res.MissingHardFields) == 0 {
		t.Error("expected missing hard fields")
	}
}

func TestValidateWorkerOutput_Unknown(t *testing.T) {
	res := ValidateWorkerOutput("unknown-worker", "任何内容")
	if res.Valid {
		t.Error("expected invalid for unknown worker")
	}
	if !strings.Contains(res.Message, "未知") {
		t.Errorf("expected unknown message, got %q", res.Message)
	}
}

func TestHandleValidate(t *testing.T) {
	args := json.RawMessage(`{"worker_name":"patent-novelty-analyzer","output_text":"对比文件D1公开了全部技术特征，因此权利要求1不具备新颖性。"}`)
	res, err := handleValidate(nil, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hr, ok := res.(agentcore.HandoffResult); ok && !hr.Success {
		t.Errorf("expected success, got %v", hr)
	}
}
