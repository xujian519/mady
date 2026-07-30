package worker

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// =============================================================================
// ExecutionRecord
// =============================================================================

// ExecutionRecord 记录一次 Worker 执行的完整信息。
type ExecutionRecord struct {
	WorkerName  string        `json:"worker_name"`
	Tier        WorkerTier    `json:"tier"`
	Mode        string        `json:"mode"` // "llm" | "graph" | "tool"
	StartedAt   time.Time     `json:"started_at"`
	Duration    time.Duration `json:"duration_ns"`
	InputValid  bool          `json:"input_valid"`
	OutputValid bool          `json:"output_valid"`
	Success     bool          `json:"success"`
	Degraded    bool          `json:"degraded"`
	ErrorMsg    string        `json:"error_msg,omitempty"`
}

// WorkerStats 按 Worker 聚合的执行统计。
type WorkerStats struct {
	WorkerName       string  `json:"worker_name"`
	Tier             string  `json:"tier"`
	TotalExecutions  int     `json:"total_executions"`
	SuccessCount     int     `json:"success_count"`
	FailureCount     int     `json:"failure_count"`
	DegradedCount    int     `json:"degraded_count"`
	InputViolations  int     `json:"input_violations"`
	OutputViolations int     `json:"output_violations"`
	AvgDurationMs    float64 `json:"avg_duration_ms"`
	P99DurationMs    float64 `json:"p99_duration_ms"`
	LastExecuted     string  `json:"last_executed,omitempty"`
}

// =============================================================================
// Monitor
// =============================================================================

// Monitor 收集 Worker 执行记录，提供聚合统计。
// 通过 Executor 的监控回调接入（参见 executor.go 的 WithMonitor 选项）。
//
// 使用方式：
//
//	monitor := worker.NewMonitor()
//	exec := worker.NewLLMExecutor(def, llmFn)
//	// 通过闭包在 Executor 的 ToPregelNode 中调用 monitor.Record()
//
//	stats := monitor.Stats()
//	for _, s := range stats {
//	    fmt.Printf("Worker %s: %d 次执行, %.0f%% 成功率\n", s.WorkerName,
//	        s.TotalExecutions, float64(s.SuccessCount)/float64(s.TotalExecutions)*100)
//	}
type Monitor struct {
	mu       sync.Mutex
	records  []ExecutionRecord
	onRecord func(ExecutionRecord) // 可选回调（如注入 Tracing span）
}

// NewMonitor 创建空的 Monitor。
func NewMonitor() *Monitor {
	return &Monitor{}
}

// WithCallback 设置每次记录时的回调函数（用于 Tracing 集成）。
func (m *Monitor) WithCallback(fn func(ExecutionRecord)) *Monitor {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onRecord = fn
	return m
}

// Record 记录一次执行。线程安全。
func (m *Monitor) Record(rec ExecutionRecord) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.onRecord != nil {
		m.onRecord(rec)
	}
	m.records = append(m.records, rec)
}

// Records 返回所有执行记录的快照（按时间排序）。
func (m *Monitor) Records() []ExecutionRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]ExecutionRecord, len(m.records))
	copy(out, m.records)
	return out
}

// Clear 清空所有记录。
func (m *Monitor) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records = nil
}

// Stats 按 Worker 名称聚合统计。
func (m *Monitor) Stats() []WorkerStats {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.records) == 0 {
		return nil
	}

	// 按 Worker 名分组
	type agg struct {
		total, success, fail, degraded, inputViol, outputViol int
		totalDuration                                         time.Duration
		durations                                             []time.Duration
		lastExec                                              time.Time
		tier                                                  WorkerTier
	}
	groups := make(map[string]*agg)
	var order []string

	for _, rec := range m.records {
		a, ok := groups[rec.WorkerName]
		if !ok {
			a = &agg{tier: rec.Tier}
			groups[rec.WorkerName] = a
			order = append(order, rec.WorkerName)
		}
		a.total++
		a.totalDuration += rec.Duration
		a.durations = append(a.durations, rec.Duration)
		if rec.Success {
			a.success++
		} else {
			a.fail++
		}
		if rec.Degraded {
			a.degraded++
		}
		if !rec.InputValid {
			a.inputViol++
		}
		if !rec.OutputValid {
			a.outputViol++
		}
		if rec.StartedAt.After(a.lastExec) {
			a.lastExec = rec.StartedAt
		}
	}

	stats := make([]WorkerStats, 0, len(groups))
	for _, name := range order {
		a := groups[name]
		avgMs := float64(0)
		p99Ms := float64(0)
		if a.total > 0 {
			avgMs = float64(a.totalDuration.Microseconds()) / float64(a.total) / 1000.0
			p99Ms = percentile(a.durations, 0.99)
		}
		lastExec := ""
		if !a.lastExec.IsZero() {
			lastExec = a.lastExec.Format(time.RFC3339)
		}
		stats = append(stats, WorkerStats{
			WorkerName:       name,
			Tier:             string(a.tier),
			TotalExecutions:  a.total,
			SuccessCount:     a.success,
			FailureCount:     a.fail,
			DegradedCount:    a.degraded,
			InputViolations:  a.inputViol,
			OutputViolations: a.outputViol,
			AvgDurationMs:    avgMs,
			P99DurationMs:    p99Ms,
			LastExecuted:     lastExec,
		})
	}

	return stats
}

// Summary 返回人类可读的统计摘要。
func (m *Monitor) Summary() string {
	stats := m.Stats()
	if len(stats) == 0 {
		return "Monitor: 无执行记录"
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Worker Monitor 统计 (%d 个 Worker):\n", len(stats))
	totalExec := 0
	totalFail := 0
	totalDegraded := 0
	for _, s := range stats {
		totalExec += s.TotalExecutions
		totalFail += s.FailureCount
		totalDegraded += s.DegradedCount
		fmt.Fprintf(&sb, "  [%s] %s: %d 次 (成功 %d, 失败 %d, 降级 %d, 平均 %.0fms)\n",
			s.Tier, s.WorkerName, s.TotalExecutions, s.SuccessCount,
			s.FailureCount, s.DegradedCount, s.AvgDurationMs)
	}
	fmt.Fprintf(&sb, "总计: %d 次执行, %d 次失败, %d 次降级\n", totalExec, totalFail, totalDegraded)
	return sb.String()
}

// =============================================================================
// 辅助
// =============================================================================

// percentile 计算排序后的 P99。
func percentile(durations []time.Duration, p float64) float64 {
	if len(durations) == 0 {
		return 0
	}
	sorted := make([]time.Duration, len(durations))
	copy(sorted, durations)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	idx := int(float64(len(sorted)-1) * p)
	if idx < 0 {
		idx = 0
	}
	return float64(sorted[idx].Microseconds()) / 1000.0
}
