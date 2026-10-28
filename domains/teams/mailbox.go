package teams

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// 邮件大小上限：远低于 JSONL 扫描器的 4 MB 单行上限，避免一封超长邮件
// 让整箱不可读。
const (
	maxSubjectRunes = 200
	maxBodyRunes    = 20000
)

// Mailbox 是每成员一个 JSONL 的团队邮箱：投递追加写、确认追加写，
// 读取按「未确认」过滤，投递过程由成员租约串行化。
type Mailbox struct {
	dir string
	mu  sync.Mutex
	seq atomic.Uint64
}

func newMailbox(dir string) *Mailbox { return &Mailbox{dir: dir} }

func (m *Mailbox) messagePath(member string) string {
	return filepath.Join(m.dir, member+".jsonl")
}

func (m *Mailbox) ackPath(member string) string {
	return filepath.Join(m.dir, member+".ack.jsonl")
}

func (m *Mailbox) leasePath(member string) string {
	return filepath.Join(m.dir, member+".lease")
}

// Send 投递一封邮件；追加写保证并发投递不互相截断。
func (m *Mailbox) Send(from, to, subject, body string) (Message, error) {
	if err := validMemberID(to); err != nil {
		return Message{}, err
	}
	if err := validMemberID(from); err != nil {
		return Message{}, fmt.Errorf("发件人无效：%w", err)
	}
	subject, body = strings.TrimSpace(subject), strings.TrimSpace(body)
	if len([]rune(subject)) > maxSubjectRunes {
		return Message{}, fmt.Errorf("邮件主题超过 %d 字上限", maxSubjectRunes)
	}
	if len([]rune(body)) > maxBodyRunes {
		return Message{}, fmt.Errorf("邮件正文超过 %d 字上限", maxBodyRunes)
	}
	message := Message{
		ID:   fmt.Sprintf("%d-%06d", time.Now().UnixNano(), m.seq.Add(1)),
		From: from, To: to, Subject: subject, Body: body, At: time.Now(),
	}
	if err := m.append(m.messagePath(to), message); err != nil {
		return Message{}, fmt.Errorf("投递邮件失败：%w", err)
	}
	return message, nil
}

func (m *Mailbox) append(path string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(m.dir, 0o750); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600) //nolint:gosec // 路径由团队目录拼接
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

// Pending 返回成员尚未确认的邮件（按投递顺序）。
func (m *Mailbox) Pending(member string) ([]Message, error) {
	if err := validMemberID(member); err != nil {
		return nil, err
	}
	acked, err := readAcks(m.ackPath(member))
	if err != nil {
		return nil, err
	}
	messages, err := readMessages(m.messagePath(member))
	if err != nil {
		return nil, err
	}
	pending := make([]Message, 0, len(messages))
	for _, message := range messages {
		if !acked[message.ID] {
			pending = append(pending, message)
		}
	}
	return pending, nil
}

func readMessages(path string) ([]Message, error) {
	return readJSONLines[Message](path)
}

func readAcks(path string) (map[string]bool, error) {
	records, err := readJSONLines[struct {
		ID string `json:"id"`
	}](path)
	if err != nil {
		return nil, err
	}
	acked := make(map[string]bool, len(records))
	for _, record := range records {
		acked[record.ID] = true
	}
	return acked, nil
}

// readJSONLines 逐行解析 JSONL；文件不存在时返回空。坏行 fail-closed 报错。
func readJSONLines[T any](path string) ([]T, error) {
	file, err := os.Open(path) //nolint:gosec // 路径由团队目录拼接
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = file.Close() }()
	var records []T
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64<<10), 4<<20)
	line := 0
	for scanner.Scan() {
		line++
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			continue
		}
		var record T
		if err := json.Unmarshal([]byte(text), &record); err != nil {
			return nil, fmt.Errorf("%s 第 %d 行无法解析：%w", filepath.Base(path), line, err)
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

// Ack 确认邮件；重复确认是幂等的。
func (m *Mailbox) Ack(member string, ids ...string) error {
	if err := validMemberID(member); err != nil {
		return err
	}
	for _, id := range ids {
		if id == "" {
			continue
		}
		if err := m.append(m.ackPath(member), map[string]string{"id": id}); err != nil {
			return fmt.Errorf("确认邮件失败：%w", err)
		}
	}
	return nil
}

// Deliver 在成员租约保护下把未确认邮件逐封交给 handle；处理成功才确认。
// 中途失败时已确认的邮件保留，其余留待下次投递。
func (m *Mailbox) Deliver(ctx context.Context, member, owner string, ttl time.Duration,
	handle func(Message) error) (int, error) {
	release, err := acquireLease(m.leasePath(member), owner, ttl)
	if err != nil {
		return 0, err
	}
	defer release()

	messages, err := m.Pending(member)
	if err != nil {
		return 0, err
	}
	delivered := 0
	for _, message := range messages {
		if err := ctx.Err(); err != nil {
			return delivered, err
		}
		if err := handle(message); err != nil {
			return delivered, fmt.Errorf("处理邮件 %s 失败：%w", message.ID, err)
		}
		if err := m.Ack(member, message.ID); err != nil {
			return delivered, err
		}
		delivered++
	}
	return delivered, nil
}
