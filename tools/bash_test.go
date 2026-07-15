package tools

import (
	"os/exec"
	"syscall"
	"testing"
	"time"
)

func TestKillProcessTree_InvalidPID(t *testing.T) {
	for _, pid := range []int{0, -1} {
		if err := killProcessTree(pid); err == nil {
			t.Fatalf("killProcessTree(%d) expected error, got nil", pid)
		}
	}
}

func TestKillProcessTree_NonExistentPID(t *testing.T) {
	// Use a high PID that is extremely unlikely to exist.
	// Both kills return ESRCH, which we treat as success (idempotent).
	if err := killProcessTree(999999); err != nil {
		t.Fatalf("expected nil for non-existent pid, got %v", err)
	}
}

func TestKillProcessTree_KillsProcessGroup(t *testing.T) {
	cmd := exec.Command("sh", "-c", "sleep 30")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start sleep: %v", err)
	}

	if err := killProcessTree(cmd.Process.Pid); err != nil {
		t.Fatalf("killProcessTree failed: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected process to be killed")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("process did not exit after kill")
	}
}
