package bootstrap

// M69 补测：PlantaskBridge.Replan 是「反馈 → 新计划 → 任务同步 → 会话写回」的
// 装配闭环，此前 0% 覆盖。这里用内存 TaskStore 与可插拔 GenerateSteps 驱动，
// 锁定三条语义：已完成步骤保持 done、显式「重跑:」移除完成标记、生成失败时
// 会话不被改写。

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"sync"
	"testing"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/agentcore/plantask"
	"github.com/xujian519/mady/domains/reasoning"
)

// fakeTaskStore 是 plantask.TaskStore 的内存实现，只服务于本文件断言。
type fakeTaskStore struct {
	mu    sync.Mutex
	seq   int
	tasks map[string]*agentcore.Task
}

func newFakeTaskStore() *fakeTaskStore {
	return &fakeTaskStore{tasks: make(map[string]*agentcore.Task)}
}

func (s *fakeTaskStore) NextID(context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	return fmt.Sprintf("task-%d", s.seq), nil
}

func (s *fakeTaskStore) Create(_ context.Context, t *agentcore.Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *t
	s.tasks[t.ID] = &cp
	return nil
}

func (s *fakeTaskStore) Update(_ context.Context, t *agentcore.Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *t
	s.tasks[t.ID] = &cp
	return nil
}

func (s *fakeTaskStore) UpdateFunc(_ context.Context, id string, mutate func(*agentcore.Task) error) (*agentcore.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tasks[id]
	if !ok {
		return nil, fmt.Errorf("task %q not found", id)
	}
	if err := mutate(t); err != nil {
		return nil, err
	}
	return t, nil
}

func (s *fakeTaskStore) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.tasks)
}

func (s *fakeTaskStore) get(id string) *agentcore.Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tasks[id]
}

// newBridgeSession 构造一个含 2 个步骤、第 1 步已完成的会话。
func newBridgeSession() (*plantask.PlanTaskSession, plantask.StepSnapshot, plantask.StepSnapshot) {
	step1 := plantask.StepSnapshot{
		Order:       1,
		Strategy:    "search",
		Description: "检索现有技术",
		Hash:        plantask.StepHash(1, "search", "检索现有技术"),
	}
	step2 := plantask.StepSnapshot{
		Order:       2,
		Strategy:    "analysis",
		Description: "比对技术特征",
		Hash:        plantask.StepHash(2, "analysis", "比对技术特征"),
	}

	sess := plantask.NewSession("sess-1", "case-1", "patent")
	sess.Plan = plantask.PlanSnapshot{PlanID: "plan-1", Steps: []plantask.StepSnapshot{step1, step2}}
	sess.CompletedIDs = []string{step1.Hash}
	return sess, step1, step2
}

func TestPlantaskBridgeKeepsCompletedStep(t *testing.T) {
	store := newFakeTaskStore()
	bridge := NewPlantaskBridge(store)
	sess, step1, step2 := newBridgeSession()

	bridge.GenerateSteps = func(_ context.Context, _ *plantask.PlanTaskSession, _ string) ([]plantask.StepSnapshot, error) {
		return []plantask.StepSnapshot{step1, step2}, nil
	}

	skip, removed, err := bridge.Replan(context.Background(), sess, "继续下一步")
	if err != nil {
		t.Fatalf("Replan 失败：%v", err)
	}
	if !slices.Equal(skip, []string{step1.Hash}) {
		t.Fatalf("skip = %v，期望仅第 1 步保持 done", skip)
	}
	if len(removed) != 0 {
		t.Fatalf("removed = %v，期望为空", removed)
	}
	if !slices.Equal(sess.CompletedIDs, []string{step1.Hash}) {
		t.Fatalf("会话 CompletedIDs = %v，期望保留第 1 步", sess.CompletedIDs)
	}
	if len(sess.TaskIDs) != 2 || store.count() != 2 {
		t.Fatalf("任务同步异常：TaskIDs=%v 存储=%d", sess.TaskIDs, store.count())
	}
	// 顺序依赖：第 2 个任务被第 1 个阻塞。
	if task := store.get(sess.TaskIDs[1]); task == nil || !slices.Equal(task.BlockedBy, []string{sess.TaskIDs[0]}) {
		t.Fatalf("任务依赖未建立：%#v", task)
	}
}

func TestPlantaskBridgeExplicitRerun(t *testing.T) {
	store := newFakeTaskStore()
	bridge := NewPlantaskBridge(store)
	sess, step1, step2 := newBridgeSession()

	bridge.GenerateSteps = func(_ context.Context, _ *plantask.PlanTaskSession, _ string) ([]plantask.StepSnapshot, error) {
		return []plantask.StepSnapshot{step1, step2}, nil
	}

	skip, removed, err := bridge.Replan(context.Background(), sess, "重跑:检索")
	if err != nil {
		t.Fatalf("Replan 失败：%v", err)
	}
	if len(skip) != 0 {
		t.Fatalf("skip = %v，显式重跑时不应保留 done", skip)
	}
	if !slices.Equal(removed, []string{step1.Hash}) {
		t.Fatalf("removed = %v，期望移除第 1 步完成标记", removed)
	}
	if len(sess.CompletedIDs) != 0 {
		t.Fatalf("会话 CompletedIDs = %v，重跑后应为空", sess.CompletedIDs)
	}
}

func TestPlantaskBridgeGenerateErrorLeavesSessionUntouched(t *testing.T) {
	store := newFakeTaskStore()
	bridge := NewPlantaskBridge(store)
	sess, step1, step2 := newBridgeSession()
	before := sess.Plan

	bridge.GenerateSteps = func(_ context.Context, _ *plantask.PlanTaskSession, _ string) ([]plantask.StepSnapshot, error) {
		return nil, fmt.Errorf("planner unavailable")
	}

	if _, _, err := bridge.Replan(context.Background(), sess, "继续"); err == nil {
		t.Fatal("生成失败时 Replan 应返回错误")
	}
	if len(sess.Plan.Steps) != len(before.Steps) || sess.Plan.Steps[0].Hash != before.Steps[0].Hash {
		t.Fatalf("会话 Plan 被改写：%#v", sess.Plan)
	}
	if store.count() != 0 {
		t.Fatalf("生成失败时不应创建任务，实际 %d", store.count())
	}
	if !slices.Equal(sess.CompletedIDs, []string{step1.Hash}) {
		t.Fatalf("CompletedIDs 被改写：%v", sess.CompletedIDs)
	}
	_ = step2
}

func TestPlantaskBridgeWithoutGeneratorKeepsCurrentSteps(t *testing.T) {
	store := newFakeTaskStore()
	bridge := NewPlantaskBridge(store) // 既无 GenerateSteps 也无 runner
	sess, step1, _ := newBridgeSession()

	skip, removed, err := bridge.Replan(context.Background(), sess, "继续")
	if err != nil {
		t.Fatalf("Replan 失败：%v", err)
	}
	if !slices.Equal(skip, []string{step1.Hash}) || len(removed) != 0 {
		t.Fatalf("skip=%v removed=%v，无生成器时应保持既有步骤与完成标记", skip, removed)
	}
	if len(sess.Plan.Steps) != 2 {
		t.Fatalf("Plan 步骤数 = %d，期望保持 2", len(sess.Plan.Steps))
	}
	if len(sess.TaskIDs) != 2 {
		t.Fatalf("TaskIDs = %v，期望同步出 2 个任务", sess.TaskIDs)
	}
}

func TestToSnapshots(t *testing.T) {
	steps := []reasoning.PlanStep{
		{Order: 2, Strategy: reasoning.StrategyChain, Description: "构造推理链"},
		{Order: 1, Strategy: reasoning.StrategyReact, Description: "检索"},
	}
	got := toSnapshots(steps)
	if len(got) != 2 {
		t.Fatalf("快照数量 = %d，期望 2", len(got))
	}
	for i, s := range got {
		if s.Order != steps[i].Order || s.Strategy != string(steps[i].Strategy) || s.Description != steps[i].Description {
			t.Fatalf("第 %d 个快照字段漂移：%#v", i, s)
		}
		if want := plantask.StepHash(s.Order, s.Strategy, s.Description); s.Hash != want {
			t.Fatalf("第 %d 个快照哈希 = %q，期望 %q", i, s.Hash, want)
		}
	}
}

func TestMapKeys(t *testing.T) {
	got := mapKeys(map[string]bool{"b": true, "a": true, "c": false})
	sort.Strings(got)
	if !slices.Equal(got, []string{"a", "b", "c"}) {
		t.Fatalf("mapKeys = %v", got)
	}
	if got := mapKeys(nil); len(got) != 0 {
		t.Fatalf("nil map 应返回空切片，实际 %v", got)
	}
}
