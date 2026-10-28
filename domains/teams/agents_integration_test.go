package teams

import (
	"context"
	"testing"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/session"
)

func TestThreeAgentsCompleteTasksWithPersistedState(t *testing.T) {
	workspace := t.TempDir()
	team, err := Open(workspace, "case-1")
	if err != nil {
		t.Fatalf("打开团队失败：%v", err)
	}
	fileStore, err := session.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatalf("创建会话存储失败：%v", err)
	}
	store := session.NewAgentStore(fileStore, workspace)
	provider := &recordingProvider{}
	runner, err := NewAgentRunner(AgentRunnerConfig{
		Store:     store,
		Config:    agentcore.StubConfig(provider),
		KeyPrefix: "team-case-1-",
	})
	if err != nil {
		t.Fatalf("创建 AgentRunner 失败：%v", err)
	}
	for _, id := range []string{"alpha", "beta", "gamma"} {
		if err := team.Join(Member{ID: id}); err != nil {
			t.Fatalf("加入成员失败：%v", err)
		}
	}

	ctx := context.Background()
	first, err := team.AddTask(ctx, "检索现有技术", "", agentcore.TaskPriorityNormal, nil)
	if err != nil {
		t.Fatalf("创建任务失败：%v", err)
	}
	tasks := []string{first.ID}
	for _, subject := range []string{"分析权利要求", "撰写对比表", "汇总报告"} {
		task, err := team.AddTask(ctx, subject, "", agentcore.TaskPriorityNormal, nil)
		if err != nil {
			t.Fatalf("创建任务失败：%v", err)
		}
		tasks = append(tasks, task.ID)
	}
	if _, err := team.AddTask(ctx, "复核结论", "", agentcore.TaskPriorityUrgent, tasks); err != nil {
		t.Fatalf("创建复核任务失败：%v", err)
	}

	executed, err := team.RunUntilIdle(ctx, runner)
	if err != nil {
		t.Fatalf("调度失败：%v", err)
	}
	if executed != 5 {
		t.Fatalf("执行任务数 = %d, want 5", executed)
	}
	view, err := team.View(ctx)
	if err != nil || view.Tasks.Completed != 5 {
		t.Fatalf("完成数 = %d, err = %v", view.Tasks.Completed, err)
	}
	for _, id := range []string{"alpha", "beta", "gamma"} {
		if exists, err := store.Has(ctx, "team-case-1-"+id); err != nil || !exists {
			t.Fatalf("成员 %s 的上下文未持久化：exists=%v err=%v", id, exists, err)
		}
	}

	// 至少有一名成员执行了两个任务，其第二轮请求应携带第一轮上下文。
	var continued bool
	provider.mu.Lock()
	for _, messages := range provider.requests {
		users := 0
		for _, message := range messages {
			if message.Role == agentcore.RoleUser {
				users++
			}
		}
		if users >= 2 {
			continued = true
		}
	}
	provider.mu.Unlock()
	if !continued {
		t.Fatal("没有观察到跨任务的上下文续聊")
	}
}
