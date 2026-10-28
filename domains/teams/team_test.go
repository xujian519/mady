package teams

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestTeam(t *testing.T, members ...string) *Team {
	t.Helper()
	team, err := Open(t.TempDir(), "case-1")
	if err != nil {
		t.Fatalf("打开团队失败：%v", err)
	}
	for i, id := range members {
		role := RoleMember
		if i == 0 {
			role = RoleLeader
		}
		if err := team.Join(Member{ID: id, Name: id, Role: role}); err != nil {
			t.Fatalf("加入成员失败：%v", err)
		}
	}
	return team
}

func TestJoinMembersPersistAndRetire(t *testing.T) {
	workspace := t.TempDir()
	team, err := Open(workspace, "case-1")
	if err != nil {
		t.Fatalf("打开团队失败：%v", err)
	}
	for _, id := range []string{"leader", "alpha", "beta"} {
		if err := team.Join(Member{ID: id}); err != nil {
			t.Fatalf("加入 %s 失败：%v", id, err)
		}
	}
	if err := team.Join(Member{ID: "alpha"}); !errors.Is(err, ErrMemberExists) {
		t.Fatalf("重复成员应返回 ErrMemberExists，得到 %v", err)
	}

	reopened, err := Open(workspace, "case-1")
	if err != nil {
		t.Fatalf("重新打开团队失败：%v", err)
	}
	if members := reopened.Members(); len(members) != 3 {
		t.Fatalf("成员未持久化：%+v", members)
	}
	if err := reopened.Retire("beta"); err != nil {
		t.Fatalf("退役失败：%v", err)
	}
	if _, err := reopened.SendMessage("leader", "beta", "报告", "进度"); !errors.Is(err, ErrMemberRetired) {
		t.Fatalf("向退役成员发信应被拒绝，得到 %v", err)
	}
	if err := reopened.SetStatus("beta", MemberIdle); !errors.Is(err, ErrMemberRetired) {
		t.Fatalf("退役不可逆，得到 %v", err)
	}
}

func TestSendMessageAndView(t *testing.T) {
	team := newTestTeam(t, "leader", "alpha")
	if _, err := team.SendMessage("leader", "alpha", "请开始", "先做检索"); err != nil {
		t.Fatalf("发信失败：%v", err)
	}
	pending, err := team.PendingMessages("alpha")
	if err != nil || len(pending) != 1 {
		t.Fatalf("未读邮件 = %d, err = %v", len(pending), err)
	}
	if err := team.mail.Ack("alpha", pending[0].ID); err != nil {
		t.Fatalf("确认失败：%v", err)
	}
	pending, err = team.PendingMessages("alpha")
	if err != nil || len(pending) != 0 {
		t.Fatalf("确认后仍有未读：%d, err = %v", len(pending), err)
	}

	view, err := team.View(context.Background())
	if err != nil {
		t.Fatalf("状态视图失败：%v", err)
	}
	if view.TeamID != "case-1" || len(view.Members) != 2 || view.PendingMessages["alpha"] != 0 {
		t.Fatalf("状态视图不符：%+v", view)
	}
	events, err := team.Events()
	if err != nil || len(events) == 0 {
		t.Fatalf("团队事件为空：%v", err)
	}
	for _, event := range events {
		if !strings.HasPrefix(event.Name, "teams/case-1/") {
			t.Fatalf("事件命名不符：%s", event.Name)
		}
	}
}

func TestMailLeaseContentionAndRetry(t *testing.T) {
	team := newTestTeam(t, "leader", "alpha")
	for i := range 2 {
		if _, err := team.SendMessage("leader", "alpha", "通知", string(rune('A'+i))); err != nil {
			t.Fatalf("发信失败：%v", err)
		}
	}
	release := make(chan struct{})
	started := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		var once bool
		_, _ = team.DeliverMail(context.Background(), "alpha", "worker-1", time.Minute, func(Message) error {
			if !once {
				once = true
				close(started)
				<-release
			}
			return nil
		})
	}()
	<-started
	if _, err := team.DeliverMail(context.Background(), "alpha", "worker-2", time.Minute,
		func(Message) error { return nil }); !errors.Is(err, ErrLocked) {
		t.Fatalf("并发投递应返回 ErrLocked，得到 %v", err)
	}
	close(release)
	<-done

	// 处理失败时消息不确认，留在邮箱等待重试。
	if _, err := team.SendMessage("leader", "alpha", "会失败", "x"); err != nil {
		t.Fatalf("发信失败：%v", err)
	}
	boom := errors.New("boom")
	if _, err := team.DeliverMail(context.Background(), "alpha", "worker-1", time.Minute,
		func(Message) error { return boom }); !errors.Is(err, boom) {
		t.Fatalf("应返回处理错误，得到 %v", err)
	}
	pending, err := team.PendingMessages("alpha")
	if err != nil || len(pending) != 1 {
		t.Fatalf("失败邮件应保留：%d, err = %v", len(pending), err)
	}
	delivered, err := team.DeliverMail(context.Background(), "alpha", "worker-1", time.Minute,
		func(Message) error { return nil })
	if err != nil || delivered != 1 {
		t.Fatalf("重试投递 = %d, err = %v", delivered, err)
	}
}

func TestTeamLockContentionAndExpiry(t *testing.T) {
	team := newTestTeam(t, "leader")
	release, err := team.Lock("case-file", "leader", time.Minute)
	if err != nil {
		t.Fatalf("首次加锁失败：%v", err)
	}
	if _, err := team.Lock("case-file", "alpha", time.Minute); !errors.Is(err, ErrLocked) {
		t.Fatalf("锁竞争应返回 ErrLocked，得到 %v", err)
	}
	release()
	releaseAgain, err := team.Lock("case-file", "alpha", time.Minute)
	if err != nil {
		t.Fatalf("释放后应可再次加锁：%v", err)
	}
	releaseAgain()

	// 直接写入一份已过期的租约（模拟崩溃残留），验证接管路径。
	stale := filepath.Join(team.Dir(), "locks", "short.lock")
	if err := os.MkdirAll(filepath.Dir(stale), 0o750); err != nil {
		t.Fatalf("创建锁目录失败：%v", err)
	}
	data, err := json.Marshal(leaseRecord{Owner: "crashed", ExpiresAt: time.Now().Add(-time.Minute)})
	if err != nil {
		t.Fatalf("编码过期租约失败：%v", err)
	}
	if err := os.WriteFile(stale, data, 0o600); err != nil {
		t.Fatalf("写入过期租约失败：%v", err)
	}
	releaseShort, err := team.Lock("short", "alpha", time.Minute)
	if err != nil {
		t.Fatalf("过期租约应可接管：%v", err)
	}
	releaseShort()
}

func TestSendMessageRejectsOversizedBody(t *testing.T) {
	team := newTestTeam(t, "leader", "alpha")
	body := strings.Repeat("长", maxBodyRunes+1)
	if _, err := team.SendMessage("leader", "alpha", "超长", body); err == nil {
		t.Fatal("超长正文应被拒绝")
	}
}
