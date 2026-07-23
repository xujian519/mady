package autoresearch

import (
	"log/slog"
	"sync"
	"time"
)

// Heartbeat 是长周期研究任务的心跳监控记录。
type Heartbeat struct {
	mu sync.Mutex `json:"-"`

	ContractID string        `json:"contract_id"`
	LastBeat   time.Time     `json:"last_beat"`
	Interval   time.Duration `json:"interval"`
	Timeout    time.Duration `json:"timeout"`
	BeatCount  int           `json:"beat_count"`
	IsStale    bool          `json:"is_stale"`
}

// NewHeartbeat 创建一个心跳监控器。
// interval 是期望的心跳间隔，timeout 是判定 stale 的阈值。
func NewHeartbeat(contractID string, interval, timeout time.Duration) *Heartbeat {
	return &Heartbeat{
		ContractID: contractID,
		LastBeat:   time.Now(),
		Interval:   interval,
		Timeout:    timeout,
	}
}

// Beat 记录一次心跳。
func (h *Heartbeat) Beat() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.LastBeat = time.Now()
	h.BeatCount++
	h.IsStale = false
	slog.Debug("autoresearch: heartbeat beat",
		"contract_id", h.ContractID, "beat_count", h.BeatCount)
}

// Check 检查心跳是否过期。
func (h *Heartbeat) Check() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.IsStale = time.Since(h.LastBeat) > h.Timeout
	if h.IsStale {
		slog.Warn("autoresearch: heartbeat stale",
			"contract_id", h.ContractID,
			"since_last_beat", time.Since(h.LastBeat).Round(time.Second),
			"timeout", h.Timeout)
	}
}

// SinceLastBeat 返回距离上次心跳的时间。
//
// 注意：此方法仅返回距上次心跳的时长，不判断是否过期。
// 过期判定请使用 Check() 后读取 IsStale 字段。
func (h *Heartbeat) SinceLastBeat() time.Duration {
	h.mu.Lock()
	defer h.mu.Unlock()
	return time.Since(h.LastBeat)
}
