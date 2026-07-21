package main

// tui_deferred.go 管理 TUI 启动时的后台延迟初始化。
//
// setupFrameworkContext()（framework.go）当前全同步执行所有装配。
// 本文件将其中的非关键步骤抽取为 deferredTask，在 app.Start() 之后
// 以 goroutine 方式逐步完成，缩短首帧前同步路径。
//
// 使用方式：
//
//	fc := setupFrameworkContext(ctx, "tui")
//	// ... 主题 / ChatApp / app.Start() ...
//	fc.Deferred.StartAll(ctx) // 在后台 goroutine 中执行
//
// 延迟任务全部标记为"失败不阻塞启动"，但失败原因通过以下途径可见：
//   - fc.Deferred.Errors() 列出所有失败任务
//   - 启动时系统消息提示降级
//   - JudgmentView 状态栏 degraded 标签

import (
	"context"
	"fmt"
	"log"
	"sync"
)

// deferredTask 描述一个可在后台执行的初始化任务。
type deferredTask struct {
	Name string       // 任务名称，用于日志和错误报告
	Fn   func() error // 执行函数，返回 error 表示失败
}

// DeferredInit 管理一组后台延迟初始化任务。
//
// 线程安全：Add / StartAll / HasStarted / Errors 可从任意 goroutine 调用。
type DeferredInit struct {
	mu      sync.Mutex
	tasks   []deferredTask
	started bool
	done    bool
	errors  map[string]string // task name → error message
}

// newDeferredInit 创建一个空的 DeferredInit。
func newDeferredInit() *DeferredInit {
	return &DeferredInit{
		errors: make(map[string]string),
	}
}

// Add 注册一个延迟任务。若任务集已启动则立即执行（非阻塞）。
func (d *DeferredInit) Add(name string, fn func() error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.started {
		// 已启动：立即在调用者 goroutine 中执行。
		// 调用方通常是 setupFrameworkContext 自己，此时 StartAll 尚未被调用；
		// 如果被误用（在 StartAll 之后 Add），仍然安全执行。
		d.mu.Unlock()
		err := fn()
		d.mu.Lock()
		if err != nil {
			d.errors[name] = err.Error()
		}
		return
	}

	d.tasks = append(d.tasks, deferredTask{Name: name, Fn: fn})
}

// StartAll 在后台 goroutine 中逐一执行所有已注册的延迟任务。
// 已启动任务的后续 Add 会立即执行而非排队。
// 所有任务执行完毕后将结果记录到 d.errors 中。
func (d *DeferredInit) StartAll(ctx context.Context) {
	d.mu.Lock()
	if d.started {
		d.mu.Unlock()
		return
	}
	d.started = true
	tasks := make([]deferredTask, len(d.tasks))
	copy(tasks, d.tasks)
	d.tasks = nil // 释放引用，后续 Add 直接执行
	d.mu.Unlock()

	go func() {
		for _, t := range tasks {
			select {
			case <-ctx.Done():
				d.mu.Lock()
				d.errors[t.Name] = "canceled: " + ctx.Err().Error()
				d.done = true
				d.mu.Unlock()
				return
			default:
			}

			if err := t.Fn(); err != nil {
				errMsg := err.Error()
				log.Printf("[mady] deferred init %s failed: %v", t.Name, err)
				d.mu.Lock()
				d.errors[t.Name] = errMsg
				d.mu.Unlock()
			} else {
				log.Printf("[mady] deferred init %s completed", t.Name)
			}
		}

		d.mu.Lock()
		d.done = true
		d.mu.Unlock()
	}()
}

// HasStarted 返回是否已调用 StartAll。
func (d *DeferredInit) HasStarted() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.started
}

// IsDone 返回所有延迟任务是否已执行完毕。
func (d *DeferredInit) IsDone() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.done
}

// Errors 返回失败任务的名称 → 错误信息映射。空 map 表示全部成功。
func (d *DeferredInit) Errors() map[string]string {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make(map[string]string, len(d.errors))
	for k, v := range d.errors {
		out[k] = v
	}
	return out
}

// HasErrors 返回是否存在任何失败任务。
func (d *DeferredInit) HasErrors() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.errors) > 0
}

// ErrorSummary 返回人类可读的错误摘要，无失败时返回空字符串。
func (d *DeferredInit) ErrorSummary() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.errors) == 0 {
		return ""
	}
	summary := fmt.Sprintf("%d 个后台初始化任务失败:", len(d.errors))
	for name, err := range d.errors {
		summary += fmt.Sprintf("\n  · %s: %s", name, err)
	}
	return summary
}
