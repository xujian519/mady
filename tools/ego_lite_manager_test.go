package tools

import (
	"context"
	"os/exec"
	"testing"
	"time"
)

func TestEgoLiteManagerCloseIdempotent(t *testing.T) {
	mgr := &EgoLiteManager{
		taskName: "test",
		pending:  make(map[string]chan egoLiteJSONResponse),
	}
	ctx, cancel := context.WithCancel(context.Background())
	mgr.ctx = ctx
	mgr.cancel = cancel
	// 第一次 Close
	if err := mgr.Close(); err != nil {
		t.Fatalf("first close failed: %v", err)
	}
	// 第二次 Close（幂等）
	if err := mgr.Close(); err != nil {
		t.Fatalf("second close failed: %v", err)
	}
}

func TestEgoLiteManagerErrorOnClosed(t *testing.T) {
	mgr := &EgoLiteManager{
		taskName: "test",
		pending:  make(map[string]chan egoLiteJSONResponse),
	}
	ctx, cancel := context.WithCancel(context.Background())
	mgr.ctx = ctx
	mgr.cancel = cancel
	mgr.Close()

	_, err := mgr.Send(context.Background(), "ping", nil)
	if err == nil {
		t.Error("expected error from closed manager")
	}
}

func TestEgoLiteAvailable(t *testing.T) {
	_, err := exec.LookPath("ego-browser")
	expect := err == nil
	got := egoLiteAvailable()
	if got != expect {
		t.Errorf("egoLiteAvailable = %v, LookPath error = %v", got, err)
	}
}

func TestEgoLiteManagerSendTimeout(t *testing.T) {
	mgr := &EgoLiteManager{
		taskName: "test",
		pending:  make(map[string]chan egoLiteJSONResponse),
	}
	ctx, cancel := context.WithCancel(context.Background())
	mgr.ctx = ctx
	mgr.cancel = cancel
	defer mgr.Close()

	deadCtx, deadCancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer deadCancel()
	time.Sleep(1 * time.Millisecond)
	_, err := mgr.Send(deadCtx, "ping", nil)
	if err == nil {
		t.Error("expected timeout error from expired context")
	}
}

func TestNewEgoLiteManagerDefaultTaskName(t *testing.T) {
	if !egoLiteAvailable() {
		t.Skip("ego-browser not installed")
	}
	mgr, err := NewEgoLiteManager("")
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()
	if mgr.taskName != "mady-agent" {
		t.Errorf("default taskName = %q, want %q", mgr.taskName, "mady-agent")
	}
}
