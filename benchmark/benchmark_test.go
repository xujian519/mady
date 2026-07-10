package benchmark

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/graph"
	"github.com/xujian519/mady/workflows/patent"
)

// ──────────────────────────────────────────────
// Agent 创建延迟
// ──────────────────────────────────────────────

func BenchmarkAgentCreation(b *testing.B) {
	cfg := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:  "bench-agent",
			Model: "stub",
		},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		agent := agentcore.New(cfg)
		agent.Close()
	}
}

// ──────────────────────────────────────────────
// Pregel 图编译延迟
// ──────────────────────────────────────────────

func BenchmarkPregelCompile(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, err := patent.BuildNoveltyGraphWithRules()
		if err != nil {
			b.Fatalf("BuildNoveltyGraphWithRules: %v", err)
		}
	}
}

// ──────────────────────────────────────────────
// Pregel 图执行延迟
// ──────────────────────────────────────────────

func BenchmarkPregelExecute(b *testing.B) {
	compiled, err := patent.BuildNoveltyGraphWithRules()
	if err != nil {
		b.Fatalf("BuildNoveltyGraphWithRules: %v", err)
	}

	input := "一种智能节水灌溉装置，其特征在于包括土壤湿度传感器、中央控制器和电磁阀；土壤湿度传感器采集土壤水分数据，中央控制器根据预设阈值判断是否需要灌溉，电磁阀根据控制信号开启或关闭。"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		state, runErr := compiled.Run(context.Background(), graph.PregelState{
			patent.StateInput: input,
		})
		if runErr != nil {
			b.Fatalf("Run: %v", runErr)
		}
		if state.GetString(patent.StateOutput) == "" {
			b.Fatal("output is empty")
		}
	}
}

// ──────────────────────────────────────────────
// HandoffContext 序列化
// ──────────────────────────────────────────────

func BenchmarkHandoffContextSerialization(b *testing.B) {
	ctx := agentcore.HandoffContext{
		FromAgent:  "test-agent",
		ToAgent:    "target-agent",
		UserIntent: "测试意图摘要内容",
		ExtractedEntities: map[string]string{
			"patent_no": "CN109690000A",
			"case_id":   "AB2024-0001",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		json.Marshal(ctx)
	}
}

// ──────────────────────────────────────────────
// Manifest 解析
// ──────────────────────────────────────────────

func BenchmarkManifestScan(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _, err := agentcore.ScanManifests("../manifests")
		if err != nil {
			b.Fatalf("ScanManifests: %v", err)
		}
	}
}
