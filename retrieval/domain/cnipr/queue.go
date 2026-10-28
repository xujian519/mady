package cnipr

import "sync"

// SerialQueue 把共享同一浏览器任务空间的工作串行化：同一时刻只有一个脚本在跑，
// 避免两个调用同时操作同一个标签页。互斥锁在 Go 运行时里近似 FIFO 唤醒。
type SerialQueue struct {
	mu sync.Mutex
}

// Run 在前一个任务结束后执行 fn；fn 返回后释放占用。
func (q *SerialQueue) Run(fn func() error) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	return fn()
}
