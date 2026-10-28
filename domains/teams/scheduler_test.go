package teams

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/xujian519/mady/agentcore"
)

type recordingRunner struct {
	mu          sync.Mutex
	order       []string
	byMember    map[string]int
	fail        bool
	failSubject string
	delay       time.Duration
	delays      map[string]time.Duration
}

func (r *recordingRunner) RunTask(_ context.Context, member Member, task *agentcore.Task) (string, error) {
	delay := r.delay
	if perTask, ok := r.delays[task.ID]; ok {
		delay = perTask
	}
	if delay > 0 {
		time.Sleep(delay)
	}
	if r.fail || (r.failSubject != "" && task.Subject == r.failSubject) {
		return "", errors.New("模拟失败")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.order = append(r.order, task.ID)
	if r.byMember == nil {
		r.byMember = make(map[string]int)
	}
	r.byMember[member.ID]++
	return "done:" + task.ID, nil
}

func (r *recordingRunner) completionIndex(taskID string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, id := range r.order {
		if id == taskID {
			return i
		}
	}
	return -1
}

func (r *recordingRunner) memberCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.byMember)
}

func TestSchedulerWaitsForDependencies(t *testing.T) {
	team := newTestTeam(t, "alpha", "beta")
	ctx := context.Background()
	first, err := team.AddTask(ctx, "A", "", agentcore.TaskPriorityNormal, nil)
	if err != nil {
		t.Fatalf("创建 A 失败：%v", err)
	}
	// B 优先级更高，会排在 A 前面；依赖关系必须仍然先执行 A。
	second, err := team.AddTask(ctx, "B", "", agentcore.TaskPriorityUrgent, []string{first.ID})
	if err != nil {
		t.Fatalf("创建 B 失败：%v", err)
	}
	third, err := team.AddTask(ctx, "C", "", agentcore.TaskPriorityNormal, nil)
	if err != nil {
		t.Fatalf("创建 C 失败：%v", err)
	}

	// A 故意更慢：若调度器忽略依赖，B 会在 A 之前完成，顺序断言即可发现。
	runner := &recordingRunner{delays: map[string]time.Duration{first.ID: 50 * time.Millisecond}}
	executed, err := team.RunUntilIdle(ctx, runner)
	if err != nil {
		t.Fatalf("调度失败：%v", err)
	}
	if executed != 3 {
		t.Fatalf("执行任务数 = %d, want 3", executed)
	}
	if runner.completionIndex(first.ID) < 0 || runner.completionIndex(second.ID) < 0 ||
		runner.completionIndex(third.ID) < 0 {
		t.Fatalf("任务未全部完成：%v", runner.order)
	}
	if runner.completionIndex(first.ID) > runner.completionIndex(second.ID) {
		t.Fatalf("依赖顺序错误：A=%d B=%d", runner.completionIndex(first.ID), runner.completionIndex(second.ID))
	}

	task, err := team.tasks.Get(ctx, second.ID)
	if err != nil {
		t.Fatalf("读取任务失败：%v", err)
	}
	if task.Status != agentcore.TaskCompleted || task.Metadata[resultKey] != "done:"+second.ID {
		t.Fatalf("任务结果未登记：%+v", task)
	}
	for _, member := range team.Members() {
		if member.Status != MemberIdle {
			t.Fatalf("成员 %s 未回到 idle：%s", member.ID, member.Status)
		}
	}
}

func TestThreeMembersCollaborate(t *testing.T) {
	team := newTestTeam(t, "alpha", "beta", "gamma")
	ctx := context.Background()
	ids := make([]string, 0, 6)
	for _, subject := range []string{"t1", "t2", "t3"} {
		task, err := team.AddTask(ctx, subject, "", agentcore.TaskPriorityNormal, nil)
		if err != nil {
			t.Fatalf("创建 %s 失败：%v", subject, err)
		}
		ids = append(ids, task.ID)
	}
	for i := range 3 {
		task, err := team.AddTask(ctx, "dep", "", agentcore.TaskPriorityNormal, []string{ids[i]})
		if err != nil {
			t.Fatalf("创建依赖任务失败：%v", err)
		}
		ids = append(ids, task.ID)
	}

	runner := &recordingRunner{delay: 5 * time.Millisecond}
	executed, err := team.RunUntilIdle(ctx, runner)
	if err != nil {
		t.Fatalf("调度失败：%v", err)
	}
	if executed != 6 {
		t.Fatalf("执行任务数 = %d, want 6", executed)
	}
	if runner.memberCount() != 3 {
		t.Fatalf("应有三名成员参与，实际 %d：%v", runner.memberCount(), runner.byMember)
	}
	for i := range 3 {
		if runner.completionIndex(ids[i]) > runner.completionIndex(ids[i+3]) {
			t.Fatalf("依赖顺序错误：%s 应在 %s 之前", ids[i], ids[i+3])
		}
	}
	view, err := team.View(ctx)
	if err != nil || view.Tasks.Completed != 6 {
		t.Fatalf("状态视图完成数 = %d, err = %v", view.Tasks.Completed, err)
	}
}

func TestFailTaskRetriesUntilCap(t *testing.T) {
	team := newTestTeam(t, "alpha")
	ctx := context.Background()
	task, err := team.AddTask(ctx, "会失败", "", agentcore.TaskPriorityNormal, nil)
	if err != nil {
		t.Fatalf("创建任务失败：%v", err)
	}
	runner := &recordingRunner{fail: true}
	executed, err := team.RunUntilIdle(ctx, runner)
	if err != nil {
		t.Fatalf("调度失败：%v", err)
	}
	if executed != defaultMaxAttempts {
		t.Fatalf("尝试次数 = %d, want %d", executed, defaultMaxAttempts)
	}
	stored, err := team.tasks.Get(ctx, task.ID)
	if err != nil {
		t.Fatalf("读取任务失败：%v", err)
	}
	if stored.Status != agentcore.TaskPending || stored.Owner != "" {
		t.Fatalf("失败任务应退回 pending 且清空 owner：%+v", stored)
	}
	if attemptsOf(stored) != defaultMaxAttempts {
		t.Fatalf("尝试计数 = %d", attemptsOf(stored))
	}
	more, err := team.RunUntilIdle(ctx, runner)
	if err != nil || more != 0 {
		t.Fatalf("达到上限后不应再认领：%d, err = %v", more, err)
	}
}

func TestRunSurvivesTaskFailures(t *testing.T) {
	team := newTestTeam(t, "alpha")
	ctx, cancel := context.WithCancel(context.Background())
	runner := &recordingRunner{failSubject: "会失败"}
	returned := make(chan error, 1)
	go func() { returned <- team.Run(ctx, runner) }()

	if _, err := team.AddTask(ctx, "会失败", "", agentcore.TaskPriorityNormal, nil); err != nil {
		t.Fatalf("创建失败任务失败：%v", err)
	}
	good, err := team.AddTask(ctx, "会成功", "", agentcore.TaskPriorityNormal, nil)
	if err != nil {
		t.Fatalf("创建成功任务失败：%v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		task, err := team.tasks.Get(ctx, good.ID)
		if err == nil && task.Status == agentcore.TaskCompleted {
			break
		}
		select {
		case err := <-returned:
			t.Fatalf("调度器在任务失败后退出：%v", err)
		case <-time.After(5 * time.Millisecond):
		}
	}
	task, err := team.tasks.Get(ctx, good.ID)
	if err != nil || task.Status != agentcore.TaskCompleted {
		t.Fatalf("成功任务未被调度：status=%v err=%v", task.Status, err)
	}

	cancel()
	if err := <-returned; !errors.Is(err, context.Canceled) {
		t.Fatalf("Run 应因 ctx 取消返回，得到 %v", err)
	}
}

func TestClaimNextIsAtomic(t *testing.T) {
	team := newTestTeam(t, "alpha", "beta")
	ctx := context.Background()
	task, err := team.AddTask(ctx, "only", "", agentcore.TaskPriorityNormal, nil)
	if err != nil {
		t.Fatalf("创建任务失败：%v", err)
	}

	var wg sync.WaitGroup
	claimed := make([]*agentcore.Task, 2)
	for i, member := range []string{"alpha", "beta"} {
		index := i
		id := member
		wg.Go(func() {
			claimed[index], _ = team.ClaimNext(ctx, id)
		})
	}
	wg.Wait()
	winners := 0
	for _, got := range claimed {
		if got != nil {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("同一任务被认领 %d 次：%+v", winners, claimed)
	}
	if claimed[0] != nil && claimed[0].ID != task.ID {
		t.Fatalf("认领到错误任务：%+v", claimed[0])
	}
	for _, member := range []string{"alpha", "beta"} {
		again, err := team.ClaimNext(ctx, member)
		if err != nil || again != nil {
			t.Fatalf("已认领的任务不应再次被认领：member=%s task=%+v err=%v", member, again, err)
		}
	}
	if err := team.CompleteTask(ctx, "nobody", task.ID, "x"); err == nil {
		t.Fatal("非认领者不应能完成任务")
	}
}
