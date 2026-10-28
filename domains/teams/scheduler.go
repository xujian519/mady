package teams

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/xujian519/mady/agentcore"
)

const (
	attemptsKey        = "team_attempts"
	resultKey          = "team_result"
	defaultMaxAttempts = 3
)

var errNotClaimable = errors.New("任务已被其他成员认领")

// Runner 执行一个被认领的团队任务；实现方须能并发处理不同成员的任务。
type Runner interface {
	RunTask(ctx context.Context, member Member, task *agentcore.Task) (string, error)
}

// AddTask 新建任务并写入依赖关系；blockedBy 中的任务必须已存在。
func (t *Team) AddTask(ctx context.Context, subject, description string,
	priority agentcore.TaskPriority, blockedBy []string) (*agentcore.Task, error) {
	if subject == "" {
		return nil, fmt.Errorf("任务标题不能为空")
	}
	for _, dependency := range blockedBy {
		if _, err := t.tasks.Get(ctx, dependency); err != nil {
			return nil, fmt.Errorf("依赖任务 %s 不存在：%w", dependency, err)
		}
	}
	id, err := t.tasks.NextID(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	task := &agentcore.Task{
		ID: id, Subject: subject, Description: description,
		Status: agentcore.TaskPending, Priority: priority,
		BlockedBy: append([]string(nil), blockedBy...),
		CreatedAt: now, UpdatedAt: now,
	}
	if err := t.tasks.Create(ctx, task); err != nil {
		return nil, fmt.Errorf("创建任务失败：%w", err)
	}
	for _, dependency := range blockedBy {
		if _, err := t.tasks.UpdateFunc(ctx, dependency, func(dep *agentcore.Task) error {
			dep.Blocks = appendUnique(dep.Blocks, id)
			dep.UpdatedAt = now
			return nil
		}); err != nil {
			return nil, fmt.Errorf("登记依赖关系失败：%w", err)
		}
	}
	t.emit("task_added", map[string]any{"task": id, "blocked_by": blockedBy})
	t.signal()
	return task.Clone(), nil
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

// ClaimNext 原子地认领下一个可执行任务；没有可执行任务时返回 (nil, nil)。
func (t *Team) ClaimNext(ctx context.Context, memberID string) (*agentcore.Task, error) {
	if err := t.requireActive(memberID); err != nil {
		return nil, err
	}
	tasks, err := t.tasks.List(ctx, false)
	if err != nil {
		return nil, err
	}
	for _, candidate := range tasks {
		if candidate.Status != agentcore.TaskPending || attemptsOf(candidate) >= defaultMaxAttempts {
			continue
		}
		ready, err := t.dependenciesCompleted(ctx, candidate)
		if err != nil {
			return nil, err
		}
		if !ready {
			continue
		}
		claimed, err := t.tasks.UpdateFunc(ctx, candidate.ID, func(task *agentcore.Task) error {
			if task.Status != agentcore.TaskPending {
				return errNotClaimable
			}
			task.Status = agentcore.TaskInProgress
			task.Owner = memberID
			task.UpdatedAt = time.Now()
			return nil
		})
		if err != nil {
			if errors.Is(err, errNotClaimable) {
				continue
			}
			return nil, err
		}
		t.emit("task_claimed", map[string]any{"task": claimed.ID, "member": memberID})
		return claimed, nil
	}
	return nil, nil
}

func (t *Team) dependenciesCompleted(ctx context.Context, task *agentcore.Task) (bool, error) {
	for _, dependency := range task.BlockedBy {
		dep, err := t.tasks.Get(ctx, dependency)
		if err != nil {
			return false, fmt.Errorf("读取依赖任务 %s 失败：%w", dependency, err)
		}
		// 归档是终态（取消/丢弃），不再阻塞下游；若业务上要求「取消即阻断」，
		// 需在任务层显式建立新的依赖。
		if dep.Status != agentcore.TaskCompleted && dep.Status != agentcore.TaskArchived {
			return false, nil
		}
	}
	return true, nil
}

// CompleteTask 由认领者把任务标记为完成。
func (t *Team) CompleteTask(ctx context.Context, memberID, taskID, result string) error {
	_, err := t.tasks.UpdateFunc(ctx, taskID, func(task *agentcore.Task) error {
		if task.Status != agentcore.TaskInProgress {
			return fmt.Errorf("任务 %s 不在进行中（%s）", taskID, task.Status)
		}
		if task.Owner != "" && task.Owner != memberID {
			return fmt.Errorf("任务 %s 由 %s 认领，%s 不能完成", taskID, task.Owner, memberID)
		}
		task.Status = agentcore.TaskCompleted
		task.UpdatedAt = time.Now()
		if result != "" {
			if task.Metadata == nil {
				task.Metadata = make(map[string]any)
			}
			task.Metadata[resultKey] = result
		}
		return nil
	})
	if err != nil {
		return err
	}
	t.emit("task_completed", map[string]any{"task": taskID, "member": memberID})
	t.signal()
	return nil
}

// FailTask 把任务退回 pending 并累计尝试次数；次数用尽后不再被认领。
func (t *Team) FailTask(ctx context.Context, memberID, taskID, reason string) error {
	_, err := t.tasks.UpdateFunc(ctx, taskID, func(task *agentcore.Task) error {
		if task.Status != agentcore.TaskInProgress {
			return fmt.Errorf("任务 %s 不在进行中（%s）", taskID, task.Status)
		}
		task.Status = agentcore.TaskPending
		task.Owner = ""
		task.UpdatedAt = time.Now()
		if task.Metadata == nil {
			task.Metadata = make(map[string]any)
		}
		task.Metadata[attemptsKey] = attemptsOf(task) + 1
		if reason != "" {
			task.Metadata[resultKey] = "失败：" + reason
		}
		return nil
	})
	if err != nil {
		return err
	}
	t.emit("task_failed", map[string]any{"task": taskID, "member": memberID, "reason": reason})
	t.signal()
	return nil
}

func attemptsOf(task *agentcore.Task) int {
	value, ok := task.Metadata[attemptsKey]
	if !ok {
		return 0
	}
	switch number := value.(type) {
	case int:
		return number
	case int64:
		return int(number)
	case float64:
		return int(number)
	default:
		return 0
	}
}

// RunUntilIdle 让所有空闲成员并行认领并执行任务，直到没有任务可执行。
// 返回本轮处理的任务数；runner 错误会记为任务失败并继续调度其余成员。
func (t *Team) RunUntilIdle(ctx context.Context, runner Runner) (int, error) {
	total := 0
	for {
		executed, err := t.dispatchRound(ctx, runner)
		total += executed
		if err != nil {
			return total, err
		}
		if executed == 0 {
			return total, nil
		}
	}
}

// dispatchRound 为每个空闲成员启动一个工作协程，各协程串行认领任务。
func (t *Team) dispatchRound(ctx context.Context, runner Runner) (int, error) {
	members := t.idleMembers()
	if len(members) == 0 {
		return 0, nil
	}
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		executed int
		firstErr error
	)
	for _, member := range members {
		wg.Go(func() {
			ran, err := t.work(ctx, runner, member)
			mu.Lock()
			executed += ran
			if err != nil && firstErr == nil {
				firstErr = err
			}
			mu.Unlock()
		})
	}
	wg.Wait()
	return executed, firstErr
}

// work 让单个成员循环认领任务直到无任务可做。
func (t *Team) work(ctx context.Context, runner Runner, member Member) (int, error) {
	executed := 0
	if err := t.SetStatus(member.ID, MemberRunning); err != nil {
		return 0, err
	}
	defer func() { _ = t.SetStatus(member.ID, MemberIdle) }()
	for {
		if err := ctx.Err(); err != nil {
			return executed, err
		}
		task, err := t.ClaimNext(ctx, member.ID)
		if err != nil {
			if errors.Is(err, ErrMemberRetired) {
				return executed, nil
			}
			return executed, err
		}
		if task == nil {
			return executed, nil
		}
		executed++
		result, runErr := runner.RunTask(ctx, member, task)
		if runErr != nil {
			if err := t.FailTask(ctx, member.ID, task.ID, runErr.Error()); err != nil {
				return executed, err
			}
			continue
		}
		if err := t.CompleteTask(ctx, member.ID, task.ID, result); err != nil {
			return executed, err
		}
	}
}

func (t *Team) idleMembers() []Member {
	members := t.Members()
	idle := make([]Member, 0, len(members))
	for _, member := range members {
		if member.Status == MemberIdle {
			idle = append(idle, member)
		}
	}
	return idle
}

// Wake 通知调度器有新事件（非阻塞）。
func (t *Team) Wake() { t.signal() }

// Run 事件驱动地持续调度，直到 ctx 取消。
//
// 单轮调度失败（任务失败、存储瞬时错误）只记录日志并继续：失败任务已退回
// pending 并累计尝试次数，达到上限后不再认领。只有 ctx 取消才结束主循环。
func (t *Team) Run(ctx context.Context, runner Runner) error {
	t.signal()
	for {
		if !t.pending.Swap(false) {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-t.wake:
			}
		}
		if _, err := t.RunUntilIdle(ctx, runner); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			slog.Warn("teams: 调度轮次失败", "team", t.id, "err", err)
		}
	}
}
