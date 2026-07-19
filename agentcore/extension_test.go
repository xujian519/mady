package agentcore

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// stubExt 是可编程的测试扩展：记录 Init/Dispose 调用，可控返回错误。
type stubExt struct {
	name      string
	initErr   error
	disposeMu sync.Mutex
	disposed  bool
}

func (s *stubExt) Name() string { return s.name }
func (s *stubExt) Init(ctx context.Context, agent *Agent) error {
	return s.initErr
}
func (s *stubExt) Dispose() error {
	s.disposeMu.Lock()
	s.disposed = true
	s.disposeMu.Unlock()
	return nil
}

// TestExtensionRegistry_RegisterFailureDisposesInitialized 验证 H6：
// 第 N 个扩展 Init 失败时，前面已 Init 成功的扩展被逆序 Dispose，
// 不泄漏资源。
func TestExtensionRegistry_RegisterFailureDisposesInitialized(t *testing.T) {
	ok1 := &stubExt{name: "ok1"}
	ok2 := &stubExt{name: "ok2"}
	fail := &stubExt{name: "fail", initErr: errors.New("init 失败")}

	reg := NewExtensionRegistry()
	agent := New(StubConfig(&stubProvider{}))

	err := reg.Register(context.Background(), agent, ok1, ok2, fail)
	if err == nil {
		t.Fatalf("期望 Register 返回错误")
	}

	if !ok1.disposed {
		t.Errorf("ok1 应被 Dispose（资源泄漏）")
	}
	if !ok2.disposed {
		t.Errorf("ok2 应被 Dispose（资源泄漏）")
	}
	// fail 未 Init 成功，不应 Dispose（但即便 Dispose 也无副作用，这里不强制）
}

// TestExtensionRegistry_RegisterSuccess 验证正常路径不 Dispose。
func TestExtensionRegistry_RegisterSuccess(t *testing.T) {
	ok1 := &stubExt{name: "ok1"}
	ok2 := &stubExt{name: "ok2"}

	reg := NewExtensionRegistry()
	agent := New(StubConfig(&stubProvider{}))

	if err := reg.Register(context.Background(), agent, ok1, ok2); err != nil {
		t.Fatalf("不期望错误: %v", err)
	}
	if ok1.disposed || ok2.disposed {
		t.Errorf("成功路径不应 Dispose")
	}
	// 显式 Dispose 释放
	_ = reg.Dispose()
	if !ok1.disposed || !ok2.disposed {
		t.Errorf("显式 Dispose 后应已释放")
	}
}
