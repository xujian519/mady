package legal

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite" // register pure-Go SQLite driver
)

// InstallOptions 是法律知识库安装参数。
type InstallOptions struct {
	// SourcePath 是源库路径（只读打开，不会被修改）。
	SourcePath string
	// OutputPath 是安装产物路径。
	OutputPath string
	// NoFTS 为 true 时删除 law_fts 索引以压缩体积，检索降级为 LIKE。
	NoFTS bool
}

// InstallResult 是一次安装的结果摘要。
type InstallResult struct {
	InputPath   string         `json:"input"`
	OutputPath  string         `json:"output"`
	InputBytes  int64          `json:"input_bytes"`
	OutputBytes int64          `json:"output_bytes"`
	DroppedFTS  []string       `json:"dropped_fts,omitempty"`
	RowCounts   map[string]int `json:"row_counts"`
}

// Install 从源库拷贝一份法律知识库到输出路径。
//
// 用 VACUUM INTO 生成一致性快照（源库可能带 WAL 边车文件，直接拷贝主文件会
// 丢掉未落盘事务），再按需删除 FTS 索引，最后原子替换输出文件。源库不是
// 法律知识库（缺少 law 表）时拒绝安装，避免把错误路径装成「空库」。
func Install(ctx context.Context, opts InstallOptions) (InstallResult, error) {
	input := strings.TrimSpace(opts.SourcePath)
	output := strings.TrimSpace(opts.OutputPath)
	if input == "" || output == "" {
		return InstallResult{}, fmt.Errorf("安装需要源库路径与输出路径")
	}
	same, err := sameFile(input, output)
	if err != nil {
		return InstallResult{}, err
	}
	if same {
		return InstallResult{}, fmt.Errorf("输出路径与源库指向同一文件，安装会破坏源库；请指定不同路径")
	}
	inputInfo, err := os.Stat(input)
	if err != nil {
		return InstallResult{}, fmt.Errorf("源库不可读: %w", err)
	}

	source, err := openForInstall(input, true)
	if err != nil {
		return InstallResult{}, err
	}
	defer func() { _ = source.Close() }()
	if !hasTable(ctx, source, "law") {
		return InstallResult{}, fmt.Errorf("源库 %s 缺少 law 表，不是可用的法律知识库", input)
	}

	if err := os.MkdirAll(filepath.Dir(output), 0o750); err != nil {
		return InstallResult{}, fmt.Errorf("创建输出目录失败: %w", err)
	}
	tmp, err := tempSibling(output)
	if err != nil {
		return InstallResult{}, err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmp)
		}
	}()

	if _, err := source.ExecContext(ctx, "VACUUM INTO ?", tmp); err != nil {
		return InstallResult{}, fmt.Errorf("生成数据库快照失败: %w", err)
	}

	var dropped []string
	if opts.NoFTS {
		dropped, err = dropFTSTables(ctx, tmp)
		if err != nil {
			return InstallResult{}, err
		}
	}

	if err := os.Rename(tmp, output); err != nil {
		// Windows 上目标已存在时 Rename 失败，先删旧文件再试一次。
		if removeErr := os.Remove(output); removeErr != nil && !os.IsNotExist(removeErr) {
			return InstallResult{}, fmt.Errorf("替换输出文件失败: %w", err)
		}
		if err := os.Rename(tmp, output); err != nil {
			return InstallResult{}, fmt.Errorf("替换输出文件失败: %w", err)
		}
	}
	cleanup = false

	outputInfo, err := os.Stat(output)
	if err != nil {
		return InstallResult{}, fmt.Errorf("读取安装产物失败: %w", err)
	}
	rows, err := countRows(ctx, output)
	if err != nil {
		return InstallResult{}, err
	}
	return InstallResult{
		InputPath:   input,
		OutputPath:  output,
		InputBytes:  inputInfo.Size(),
		OutputBytes: outputInfo.Size(),
		DroppedFTS:  dropped,
		RowCounts:   rows,
	}, nil
}

// openForInstall 打开安装用连接；readOnly 时源库不会被修改。
func openForInstall(path string, readOnly bool) (*sql.DB, error) {
	dsn := path
	if readOnly {
		dsn = fmt.Sprintf("file:%s?mode=ro&_pragma=busy_timeout(5000)", path)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开 %s 失败: %w", path, err)
	}
	db.SetMaxOpenConns(1)
	return db, nil
}

// dropFTSTables 删除 law_fts 虚表（影子表随虚表一并删除）并整理空间。
func dropFTSTables(ctx context.Context, path string) ([]string, error) {
	db, err := openForInstall(path, false)
	if err != nil {
		return nil, err
	}
	defer func() { _ = db.Close() }()

	if !hasTable(ctx, db, "law_fts") {
		return nil, nil
	}
	if _, err := db.ExecContext(ctx, "DROP TABLE law_fts"); err != nil {
		return nil, fmt.Errorf("删除 law_fts 失败: %w", err)
	}
	if _, err := db.ExecContext(ctx, "VACUUM"); err != nil {
		return nil, fmt.Errorf("整理安装产物失败: %w", err)
	}
	return []string{"law_fts"}, nil
}

// countRows 统计关键表的行数，供安装摘要使用。
func countRows(ctx context.Context, path string) (map[string]int, error) {
	db, err := openForInstall(path, true)
	if err != nil {
		return nil, err
	}
	defer func() { _ = db.Close() }()

	counts := make(map[string]int)
	for _, table := range []string{"law", "guiding_cases"} {
		if !hasTable(ctx, db, table) {
			continue
		}
		var n int
		if err := db.QueryRowContext(ctx, fmt.Sprintf(`SELECT COUNT(*) FROM "%s"`, table)).Scan(&n); err != nil {
			return nil, fmt.Errorf("统计 %s 行数失败: %w", table, err)
		}
		counts[table] = n
	}
	return counts, nil
}

// tempSibling 在输出文件同目录创建一个临时文件并返回其路径。
// 同目录可保证随后的 Rename 在同一文件系统内完成。
func tempSibling(output string) (string, error) {
	dir := filepath.Dir(output)
	f, err := os.CreateTemp(dir, ".legal-*.db")
	if err != nil {
		return "", fmt.Errorf("创建临时文件失败: %w", err)
	}
	name := f.Name()
	if err := f.Close(); err != nil {
		_ = os.Remove(name)
		return "", fmt.Errorf("关闭临时文件失败: %w", err)
	}
	// VACUUM INTO 要求目标不存在。
	if err := os.Remove(name); err != nil {
		return "", fmt.Errorf("准备临时文件失败: %w", err)
	}
	return name, nil
}

// sameFile 判断两个路径是否指向同一文件（解析符号链接与相对路径）。
func sameFile(a, b string) (bool, error) {
	absA, err := filepath.Abs(a)
	if err != nil {
		return false, fmt.Errorf("解析路径 %s: %w", a, err)
	}
	absB, err := filepath.Abs(b)
	if err != nil {
		return false, fmt.Errorf("解析路径 %s: %w", b, err)
	}
	infoA, errA := os.Stat(absA)
	infoB, errB := os.Stat(absB)
	if errA == nil && errB == nil {
		return os.SameFile(infoA, infoB), nil
	}
	return absA == absB, nil
}
