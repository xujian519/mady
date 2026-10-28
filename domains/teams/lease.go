package teams

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultLeaseTTL = 30 * time.Second
	defaultLockTTL  = 5 * time.Minute
)

type leaseRecord struct {
	Owner     string    `json:"owner"`
	ExpiresAt time.Time `json:"expires_at"`
}

// acquireLease 以 O_EXCL 原子地获取文件租约；未过期的既有租约返回 ErrLocked，
// 过期租约清理后重试一次。返回的释放函数只删除自己持有的租约。
func acquireLease(path, owner string, ttl time.Duration) (func(), error) {
	if ttl <= 0 {
		ttl = defaultLeaseTTL
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, fmt.Errorf("创建租约目录失败：%w", err)
	}
	for range 2 {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) //nolint:gosec // 路径由团队目录拼接
		if err == nil {
			data, err := json.Marshal(leaseRecord{Owner: owner, ExpiresAt: time.Now().Add(ttl)})
			if err != nil {
				_ = file.Close()
				_ = os.Remove(path)
				return nil, fmt.Errorf("编码租约失败：%w", err)
			}
			writeErr := func() error {
				if _, err := file.Write(data); err != nil {
					return err
				}
				return file.Close()
			}()
			if writeErr != nil {
				_ = os.Remove(path)
				return nil, fmt.Errorf("写入租约失败：%w", writeErr)
			}
			return func() { releaseLease(path, owner) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("获取租约失败：%w", err)
		}
		if !leaseExpired(path) {
			return nil, ErrLocked
		}
		// 过期租约用 rename 原子接管：只有把自己看到的那份文件移走的人才继续
		// 竞争；直接 Remove 可能删掉别人刚建好的新租约。
		stale := fmt.Sprintf("%s.stale-%d", path, time.Now().UnixNano())
		if err := os.Rename(path, stale); err != nil {
			continue // 竞态中已被他人接管，下一轮重新 O_EXCL
		}
		_ = os.Remove(stale)
	}
	return nil, ErrLocked
}

// leaseExpired 判断租约是否已过期。无法解析的租约按「占用中」处理，
// 由 TTL 到期后清理；文件不存在视为已释放。
func leaseExpired(path string) bool {
	data, err := os.ReadFile(path) //nolint:gosec // 路径由团队目录拼接
	if err != nil {
		return errors.Is(err, os.ErrNotExist)
	}
	var record leaseRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return false
	}
	return time.Now().After(record.ExpiresAt)
}

// releaseLease 只释放 owner 自己持有的租约，避免误删他人的锁。
func releaseLease(path, owner string) {
	data, err := os.ReadFile(path) //nolint:gosec // 路径由团队目录拼接
	if err != nil {
		return
	}
	var record leaseRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return
	}
	if record.Owner == owner {
		_ = os.Remove(path)
	}
}

// validMemberID 校验成员 ID：小写字母/数字/下划线/连字符，且以字母或数字开头。
func validMemberID(id string) error {
	if id == "" || len(id) > 64 {
		return fmt.Errorf("成员 ID 必须为 1–64 个字符")
	}
	for i, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
		case (r == '_' || r == '-') && i > 0:
		default:
			return fmt.Errorf("成员 ID 只允许小写字母、数字、下划线与连字符：%q", id)
		}
	}
	return nil
}

// validName 校验锁名与团队 ID：字母/数字/下划线/连字符/点，且不以点开头。
func validName(name string) error {
	if name == "" || len(name) > 64 || strings.HasPrefix(name, ".") {
		return fmt.Errorf("名称必须为 1–64 个字符且不能以点开头：%q", name)
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '_' || r == '-' || r == '.':
		default:
			return fmt.Errorf("名称只允许字母、数字、下划线、连字符与点：%q", name)
		}
	}
	return nil
}
