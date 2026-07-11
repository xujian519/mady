package psychological

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Store 持久化 SDT 状态和历史情绪摘要
// 使用 JSON 文件存储，路径: {dir}/{sessionID}.json
type Store struct {
	dir string
}

// NewStore 创建持久化存储
// dir 为存储目录，默认使用 ~/.mady/psychological/
func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Store{dir: dir}, nil
}

// storedData 持久化的数据结构
type storedData struct {
	SDTState   SDTState `json:"sdt_state"`
	RoundCount int      `json:"round_count"`
}

// LoadSDTState 从文件加载 SDT 状态
// 如果文件不存在则返回 (nil, nil) 表示首次会话
func (s *Store) LoadSDTState(sessionID string) (*storedData, error) {
	path := filepath.Join(s.dir, sessionID+".json")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var data storedData
	if err := json.NewDecoder(f).Decode(&data); err != nil {
		return nil, err
	}
	return &data, nil
}

// SaveSDTState 持久化 SDT 状态
func (s *Store) SaveSDTState(sessionID string, state SDTState, roundCount int) error {
	data := storedData{SDTState: state, RoundCount: roundCount}
	encoded, err := json.Marshal(data)
	if err != nil {
		return err
	}
	path := filepath.Join(s.dir, sessionID+".json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, encoded, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
