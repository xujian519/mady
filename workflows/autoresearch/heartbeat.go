package autoresearch

import "time"

// Heartbeat 是长周期研究任务的心跳监控记录。
type Heartbeat struct {
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
	h.LastBeat = time.Now()
	h.BeatCount++
	h.IsStale = false
}

// Check 检查心跳是否过期。
func (h *Heartbeat) Check() {
	h.IsStale = time.Since(h.LastBeat) > h.Timeout
}

// SinceLastBeat 返回距离上次心跳的时间。
func (h *Heartbeat) SinceLastBeat() time.Duration {
	return time.Since(h.LastBeat)
}
