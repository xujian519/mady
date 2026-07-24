package server

import (
	"expvar"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// MetricsRecorder 定义服务器监控指标接口。
// 生产环境可替换为 Prometheus 等实现。
type MetricsRecorder interface {
	// RequestDuration 记录 HTTP 请求持续时间（秒）。
	RequestDuration(method, path string, statusCode int, durationSeconds float64)
	// ActiveConnections 记录当前活跃连接数。
	ActiveConnections(n int)
}

// NopMetricsRecorder 是 MetricsRecorder 的空实现，所有方法均为 no-op。
type NopMetricsRecorder struct{}

func (NopMetricsRecorder) RequestDuration(string, string, int, float64) {}
func (NopMetricsRecorder) ActiveConnections(int)                        {}

// ExpvarMetricsRecorder 是基于 go 标准库 expvar 包的 MetricsRecorder 实现。
// 通过 expvar 自动注册的 /debug/vars 端点暴露指标，零外部依赖。
type ExpvarMetricsRecorder struct {
	mu             sync.Mutex
	requestCount   *expvar.Int
	requestErrors  *expvar.Int
	activeConns    *expvar.Int
	latencyBuckets []*latencyBucket
}

// latencyBucket 是请求延迟直方图的一个桶。
type latencyBucket struct {
	upperBound float64 // 秒
	count      *expvar.Int
}

// NewExpvarMetricsRecorder 创建一个基于 expvar 的指标记录器。
// 指标通过 /debug/vars 暴露。
func NewExpvarMetricsRecorder() *ExpvarMetricsRecorder {
	r := &ExpvarMetricsRecorder{
		requestCount:  expvar.NewInt("http_requests_total"),
		requestErrors: expvar.NewInt("http_request_errors_total"),
		activeConns:   expvar.NewInt("http_active_connections"),
		latencyBuckets: []*latencyBucket{
			{upperBound: 0.05, count: expvar.NewInt("http_request_duration_seconds_bucket_0_05")},
			{upperBound: 0.1, count: expvar.NewInt("http_request_duration_seconds_bucket_0_1")},
			{upperBound: 0.25, count: expvar.NewInt("http_request_duration_seconds_bucket_0_25")},
			{upperBound: 0.5, count: expvar.NewInt("http_request_duration_seconds_bucket_0_5")},
			{upperBound: 1.0, count: expvar.NewInt("http_request_duration_seconds_bucket_1")},
			{upperBound: 2.5, count: expvar.NewInt("http_request_duration_seconds_bucket_2_5")},
			{upperBound: 5.0, count: expvar.NewInt("http_request_duration_seconds_bucket_5")},
			{upperBound: 10.0, count: expvar.NewInt("http_request_duration_seconds_bucket_10")},
			{upperBound: 30.0, count: expvar.NewInt("http_request_duration_seconds_bucket_30")},
		},
	}

	// 注册一个 expvar.Func 以动态报告当前活跃连接数
	expvar.Publish("http_active_connections", expvar.Func(func() any {
		return r.activeConns.Value()
	}))

	// 注册一个 expvar.Func 以动态报告总的请求延迟桶计数
	expvar.Publish("http_latency_buckets", expvar.Func(func() any {
		r.mu.Lock()
		defer r.mu.Unlock()
		buckets := make(map[string]int64, len(r.latencyBuckets))
		for _, b := range r.latencyBuckets {
			key := "le_" + strconv.FormatFloat(b.upperBound, 'f', -1, 64)
			buckets[key] = b.count.Value()
		}
		return buckets
	}))

	return r
}

// RequestDuration 实现 MetricsRecorder 接口。
// 递增请求计数器，记录到相应的延迟桶。
func (e *ExpvarMetricsRecorder) RequestDuration(method, path string, statusCode int, durationSeconds float64) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.requestCount.Add(1)
	if statusCode >= 500 {
		e.requestErrors.Add(1)
	}

	for _, b := range e.latencyBuckets {
		if durationSeconds <= b.upperBound {
			b.count.Add(1)
		}
	}
}

// ActiveConnections 实现 MetricsRecorder 接口。
func (e *ExpvarMetricsRecorder) ActiveConnections(n int) {
	e.activeConns.Set(int64(n))
}

// HandleMetrics 注册一个简单的 /metrics 端点，
// 返回 Prometheus 风格的纯文本指标（可使用 expvar 数据源）。
func (e *ExpvarMetricsRecorder) HandleMetrics(mux *http.ServeMux) {
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)

		now := time.Now().Unix()
		w.Write([]byte("# HELP http_requests_total Total number of HTTP requests\n"))
		w.Write([]byte("# TYPE http_requests_total counter\n"))
		w.Write([]byte("http_requests_total " + strconv.FormatInt(e.requestCount.Value(), 10) + " " + strconv.FormatInt(now, 10) + "\n"))

		w.Write([]byte("# HELP http_request_errors_total Total number of HTTP 5xx errors\n"))
		w.Write([]byte("# TYPE http_request_errors_total counter\n"))
		w.Write([]byte("http_request_errors_total " + strconv.FormatInt(e.requestErrors.Value(), 10) + " " + strconv.FormatInt(now, 10) + "\n"))

		w.Write([]byte("# HELP http_active_connections Current number of active connections\n"))
		w.Write([]byte("# TYPE http_active_connections gauge\n"))
		w.Write([]byte("http_active_connections " + strconv.FormatInt(e.activeConns.Value(), 10) + "\n"))
	})
}
