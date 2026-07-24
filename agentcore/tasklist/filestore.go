package tasklist

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/xujian519/mady/agentcore"
)

// FileStore 是基于文件系统的 Store 实现。
// 每个任务存为 baseDir/<id>.json，ID 计数器存为 baseDir/.nextid。
// 写入采用"写临时文件 + rename"的原子模式，防止崩溃导致数据损坏。
// baseDir 在构造时确保存在；所有路径都在 baseDir 内，不访问外部目录。
type FileStore struct {
	mu      sync.Mutex
	baseDir string
	nextID  int64 // 内存缓存，启动时从 .nextid 文件加载；所有访问在 mu 下
}

// NewFileStore 在 baseDir 下创建文件存储。baseDir 不存在时自动创建。
func NewFileStore(baseDir string) (*FileStore, error) {
	if baseDir == "" {
		return nil, fmt.Errorf("tasklist: baseDir is empty")
	}
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("tasklist: create base dir %s: %w", baseDir, err)
	}
	fs := &FileStore{baseDir: baseDir}
	fs.loadNextID()
	return fs, nil
}

// loadNextID 从 .nextid 文件加载计数器，不存在时从现有 JSON 文件推断。
func (f *FileStore) loadNextID() {
	// 先尝试读取 .nextid 文件
	if data, err := os.ReadFile(f.nextIDPath()); err == nil {
		if n, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64); err == nil && n > 0 {
			f.nextID = n
			return
		}
	}
	// 回退：扫描现有 JSON 文件，取最大数字 ID
	maxID := int64(0)
	entries, err := os.ReadDir(f.baseDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		idStr := strings.TrimSuffix(name, ".json")
		if n, err := strconv.ParseInt(idStr, 10, 64); err == nil && n > maxID {
			maxID = n
		}
	}
	f.nextID = maxID
}

func (f *FileStore) nextIDPath() string        { return filepath.Join(f.baseDir, ".nextid") }
func (f *FileStore) taskPath(id string) string { return filepath.Join(f.baseDir, id+".json") }

func (f *FileStore) Create(_ context.Context, t *agentcore.Task) error {
	if t.ID == "" {
		return fmt.Errorf("tasklist: task ID is empty")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	path := f.taskPath(t.ID)
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("tasklist: task #%s already exists", t.ID)
	}
	return f.writeTask(path, t)
}

func (f *FileStore) Get(_ context.Context, id string) (*agentcore.Task, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.readTask(f.taskPath(id))
}

func (f *FileStore) Update(_ context.Context, t *agentcore.Task) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	path := f.taskPath(t.ID)
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("tasklist: task #%s not found", t.ID)
	}
	return f.writeTask(path, t)
}

func (f *FileStore) UpdateFunc(_ context.Context, id string, mutate func(*agentcore.Task) error) (*agentcore.Task, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	path := f.taskPath(id)
	t, err := f.readTask(path)
	if err != nil {
		return nil, err
	}
	if err := mutate(t); err != nil {
		return nil, err
	}
	if err := f.writeTask(path, t); err != nil {
		return nil, err
	}
	return t, nil
}

func (f *FileStore) List(_ context.Context, includeArchived bool) ([]*agentcore.Task, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	entries, err := os.ReadDir(f.baseDir)
	if err != nil {
		return nil, fmt.Errorf("tasklist: list dir %s: %w", f.baseDir, err)
	}
	var result []*agentcore.Task
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		t, err := f.readTask(filepath.Join(f.baseDir, name))
		if err != nil {
			continue // 跳过无法解析的文件
		}
		if !includeArchived && t.Status == agentcore.TaskArchived {
			continue
		}
		result = append(result, t)
	}
	sortTasks(result)
	return result, nil
}

func (f *FileStore) Delete(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	path := f.taskPath(id)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("tasklist: delete task #%s: %w", id, err)
	}
	return nil
}

func (f *FileStore) NextID(_ context.Context) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	id := f.nextID
	// 持久化计数器（原子写入）
	idStr := fmt.Sprintf("%d", id)
	tmp := f.nextIDPath() + ".tmp"
	if err := os.WriteFile(tmp, []byte(idStr), 0600); err != nil {
		return "", fmt.Errorf("tasklist: write nextid: %w", err)
	}
	if err := os.Rename(tmp, f.nextIDPath()); err != nil {
		return "", fmt.Errorf("tasklist: rename nextid: %w", err)
	}
	return idStr, nil
}

// writeTask 以原子方式（写临时文件 + rename）写入任务 JSON。
func (f *FileStore) writeTask(path string, t *agentcore.Task) error {
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return fmt.Errorf("tasklist: marshal task #%s: %w", t.ID, err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("tasklist: write task #%s: %w", t.ID, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("tasklist: rename task #%s: %w", t.ID, err)
	}
	return nil
}

// readTask 从文件读取并反序列化任务。
func (f *FileStore) readTask(path string) (*agentcore.Task, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("tasklist: task not found: %s", filepath.Base(path))
		}
		return nil, fmt.Errorf("tasklist: read %s: %w", path, err)
	}
	var t agentcore.Task
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("tasklist: parse %s: %w", path, err)
	}
	return &t, nil
}

// compile-time interface check
var (
	_ Store = (*MemoryStore)(nil)
	_ Store = (*FileStore)(nil)
)
