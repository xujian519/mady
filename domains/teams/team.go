package teams

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/agentcore/tasklist"
)

const teamsDirName = ".teams"

// Team 是一个团队目录的会话级协作句柄。
//
// 团队目录布局：
//
//	<workspace>/.teams/<teamId>/
//	  members.json     成员登记（原子写）
//	  events.jsonl     团队事件（追加写，命名 teams/<teamId>/<event>）
//	  tasks/           复用 tasklist.FileStore
//	  mail/            每成员一个 JSONL + 确认日志 + 投递租约
//	  locks/           团队锁
type Team struct {
	id        string
	dir       string
	createdAt time.Time
	tasks     tasklist.Store
	mail      *Mailbox
	events    *eventLog
	wake      chan struct{}
	pending   atomic.Bool
	mu        sync.Mutex
	members   map[string]*Member
}

type membersFile struct {
	TeamID    string    `json:"team_id"`
	CreatedAt time.Time `json:"created_at"`
	Members   []Member  `json:"members"`
}

// Open 打开或创建团队目录。同一团队应只由一个进程写入（租约只保护邮件投递
// 与团队锁），成员登记由进程内互斥保护。
func Open(workspaceDir, teamID string) (*Team, error) {
	if strings.TrimSpace(workspaceDir) == "" {
		return nil, fmt.Errorf("工作区目录不能为空")
	}
	if err := validName(teamID); err != nil {
		return nil, fmt.Errorf("团队 ID 无效：%w", err)
	}
	dir := filepath.Join(workspaceDir, teamsDirName, teamID)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("创建团队目录失败：%w", err)
	}
	tasks, err := tasklist.NewFileStore(filepath.Join(dir, "tasks"))
	if err != nil {
		return nil, fmt.Errorf("创建团队任务存储失败：%w", err)
	}
	team := &Team{
		id: teamID, dir: dir, createdAt: time.Now(), tasks: tasks,
		mail:    newMailbox(filepath.Join(dir, "mail")),
		events:  newEventLog(filepath.Join(dir, "events.jsonl")),
		wake:    make(chan struct{}, 1),
		members: make(map[string]*Member),
	}
	if err := team.loadMembers(); err != nil {
		return nil, err
	}
	return team, nil
}

func (t *Team) membersPath() string { return filepath.Join(t.dir, "members.json") }

func (t *Team) loadMembers() error {
	data, err := os.ReadFile(t.membersPath()) //nolint:gosec // 团队目录内固定文件名
	if err != nil {
		if os.IsNotExist(err) {
			return t.persistMembers()
		}
		return fmt.Errorf("读取成员登记失败：%w", err)
	}
	var file membersFile
	if err := json.Unmarshal(data, &file); err != nil {
		return fmt.Errorf("解析成员登记失败：%w", err)
	}
	for i := range file.Members {
		member := file.Members[i]
		t.members[member.ID] = &member
	}
	if !file.CreatedAt.IsZero() {
		t.createdAt = file.CreatedAt
	}
	return nil
}

// signal 非阻塞地唤醒调度器。channel 已满时置挂起标记，避免调度器
// 在处理上一轮事件期间丢失本次唤醒。
func (t *Team) signal() {
	select {
	case t.wake <- struct{}{}:
	default:
		t.pending.Store(true)
	}
}

// persistMembers 原子写入成员登记；调用方须持有 t.mu。
func (t *Team) persistMembers() error {
	members := make([]Member, 0, len(t.members))
	for _, member := range t.members {
		members = append(members, *member)
	}
	sort.Slice(members, func(i, j int) bool { return members[i].ID < members[j].ID })
	data, err := json.MarshalIndent(membersFile{TeamID: t.id, CreatedAt: t.createdAt, Members: members}, "", "  ")
	if err != nil {
		return err
	}
	tmp := t.membersPath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("写入成员登记失败：%w", err)
	}
	if err := os.Rename(tmp, t.membersPath()); err != nil {
		return fmt.Errorf("提交成员登记失败：%w", err)
	}
	return nil
}

// ID 返回团队标识。
func (t *Team) ID() string { return t.id }

// Dir 返回团队目录。
func (t *Team) Dir() string { return t.dir }

// Join 登记一名成员；重复 ID 返回 ErrMemberExists。
func (t *Team) Join(member Member) error {
	if err := validMemberID(member.ID); err != nil {
		return err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, exists := t.members[member.ID]; exists {
		return fmt.Errorf("%w：%s", ErrMemberExists, member.ID)
	}
	now := time.Now()
	if member.Name == "" {
		member.Name = member.ID
	}
	if member.Role == "" {
		member.Role = RoleMember
	}
	member.Status = MemberIdle
	member.CreatedAt, member.UpdatedAt = now, now
	t.members[member.ID] = &member
	if err := t.persistMembers(); err != nil {
		delete(t.members, member.ID)
		return err
	}
	t.emit("member_joined", map[string]any{"member": member.ID, "role": member.Role})
	return nil
}

// Retire 将成员标记为退役；退役成员不再接收任务与邮件。
func (t *Team) Retire(memberID string) error {
	return t.SetStatus(memberID, MemberRetired)
}

// SetStatus 更新成员状态；退役不可逆。
func (t *Team) SetStatus(memberID string, status MemberStatus) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	member, ok := t.members[memberID]
	if !ok {
		return fmt.Errorf("%w：%s", ErrMemberNotFound, memberID)
	}
	if member.Status == MemberRetired && status != MemberRetired {
		return fmt.Errorf("%w：%s", ErrMemberRetired, memberID)
	}
	if member.Status == status {
		return nil
	}
	member.Status = status
	member.UpdatedAt = time.Now()
	if err := t.persistMembers(); err != nil {
		return err
	}
	t.emit("member_status", map[string]any{"member": memberID, "status": string(status)})
	return nil
}

// Members 返回按 ID 排序的成员副本。
func (t *Team) Members() []Member {
	t.mu.Lock()
	defer t.mu.Unlock()
	members := make([]Member, 0, len(t.members))
	for _, member := range t.members {
		members = append(members, *member)
	}
	sort.Slice(members, func(i, j int) bool { return members[i].ID < members[j].ID })
	return members
}

// Member 按 ID 取成员副本。
func (t *Team) Member(memberID string) (Member, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	member, ok := t.members[memberID]
	if !ok {
		return Member{}, false
	}
	return *member, true
}

// SendMessage 在成员之间投递邮件。
func (t *Team) SendMessage(from, to, subject, body string) (Message, error) {
	if err := t.requireActive(to); err != nil {
		return Message{}, err
	}
	if err := t.requireActive(from); err != nil {
		return Message{}, fmt.Errorf("发件人：%w", err)
	}
	message, err := t.mail.Send(from, to, subject, body)
	if err != nil {
		return Message{}, err
	}
	t.emit("message_sent", map[string]any{"id": message.ID, "from": from, "to": to})
	return message, nil
}

// PendingMessages 返回成员的未确认邮件。
func (t *Team) PendingMessages(memberID string) ([]Message, error) {
	if _, ok := t.Member(memberID); !ok {
		return nil, fmt.Errorf("%w：%s", ErrMemberNotFound, memberID)
	}
	return t.mail.Pending(memberID)
}

// DeliverMail 在成员租约保护下投递未确认邮件；退役成员不再投递。
func (t *Team) DeliverMail(ctx context.Context, memberID, owner string, ttl time.Duration,
	handle func(Message) error) (int, error) {
	if err := t.requireActive(memberID); err != nil {
		return 0, err
	}
	delivered, err := t.mail.Deliver(ctx, memberID, owner, ttl, handle)
	if delivered > 0 {
		t.emit("mail_delivered", map[string]any{"member": memberID, "count": delivered})
	}
	return delivered, err
}

func (t *Team) requireActive(memberID string) error {
	member, ok := t.Member(memberID)
	if !ok {
		return fmt.Errorf("%w：%s", ErrMemberNotFound, memberID)
	}
	if member.Status == MemberRetired {
		return fmt.Errorf("%w：%s", ErrMemberRetired, memberID)
	}
	return nil
}

// Lock 获取团队锁；未过期的同名锁返回 ErrLocked。
func (t *Team) Lock(name, owner string, ttl time.Duration) (func(), error) {
	if err := validName(name); err != nil {
		return nil, err
	}
	if ttl <= 0 {
		ttl = defaultLockTTL
	}
	release, err := acquireLease(filepath.Join(t.dir, "locks", name+".lock"), owner, ttl)
	if err != nil {
		return nil, err
	}
	t.emit("lock_acquired", map[string]any{"lock": name, "owner": owner})
	return func() {
		release()
		t.emit("lock_released", map[string]any{"lock": name, "owner": owner})
	}, nil
}

// LockNames 返回当前存在的锁（不区分是否过期）。
func (t *Team) LockNames() []string {
	entries, err := os.ReadDir(filepath.Join(t.dir, "locks"))
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".lock") {
			names = append(names, strings.TrimSuffix(entry.Name(), ".lock"))
		}
	}
	sort.Strings(names)
	return names
}

// View 汇总团队状态；任务列表含归档任务，便于审计。
func (t *Team) View(ctx context.Context) (*View, error) {
	tasks, err := t.tasks.List(ctx, true)
	if err != nil {
		return nil, err
	}
	view := &View{
		TeamID: t.id, Dir: t.dir, Members: t.Members(),
		PendingMessages: make(map[string]int), Locks: t.LockNames(),
	}
	for _, task := range tasks {
		switch task.Status {
		case agentcore.TaskPending:
			view.Tasks.Pending++
		case agentcore.TaskInProgress:
			view.Tasks.InProgress++
		case agentcore.TaskCompleted:
			view.Tasks.Completed++
		case agentcore.TaskArchived:
			view.Tasks.Archived++
		}
		view.TaskList = append(view.TaskList, TaskSummary{
			ID: task.ID, Subject: task.Subject, Status: task.Status, Owner: task.Owner,
		})
	}
	for _, member := range view.Members {
		pending, err := t.mail.Pending(member.ID)
		if err != nil {
			return nil, err
		}
		view.PendingMessages[member.ID] = len(pending)
	}
	return view, nil
}

// EventRecord 是一条团队事件。
type EventRecord struct {
	Name string          `json:"name"`
	At   time.Time       `json:"at"`
	Data json.RawMessage `json:"data,omitempty"`
}

// eventLog 以追加写记录团队事件。
type eventLog struct {
	path string
	mu   sync.Mutex
}

func newEventLog(path string) *eventLog { return &eventLog{path: path} }

func (l *eventLog) append(record EventRecord) error {
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	file, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(append(data, '\n'))
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}

// emit 写入团队事件；事件名遵循 teams/<teamId>/<event>。
func (t *Team) emit(event string, data map[string]any) {
	raw, err := json.Marshal(data)
	if err != nil {
		raw = []byte(`{}`)
	}
	_ = t.events.append(EventRecord{
		Name: "teams/" + t.id + "/" + event, At: time.Now(), Data: raw,
	})
}

// Events 读取全部团队事件（按写入顺序）。
func (t *Team) Events() ([]EventRecord, error) {
	file, err := os.Open(t.events.path) //nolint:gosec // 团队目录内固定文件名
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = file.Close() }()
	var records []EventRecord
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64<<10), 4<<20)
	for scanner.Scan() {
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			continue
		}
		var record EventRecord
		if err := json.Unmarshal([]byte(text), &record); err != nil {
			return nil, fmt.Errorf("解析团队事件失败：%w", err)
		}
		records = append(records, record)
	}
	return records, scanner.Err()
}
